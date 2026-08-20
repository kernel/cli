package browserimport

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/net/publicsuffix"
)

const (
	maxDocumentBytes = 16 << 20
	maxSQLiteOutput  = 64 << 20
	maxSQLiteBytes   = 2 << 30
	maxStorageBytes  = MaxPortableStorageSize
	maxStorageSource = 512 << 20
	maxStorageRecord = 1 << 20
	maxStorageCount  = 100_000
)

var profileIDCharacters = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func DiscoverMacOSProfiles(home string) ([]Profile, error) {
	browsers := []Browser{
		{ID: "chrome", Name: "Google Chrome", Root: filepath.Join(home, "Library/Application Support/Google/Chrome"), Keychain: KeychainIdentity{Service: "Chrome Safe Storage", Account: "Chrome"}},
		{ID: "helium", Name: "Helium", Root: filepath.Join(home, "Library/Application Support/net.imput.helium"), Keychain: KeychainIdentity{Service: "Helium Storage Key", Account: "Helium"}},
	}
	profiles := make([]Profile, 0)
	for _, browser := range browsers {
		found, err := discoverProfiles(browser)
		if err != nil {
			return nil, fmt.Errorf("discover %s profiles: %w", browser.Name, err)
		}
		profiles = append(profiles, found...)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].DisplayName() < profiles[j].DisplayName() })
	return profiles, nil
}

func discoverProfiles(browser Browser) ([]Profile, error) {
	if _, err := os.Stat(browser.Root); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	names := make(map[string]string)
	data, err := readFileBounded(filepath.Join(browser.Root, "Local State"), maxDocumentBytes)
	if err == nil {
		var state struct {
			Profile struct {
				InfoCache map[string]struct {
					Name string `json:"name"`
				} `json:"info_cache"`
			} `json:"profile"`
		}
		if json.Unmarshal(data, &state) == nil {
			for directory, info := range state.Profile.InfoCache {
				names[directory] = info.Name
			}
		}
	}
	if len(names) == 0 {
		entries, err := os.ReadDir(browser.Root)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() && (entry.Name() == "Default" || strings.HasPrefix(entry.Name(), "Profile ")) {
				names[entry.Name()] = entry.Name()
			}
		}
	}

	directories := make([]string, 0, len(names))
	for directory := range names {
		directories = append(directories, directory)
	}
	sort.Strings(directories)
	profiles := make([]Profile, 0, len(directories))
	for _, directory := range directories {
		path := filepath.Join(browser.Root, directory)
		if !fileExists(filepath.Join(path, "History")) && !fileExists(filepath.Join(path, "Network/Cookies")) && !fileExists(filepath.Join(path, "Cookies")) {
			continue
		}
		profiles = append(profiles, Profile{ID: profileID(browser.ID, directory), Name: names[directory], Browser: browser, Path: path, Directory: directory})
	}
	return profiles, nil
}

func profileID(browserID, directory string) string {
	value := strings.Trim(profileIDCharacters.ReplaceAllString(strings.ToLower(browserID+"-"+directory), "-"), "-")
	digest := sha256.Sum256([]byte(browserID + "\x00" + directory))
	return value + "-" + hex.EncodeToString(digest[:4])
}

func RecentSites(ctx context.Context, profile Profile, since time.Time, limit int) ([]Site, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("site limit must be positive")
	}
	const chromeEpochMicros = int64(11_644_473_600_000_000)
	cutoff := since.UnixMicro() + chromeEpochMicros
	query := fmt.Sprintf(`SELECT u.url, COUNT(*) AS visits, MAX(v.visit_time) AS last_visit FROM visits v JOIN urls u ON u.id = v.url WHERE v.visit_time >= %d AND (u.url LIKE 'http://%%' OR u.url LIKE 'https://%%') GROUP BY u.url ORDER BY visits DESC, last_visit DESC LIMIT 10000`, cutoff)
	data, err := sqliteSnapshotJSON(ctx, filepath.Join(profile.Path, "History"), query)
	if err != nil {
		return nil, fmt.Errorf("read recent browser history: %w", err)
	}
	var rows []struct {
		URL       string `json:"url"`
		Visits    int    `json:"visits"`
		LastVisit int64  `json:"last_visit"`
	}
	if len(bytes.TrimSpace(data)) != 0 {
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, fmt.Errorf("decode recent browser history: %w", err)
		}
	}

	byDomain := make(map[string]Site)
	for _, row := range rows {
		parsed, err := url.Parse(row.URL)
		if err != nil || parsed.Hostname() == "" {
			continue
		}
		domain, err := CanonicalSite(parsed.Hostname())
		if err != nil {
			continue
		}
		site := byDomain[domain]
		site.Domain = domain
		site.Visits += row.Visits
		visited := chromiumTime(row.LastVisit)
		if visited.After(site.LastVisited) {
			site.LastVisited = visited
		}
		byDomain[domain] = site
	}
	sites := make([]Site, 0, len(byDomain))
	for _, site := range byDomain {
		sites = append(sites, site)
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].Visits == sites[j].Visits {
			return sites[i].LastVisited.After(sites[j].LastVisited)
		}
		return sites[i].Visits > sites[j].Visits
	})
	if len(sites) > limit {
		sites = sites[:limit]
	}
	return sites, nil
}

func ExportCookies(ctx context.Context, profile Profile, selectedSites []string) ([]Cookie, error) {
	canonicalSites := make([]string, 0, len(selectedSites))
	seenSites := make(map[string]struct{}, len(selectedSites))
	for _, selected := range selectedSites {
		site, err := CanonicalSite(selected)
		if err != nil {
			return nil, err
		}
		if _, exists := seenSites[site]; !exists {
			seenSites[site] = struct{}{}
			canonicalSites = append(canonicalSites, site)
		}
	}
	selectedSites = canonicalSites
	snapshot, whereClause, cleanup, err := cookieDatabaseSnapshot(ctx, profile)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	selectionCTE, siteClause := selectedCookieFilter(selectedSites, true)
	query := selectionCTE + `SELECT host_key, path, name, value, hex(encrypted_value) AS encrypted_value, expires_utc, is_httponly, is_secure, samesite FROM cookies` + whereClause + siteClause + ` ORDER BY host_key, name`
	data, err := sqliteJSON(ctx, snapshot, query)
	if err != nil {
		return nil, fmt.Errorf("read browser cookies: %w", err)
	}
	var rows []struct {
		Domain       string `json:"host_key"`
		Path         string `json:"path"`
		Name         string `json:"name"`
		Value        string `json:"value"`
		EncryptedHex string `json:"encrypted_value"`
		Expires      int64  `json:"expires_utc"`
		HTTPOnly     int    `json:"is_httponly"`
		Secure       int    `json:"is_secure"`
		SameSite     int    `json:"samesite"`
	}
	if len(bytes.TrimSpace(data)) != 0 {
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, fmt.Errorf("decode browser cookies: %w", err)
		}
	}
	var key []byte
	cookies := make([]Cookie, 0)
	for _, row := range rows {
		if len(selectedSites) == 0 {
			if _, err := CanonicalSite(row.Domain); err != nil {
				continue
			}
		}
		if !matchesAnySite(row.Domain, selectedSites) {
			continue
		}
		value := row.Value
		if value == "" && row.EncryptedHex != "" {
			if key == nil {
				key, err = chromiumSafeStorageKey(ctx, profile.Browser)
				if err != nil {
					return nil, err
				}
			}
			encrypted, err := hex.DecodeString(row.EncryptedHex)
			if err != nil {
				return nil, fmt.Errorf("decode cookie %s: %w", row.Name, err)
			}
			decrypted, err := decryptChromiumValue(key, row.Domain, encrypted)
			if err != nil {
				return nil, fmt.Errorf("decrypt cookie %s: %w", row.Name, err)
			}
			value = string(decrypted)
		}
		cookie := Cookie{Domain: row.Domain, Path: row.Path, Name: row.Name, Value: value, HTTPOnly: row.HTTPOnly != 0, Secure: row.Secure != 0, SameSite: chromiumSameSite(row.SameSite, row.Secure != 0)}
		if row.Expires > 0 {
			expires := chromiumTime(row.Expires)
			cookie.ExpiresAt = &expires
		}
		cookies = append(cookies, cookie)
	}
	return cookies, nil
}

func ExportBookmarks(profile Profile) (BookmarkDocument, int, error) {
	payload, err := readFileBounded(filepath.Join(profile.Path, "Bookmarks"), maxDocumentBytes)
	if errors.Is(err, os.ErrNotExist) {
		return BookmarkDocument{}, 0, nil
	}
	if err != nil {
		return BookmarkDocument{}, 0, fmt.Errorf("read browser bookmarks: %w", err)
	}
	var source struct {
		Roots map[string]chromiumBookmarkNode `json:"roots"`
	}
	if err := json.Unmarshal(payload, &source); err != nil {
		return BookmarkDocument{}, 0, fmt.Errorf("decode browser bookmarks: %w", err)
	}
	document := BookmarkDocument{Roots: make([]BookmarkRoot, 0, 3)}
	count := 0
	for _, name := range []string{"bookmark_bar", "other", "synced"} {
		root, ok := source.Roots[name]
		if !ok {
			continue
		}
		portableName := name
		if name == "synced" {
			portableName = "mobile"
		}
		document.Roots = append(document.Roots, BookmarkRoot{Name: portableName, Children: portableBookmarkNodes(root.Children, &count)})
	}
	return document, count, nil
}

type chromiumBookmarkNode struct {
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	URL          string                 `json:"url"`
	DateAdded    string                 `json:"date_added"`
	DateLastUsed string                 `json:"date_last_used"`
	Children     []chromiumBookmarkNode `json:"children"`
}

func portableBookmarkNodes(nodes []chromiumBookmarkNode, count *int) []BookmarkNode {
	result := make([]BookmarkNode, 0, len(nodes))
	for _, node := range nodes {
		if node.Type == "url" {
			parsed, err := url.Parse(node.URL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				continue
			}
			*count++
			result = append(result, BookmarkNode{Title: node.Name, URL: node.URL, DateAdded: chromiumTimestamp(node.DateAdded), DateLastUsed: chromiumTimestamp(node.DateLastUsed)})
			continue
		}
		if node.Type != "folder" {
			continue
		}
		children := portableBookmarkNodes(node.Children, count)
		*count++
		result = append(result, BookmarkNode{Title: node.Name, Children: children, DateAdded: chromiumTimestamp(node.DateAdded)})
	}
	return result
}

func chromiumTimestamp(value string) *time.Time {
	microseconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || microseconds <= 0 {
		return nil
	}
	parsed := chromiumTime(microseconds)
	return &parsed
}

func ExportHistory(ctx context.Context, profile Profile, since time.Time) ([]HistoryRecord, error) {
	const chromeEpochMicros = int64(11_644_473_600_000_000)
	cutoff := since.UnixMicro() + chromeEpochMicros
	query := fmt.Sprintf(`SELECT urls.url, urls.title, MAX(visits.visit_time) AS last_visit_time, COUNT(*) AS visit_count FROM visits JOIN urls ON urls.id = visits.url WHERE visits.visit_time >= %d AND (urls.url LIKE 'http://%%' OR urls.url LIKE 'https://%%') GROUP BY urls.id ORDER BY last_visit_time DESC LIMIT 100001`, cutoff)
	payload, err := sqliteSnapshotJSON(ctx, filepath.Join(profile.Path, "History"), query)
	if err != nil {
		return nil, fmt.Errorf("read browser history: %w", err)
	}
	var rows []struct {
		URL       string `json:"url"`
		Title     string `json:"title"`
		VisitedAt int64  `json:"last_visit_time"`
		Count     int    `json:"visit_count"`
	}
	if len(bytes.TrimSpace(payload)) != 0 {
		if err := json.Unmarshal(payload, &rows); err != nil {
			return nil, fmt.Errorf("decode browser history: %w", err)
		}
	}
	if len(rows) > 100000 {
		return nil, fmt.Errorf("browser history exceeds the 100000-record import limit; use fewer days")
	}
	records := make([]HistoryRecord, 0, len(rows))
	for _, row := range rows {
		parsed, err := url.Parse(row.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			continue
		}
		records = append(records, HistoryRecord{URL: row.URL, Title: row.Title, VisitedAt: chromiumTime(row.VisitedAt), VisitCount: max(1, row.Count)})
	}
	return records, nil
}

func HistoryCount(ctx context.Context, profile Profile, since time.Time) (int, error) {
	const chromeEpochMicros = int64(11_644_473_600_000_000)
	cutoff := since.UnixMicro() + chromeEpochMicros
	query := fmt.Sprintf(`SELECT COUNT(*) AS count FROM visits JOIN urls ON urls.id = visits.url WHERE visits.visit_time >= %d AND (urls.url LIKE 'http://%%' OR urls.url LIKE 'https://%%')`, cutoff)
	payload, err := sqliteSnapshotJSON(ctx, filepath.Join(profile.Path, "History"), query)
	if err != nil {
		return 0, fmt.Errorf("count browser history: %w", err)
	}
	var rows []struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(payload, &rows); err != nil {
		return 0, fmt.Errorf("decode browser history count: %w", err)
	}
	if len(rows) != 1 || rows[0].Count < 0 {
		return 0, fmt.Errorf("browser history count is invalid")
	}
	return rows[0].Count, nil
}

func LocalStorageSites(ctx context.Context, profile Profile) ([]StorageSite, error) {
	database, cleanup, err := levelDBSnapshot(ctx, filepath.Join(profile.Path, "Local Storage", "leveldb"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("snapshot browser local storage: %w", err)
	}
	defer cleanup()

	db, err := leveldb.OpenFile(database, nil)
	if err != nil {
		return nil, fmt.Errorf("open browser local storage: %w", err)
	}
	defer db.Close()

	metadataBytes := make(map[string]int64)
	recordBytes := make(map[string]int64)
	iterator := db.NewIterator(nil, nil)
	defer iterator.Release()
	for iterator.Next() {
		key := iterator.Key()
		if bytes.HasPrefix(key, []byte("META:")) {
			origin, ok := portableStorageOrigin(string(key[len("META:"):]))
			if !ok {
				continue
			}
			size, err := localStorageMetadataSize(iterator.Value())
			if err != nil {
				return nil, fmt.Errorf("decode local storage metadata for %s: %w", origin, err)
			}
			metadataBytes[origin] = size
			continue
		}
		if len(key) < 2 || key[0] != '_' {
			continue
		}
		separator := bytes.IndexByte(key[1:], 0)
		if separator < 1 {
			continue
		}
		origin, ok := portableStorageOrigin(string(key[1 : 1+separator]))
		if ok {
			recordBytes[origin] += int64(len(key[2+separator:]) + len(iterator.Value()))
		}
	}
	if err := iterator.Error(); err != nil {
		return nil, fmt.Errorf("read browser local storage metadata: %w", err)
	}
	sites := make([]StorageSite, 0, len(metadataBytes)+len(recordBytes))
	for origin, bytes := range recordBytes {
		if metadataBytes[origin] > bytes {
			bytes = metadataBytes[origin]
		}
		sites = append(sites, StorageSite{Origin: origin, Bytes: bytes})
		delete(metadataBytes, origin)
	}
	for origin, bytes := range metadataBytes {
		sites = append(sites, StorageSite{Origin: origin, Bytes: bytes})
	}
	sort.Slice(sites, func(left, right int) bool { return sites[left].Origin < sites[right].Origin })
	return sites, nil
}

func ExportLocalStorage(ctx context.Context, profile Profile, selectedOrigins []string) (StorageExport, error) {
	database, cleanup, err := levelDBSnapshot(ctx, filepath.Join(profile.Path, "Local Storage", "leveldb"))
	if errors.Is(err, os.ErrNotExist) {
		return StorageExport{}, nil
	}
	if err != nil {
		return StorageExport{}, fmt.Errorf("snapshot browser local storage: %w", err)
	}
	defer cleanup()

	db, err := leveldb.OpenFile(database, nil)
	if err != nil {
		return StorageExport{}, fmt.Errorf("open browser local storage: %w", err)
	}
	defer db.Close()

	selected := make(map[string]struct{}, len(selectedOrigins))
	for _, origin := range selectedOrigins {
		selected[origin] = struct{}{}
	}
	records := make([]StorageRecord, 0)
	skippedOrigins := make(map[string]struct{})
	skippedRecords := 0
	encodedBytes := 0
	iterator := db.NewIterator(util.BytesPrefix([]byte("_")), nil)
	defer iterator.Release()
	for iterator.Next() {
		key := iterator.Key()[1:]
		separator := bytes.IndexByte(key, 0)
		if separator < 1 || separator == len(key)-1 {
			continue
		}
		origin, ok := portableStorageOrigin(string(key[:separator]))
		if !ok {
			continue
		}
		if len(selected) > 0 {
			if _, ok := selected[origin]; !ok {
				continue
			}
		}
		scriptKey, err := decodeChromiumStorageString(key[separator+1:])
		if err != nil || scriptKey == "" {
			continue
		}
		value, err := decodeChromiumStorageString(iterator.Value())
		if err != nil {
			continue
		}
		record := StorageRecord{Origin: origin, Kind: StorageKindLocal, Key: scriptKey, Value: value}
		encoded, err := json.Marshal(record)
		if err != nil {
			return StorageExport{}, fmt.Errorf("encode browser local storage: %w", err)
		}
		if len(encoded)+1 > maxStorageRecord {
			skippedRecords++
			skippedOrigins[origin] = struct{}{}
			continue
		}
		encodedBytes += len(encoded) + 1
		if encodedBytes > maxStorageBytes {
			return StorageExport{}, fmt.Errorf("browser local storage exceeds the 64 MiB import limit; choose fewer websites")
		}
		records = append(records, record)
		if len(records) > maxStorageCount {
			return StorageExport{}, fmt.Errorf("browser local storage exceeds the 100000-record import limit; choose fewer websites")
		}
	}
	if err := iterator.Error(); err != nil {
		return StorageExport{}, fmt.Errorf("read browser local storage: %w", err)
	}
	sort.Slice(records, func(left, right int) bool {
		if records[left].Origin == records[right].Origin {
			return records[left].Key < records[right].Key
		}
		return records[left].Origin < records[right].Origin
	})
	return StorageExport{Records: records, SkippedRecords: skippedRecords, SkippedOrigins: len(skippedOrigins)}, nil
}

func portableStorageOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	origin := parsed.Scheme + "://" + parsed.Host
	if raw != origin && raw != origin+"/" {
		return "", false
	}
	return origin, true
}

func decodeChromiumStorageString(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("string is missing its encoding prefix")
	}
	switch raw[0] {
	case 0:
		if len(raw[1:])%2 != 0 {
			return "", fmt.Errorf("UTF-16 string has an odd byte length")
		}
		units := make([]uint16, len(raw[1:])/2)
		for index := range units {
			units[index] = binary.LittleEndian.Uint16(raw[1+index*2:])
		}
		return string(utf16.Decode(units)), nil
	case 1:
		runes := make([]rune, len(raw)-1)
		for index, value := range raw[1:] {
			runes[index] = rune(value)
		}
		return string(runes), nil
	default:
		return "", fmt.Errorf("unsupported string encoding prefix %d", raw[0])
	}
}

func localStorageMetadataSize(raw []byte) (int64, error) {
	for len(raw) > 0 {
		tag, read := binary.Uvarint(raw)
		if read <= 0 {
			return 0, fmt.Errorf("invalid protobuf tag")
		}
		raw = raw[read:]
		field, wire := tag>>3, tag&7
		if wire != 0 {
			return 0, fmt.Errorf("unsupported protobuf wire type %d", wire)
		}
		value, read := binary.Uvarint(raw)
		if read <= 0 {
			return 0, fmt.Errorf("invalid protobuf value")
		}
		raw = raw[read:]
		if field == 2 {
			if value > uint64(^uint64(0)>>1) {
				return 0, fmt.Errorf("size exceeds int64")
			}
			return int64(value), nil
		}
	}
	return 0, fmt.Errorf("size field is missing")
}

// CookieSites returns every website with importable cookies, ranked by recent
// browser use. It reads cookie metadata only; values are decrypted by
// ExportCookies after the user chooses what to import.
func CookieSites(ctx context.Context, profile Profile, recent []Site) ([]Site, error) {
	snapshot, whereClause, cleanup, err := cookieDatabaseSnapshot(ctx, profile)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	data, err := sqliteJSON(ctx, snapshot, `SELECT host_key, COUNT(*) AS cookie_count FROM cookies`+whereClause+` GROUP BY host_key`)
	if err != nil {
		return nil, fmt.Errorf("find browser cookie websites: %w", err)
	}
	var rows []struct {
		Domain      string `json:"host_key"`
		CookieCount int    `json:"cookie_count"`
	}
	if len(bytes.TrimSpace(data)) != 0 {
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, fmt.Errorf("decode browser cookie websites: %w", err)
		}
	}
	byDomain := make(map[string]Site, len(rows))
	for _, site := range recent {
		byDomain[site.Domain] = site
	}
	for _, row := range rows {
		domain, err := CanonicalSite(row.Domain)
		if err != nil {
			continue
		}
		site := byDomain[domain]
		site.Domain = domain
		site.CookieCount += row.CookieCount
		byDomain[domain] = site
	}
	result := make([]Site, 0, len(byDomain))
	for _, site := range byDomain {
		if site.CookieCount > 0 {
			result = append(result, site)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Visits != result[j].Visits {
			return result[i].Visits > result[j].Visits
		}
		if !result[i].LastVisited.Equal(result[j].LastVisited) {
			return result[i].LastVisited.After(result[j].LastVisited)
		}
		return result[i].Domain < result[j].Domain
	})
	return result, nil
}

func cookieDatabaseSnapshot(ctx context.Context, profile Profile) (string, string, func(), error) {
	path := filepath.Join(profile.Path, "Network/Cookies")
	if _, err := os.Stat(path); err != nil {
		path = filepath.Join(profile.Path, "Cookies")
	}
	snapshot, cleanup, err := sqliteSnapshot(ctx, path)
	if err != nil {
		return "", "", nil, fmt.Errorf("snapshot cookie database: %w", err)
	}
	columnsData, err := sqliteJSON(ctx, snapshot, `SELECT name FROM pragma_table_info('cookies')`)
	if err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("inspect cookie database: %w", err)
	}
	var columns []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(columnsData, &columns); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("decode cookie schema: %w", err)
	}
	predicates := []string{"(expires_utc = 0 OR expires_utc > " + fmt.Sprintf("%d", time.Now().UnixMicro()+11_644_473_600_000_000) + ")"}
	for _, column := range columns {
		switch column.Name {
		case "top_frame_site_key":
			predicates = append(predicates, "top_frame_site_key = ''")
		case "is_partitioned":
			predicates = append(predicates, "is_partitioned = 0")
		}
	}
	return snapshot, " WHERE " + strings.Join(predicates, " AND "), cleanup, nil
}

func sqliteJSON(ctx context.Context, databasePath, query string) ([]byte, error) {
	command := exec.CommandContext(ctx, "/usr/bin/sqlite3", "-json", databasePath, query)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxSQLiteOutput+1))
	if int64(len(output)) > maxSQLiteOutput {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("sqlite output exceeds 64 MiB")
	}
	waitErr := command.Wait()
	if readErr != nil {
		return nil, readErr
	}
	err = waitErr
	if err != nil {
		return nil, fmt.Errorf("sqlite3: %s", strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

func sqliteSnapshotJSON(ctx context.Context, databasePath, query string) ([]byte, error) {
	snapshot, cleanup, err := sqliteSnapshot(ctx, databasePath)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return sqliteJSON(ctx, snapshot, query)
}

type fileFingerprint struct {
	exists   bool
	size     int64
	modified time.Time
}

type directoryFingerprint struct {
	name     string
	size     int64
	modified time.Time
}

func levelDBSnapshot(ctx context.Context, sourceDirectory string) (string, func(), error) {
	if _, err := os.Stat(sourceDirectory); err != nil {
		return "", nil, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		before, err := fingerprintLevelDB(sourceDirectory)
		if err != nil {
			return "", nil, err
		}
		var total int64
		for _, file := range before {
			total += file.size
		}
		if total > maxStorageSource {
			return "", nil, fmt.Errorf("browser local storage source exceeds 512 MiB")
		}
		root, err := os.MkdirTemp("", "kernel-browser-import-leveldb-")
		if err != nil {
			return "", nil, err
		}
		destination := filepath.Join(root, "leveldb")
		if err := os.Mkdir(destination, 0o700); err != nil {
			_ = os.RemoveAll(root)
			return "", nil, err
		}
		changed := false
		for _, file := range before {
			err := copyFileBounded(ctx, filepath.Join(sourceDirectory, file.name), filepath.Join(destination, file.name), file.size)
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, errFileGrew) {
				changed = true
				break
			}
			if err != nil {
				_ = os.RemoveAll(root)
				return "", nil, fmt.Errorf("copy local storage %s: %w", file.name, err)
			}
		}
		after, err := fingerprintLevelDB(sourceDirectory)
		if !changed && err == nil && directoryFingerprintsEqual(before, after) {
			return destination, func() { _ = os.RemoveAll(root) }, nil
		}
		_ = os.RemoveAll(root)
	}
	return "", nil, fmt.Errorf("browser local storage changed while it was being read; try again")
}

func fingerprintLevelDB(directory string) ([]directoryFingerprint, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	result := make([]directoryFingerprint, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "LOCK" || entry.Name() == "LOG" || entry.Name() == "LOG.old" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		result = append(result, directoryFingerprint{name: entry.Name(), size: info.Size(), modified: info.ModTime()})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].name < result[right].name })
	return result, nil
}

func directoryFingerprintsEqual(left, right []directoryFingerprint) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sqliteSnapshot(ctx context.Context, databasePath string) (string, func(), error) {
	if !fileExists(databasePath) {
		return "", nil, fmt.Errorf("browser database %q was not found", filepath.Base(databasePath))
	}
	paths := []string{databasePath, databasePath + "-wal", databasePath + "-journal"}
	for attempt := 0; attempt < 3; attempt++ {
		before, err := fingerprintFiles(paths)
		if err != nil {
			return "", nil, err
		}
		var totalBytes int64
		for _, fingerprint := range before {
			totalBytes += fingerprint.size
		}
		if totalBytes > maxSQLiteBytes {
			return "", nil, fmt.Errorf("browser database snapshot exceeds %d bytes", maxSQLiteBytes)
		}
		temporaryDirectory, err := os.MkdirTemp("", "kernel-browser-import-sqlite-")
		if err != nil {
			return "", nil, err
		}
		snapshot := filepath.Join(temporaryDirectory, filepath.Base(databasePath))
		copyChanged := false
		for index, source := range paths {
			if !before[index].exists {
				continue
			}
			if err := copyFileBounded(ctx, source, snapshot+strings.TrimPrefix(source, databasePath), before[index].size); err != nil {
				os.RemoveAll(temporaryDirectory)
				if errors.Is(err, os.ErrNotExist) || errors.Is(err, errFileGrew) {
					copyChanged = true
					break
				}
				return "", nil, fmt.Errorf("copy %s: %w", filepath.Base(source), err)
			}
		}
		after, err := fingerprintFiles(paths)
		if !copyChanged && err == nil && fingerprintsEqual(before, after) {
			return snapshot, func() { _ = os.RemoveAll(temporaryDirectory) }, nil
		}
		os.RemoveAll(temporaryDirectory)
	}
	return "", nil, fmt.Errorf("browser database changed while it was being read; try again")
}

func fingerprintFiles(paths []string) ([]fileFingerprint, error) {
	result := make([]fileFingerprint, len(paths))
	for index, path := range paths {
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result[index] = fileFingerprint{exists: true, size: info.Size(), modified: info.ModTime()}
	}
	return result, nil
}

func fingerprintsEqual(left, right []fileFingerprint) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func copyFileBounded(ctx context.Context, source, destination string, limit int64) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer output.Close()

	buffer := make([]byte, 256<<10)
	var copied int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		read, readErr := input.Read(buffer)
		if read > 0 {
			copied += int64(read)
			if copied > limit {
				return errFileGrew
			}
			if _, err := output.Write(buffer[:read]); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return output.Close()
		}
		if readErr != nil {
			return readErr
		}
	}
}

var errFileGrew = errors.New("file grew while being copied")

func selectedCookieFilter(sites []string, hasWhere bool) (string, string) {
	if len(sites) == 0 {
		return "", ""
	}
	values := make([]string, 0, len(sites))
	for _, site := range sites {
		site = strings.TrimPrefix(strings.ToLower(site), ".")
		values = append(values, "("+sqliteLiteral(site)+")")
	}
	prefix := " WHERE "
	if hasWhere {
		prefix = " AND "
	}
	cte := "WITH selected_sites(site) AS (VALUES " + strings.Join(values, ",") + ") "
	predicate := prefix + "EXISTS (SELECT 1 FROM selected_sites WHERE host_key = site OR host_key = '.' || site OR host_key LIKE '%.' || site)"
	return cte, predicate
}

func sqliteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func chromiumSafeStorageKey(ctx context.Context, browser Browser) ([]byte, error) {
	command := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-w", "-s", browser.Keychain.Service, "-a", browser.Keychain.Account)
	password, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("allow %s cookie access in macOS Keychain", browser.Name)
	}
	return pbkdf2.Key([]byte(strings.TrimSpace(string(password))), []byte("saltysalt"), 1003, 16, sha1.New), nil
}

func decryptChromiumValue(key []byte, domain string, encrypted []byte) ([]byte, error) {
	if len(encrypted) < 3 || (string(encrypted[:3]) != "v10" && string(encrypted[:3]) != "v11") {
		return nil, fmt.Errorf("unsupported encrypted value format")
	}
	ciphertext := encrypted[3:]
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("encrypted value has invalid length")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, bytes.Repeat([]byte{' '}, aes.BlockSize)).CryptBlocks(plaintext, ciphertext)
	padding := int(plaintext[len(plaintext)-1])
	if padding == 0 || padding > aes.BlockSize || padding > len(plaintext) {
		return nil, fmt.Errorf("encrypted value has invalid padding")
	}
	for _, value := range plaintext[len(plaintext)-padding:] {
		if int(value) != padding {
			return nil, fmt.Errorf("encrypted value has invalid padding")
		}
	}
	plaintext = plaintext[:len(plaintext)-padding]
	hostDigest := sha256.Sum256([]byte(domain))
	if len(plaintext) >= len(hostDigest) && bytes.Equal(plaintext[:len(hostDigest)], hostDigest[:]) {
		plaintext = plaintext[len(hostDigest):]
	}
	return plaintext, nil
}

// CanonicalSite validates a website hostname and returns its registrable domain.
func CanonicalSite(value string) (string, error) {
	value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), ".")
	if value == "" || strings.ContainsAny(value, "/:*@?#[] ") {
		return "", fmt.Errorf("%q is not a website domain", value)
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(value)
	if err != nil {
		return "", fmt.Errorf("%q is not a registrable website domain", value)
	}
	return domain, nil
}

func matchesAnySite(cookieDomain string, sites []string) bool {
	if len(sites) == 0 {
		return true
	}
	for _, site := range sites {
		if domainMatchesSite(cookieDomain, site) {
			return true
		}
	}
	return false
}

func domainMatchesSite(cookieDomain, site string) bool {
	cookieDomain = strings.TrimPrefix(strings.ToLower(cookieDomain), ".")
	site = strings.ToLower(site)
	return cookieDomain == site || strings.HasSuffix(cookieDomain, "."+site) || strings.HasSuffix(site, "."+cookieDomain)
}

func chromiumTime(microseconds int64) time.Time {
	const chromiumToUnixMicroseconds = int64(11_644_473_600_000_000)
	return time.UnixMicro(microseconds - chromiumToUnixMicroseconds).UTC()
}

func chromiumSameSite(value int, secure bool) string {
	switch value {
	case 1:
		return "lax"
	case 2:
		return "strict"
	case 0:
		if secure {
			return "none"
		}
	}
	return ""
}

func readFileBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return data, nil
}
