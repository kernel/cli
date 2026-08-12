package browserimport

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/sha256"
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
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/net/publicsuffix"
)

const (
	maxDocumentBytes = 16 << 20
	maxSQLiteOutput  = 64 << 20
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
		if _, err := os.Stat(filepath.Join(path, "History")); err != nil {
			continue
		}
		profiles = append(profiles, Profile{ID: profileID(browser.ID, directory), Name: names[directory], Browser: browser, Path: path})
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
	data, err := sqliteJSON(ctx, filepath.Join(profile.Path, "History"), query)
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
		domain := registrableDomain(parsed.Hostname())
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
	path := filepath.Join(profile.Path, "Network/Cookies")
	if _, err := os.Stat(path); err != nil {
		path = filepath.Join(profile.Path, "Cookies")
	}
	columnsData, err := sqliteJSON(ctx, path, `SELECT name FROM pragma_table_info('cookies')`)
	if err != nil {
		return nil, fmt.Errorf("inspect cookie database: %w", err)
	}
	var columns []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(columnsData, &columns); err != nil {
		return nil, fmt.Errorf("decode cookie schema: %w", err)
	}
	partitionClause := ""
	for _, column := range columns {
		switch column.Name {
		case "top_frame_site_key":
			partitionClause = " WHERE top_frame_site_key = ''"
		case "is_partitioned":
			if partitionClause == "" {
				partitionClause = " WHERE is_partitioned = 0"
			}
		}
	}
	query := `SELECT host_key, path, name, value, hex(encrypted_value) AS encrypted_value, expires_utc, is_httponly, is_secure, samesite FROM cookies` + partitionClause + ` ORDER BY host_key, name`
	data, err := sqliteJSON(ctx, path, query)
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

func CountCookiesBySite(cookies []Cookie, sites []Site) []Site {
	result := append([]Site(nil), sites...)
	for i := range result {
		for _, cookie := range cookies {
			if domainMatchesSite(cookie.Domain, result[i].Domain) {
				result[i].CookieCount++
			}
		}
	}
	return result
}

func sqliteJSON(ctx context.Context, databasePath, query string) ([]byte, error) {
	command := exec.CommandContext(ctx, "/usr/bin/sqlite3", "-readonly", "-json", databasePath, query)
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("sqlite3: %s", strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, err
	}
	if len(output) > maxSQLiteOutput {
		return nil, fmt.Errorf("sqlite output exceeds 64 MiB")
	}
	return output, nil
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

func registrableDomain(host string) string {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	domain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err == nil {
		return domain
	}
	return host
}

func matchesAnySite(cookieDomain string, sites []string) bool {
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
