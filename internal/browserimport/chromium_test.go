package browserimport

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
)

func TestDiscoverMacOSProfilesFindsChromeAndHelium(t *testing.T) {
	home := t.TempDir()
	writeBrowserFixture(t, home, "Google/Chrome", "Default", "Personal")
	writeBrowserFixture(t, home, "net.imput.helium", "Profile 1", "Work")

	profiles, err := DiscoverMacOSProfiles(home)
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	assert.Equal(t, "Google Chrome / Personal", profiles[0].DisplayName())
	assert.Equal(t, "Helium / Work", profiles[1].DisplayName())
	assert.NotEqual(t, profiles[0].ID, profiles[1].ID)
}

func TestDomainMatchesSelectedSite(t *testing.T) {
	assert.True(t, domainMatchesSite(".accounts.google.com", "google.com"))
	assert.True(t, domainMatchesSite("google.com", "accounts.google.com"))
	assert.False(t, domainMatchesSite("notgoogle.com", "google.com"))
}

func TestCanonicalSiteUsesRegistrableDomain(t *testing.T) {
	domain, err := CanonicalSite("accounts.Google.com")
	require.NoError(t, err)
	assert.Equal(t, "google.com", domain)
	_, err = CanonicalSite("com")
	assert.Error(t, err)
	_, err = CanonicalSite("https://google.com")
	assert.Error(t, err)
}

func TestDecryptChromiumValueSupportsHostDigest(t *testing.T) {
	key := []byte("0123456789abcdef")
	domain := ".example.com"
	digest := sha256.Sum256([]byte(domain))
	plaintext := append(digest[:], []byte("secret")...)
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	plaintext = append(plaintext, bytesRepeat(byte(padding), padding)...)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, bytesRepeat(' ', aes.BlockSize)).CryptBlocks(ciphertext, plaintext)

	decrypted, err := decryptChromiumValue(key, domain, append([]byte("v10"), ciphertext...))
	require.NoError(t, err)
	assert.Equal(t, "secret", string(decrypted))
}

func TestRecentSitesRanksDomainsFromWALDatabase(t *testing.T) {
	profile := sqliteProfileFixture(t)
	database := filepath.Join(profile.Path, "History")
	runSQLite(t, database, `
PRAGMA journal_mode=WAL;
CREATE TABLE urls (id INTEGER PRIMARY KEY, url TEXT);
CREATE TABLE visits (id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER);
INSERT INTO urls VALUES (1, 'https://github.com/a'), (2, 'https://www.github.com/b'), (3, 'https://example.com');
INSERT INTO visits VALUES (1, 1, 13400000000000000), (2, 2, 13400000001000000), (3, 3, 13400000002000000);
`)

	sites, err := RecentSites(context.Background(), profile, time.Unix(0, 0), 5)
	require.NoError(t, err)
	require.Len(t, sites, 2)
	assert.Equal(t, "github.com", sites[0].Domain)
	assert.Equal(t, 2, sites[0].Visits)
	assert.Equal(t, "example.com", sites[1].Domain)
}

func TestRecentSitesOmitsNonPortableLocalHosts(t *testing.T) {
	profile := sqliteProfileFixture(t)
	database := filepath.Join(profile.Path, "History")
	runSQLite(t, database, `
CREATE TABLE urls (id INTEGER PRIMARY KEY, url TEXT);
CREATE TABLE visits (id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER);
INSERT INTO urls VALUES (1, 'http://localhost:3000'), (2, 'https://github.com/kernel');
INSERT INTO visits VALUES (1, 1, 13400000000000000), (2, 2, 13400000001000000);
`)

	sites, err := RecentSites(context.Background(), profile, time.Unix(0, 0), 5)
	require.NoError(t, err)
	require.Len(t, sites, 1)
	assert.Equal(t, "github.com", sites[0].Domain)
}

func TestRecentSitesSnapshotsLockedDatabase(t *testing.T) {
	profile := sqliteProfileFixture(t)
	database := filepath.Join(profile.Path, "History")
	runSQLite(t, database, `
PRAGMA journal_mode=WAL;
CREATE TABLE urls (id INTEGER PRIMARY KEY, url TEXT);
CREATE TABLE visits (id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER);
INSERT INTO urls VALUES (1, 'https://github.com/kernel');
INSERT INTO visits VALUES (1, 1, 13400000000000000);
`)

	locker := exec.Command("/usr/bin/sqlite3", database)
	stdin, err := locker.StdinPipe()
	require.NoError(t, err)
	stdout, err := locker.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, locker.Start())
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = locker.Process.Kill()
		_ = locker.Wait()
	})
	_, err = stdin.Write([]byte("PRAGMA locking_mode=EXCLUSIVE; BEGIN EXCLUSIVE;\n.print READY\n"))
	require.NoError(t, err)
	require.True(t, bufio.NewScanner(stdout).Scan())

	direct := exec.Command("/usr/bin/sqlite3", "-readonly", database, "SELECT COUNT(*) FROM visits")
	require.Error(t, direct.Run(), "fixture must prove the source database is locked")
	sites, err := RecentSites(context.Background(), profile, time.Unix(0, 0), 5)
	require.NoError(t, err)
	require.Len(t, sites, 1)
	assert.Equal(t, "github.com", sites[0].Domain)
}

func TestRecentSitesSnapshotsRollbackJournal(t *testing.T) {
	profile := sqliteProfileFixture(t)
	database := filepath.Join(profile.Path, "History")
	runSQLite(t, database, `
CREATE TABLE urls (id INTEGER PRIMARY KEY, url TEXT);
CREATE TABLE visits (id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER);
INSERT INTO urls VALUES (1, 'https://github.com/kernel');
INSERT INTO visits VALUES (1, 1, 13400000000000000);
`)

	locker := exec.Command("/usr/bin/sqlite3", database)
	stdin, err := locker.StdinPipe()
	require.NoError(t, err)
	stdout, err := locker.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, locker.Start())
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = locker.Process.Kill()
		_ = locker.Wait()
	})
	_, err = stdin.Write([]byte("PRAGMA journal_mode=DELETE; PRAGMA locking_mode=EXCLUSIVE; BEGIN EXCLUSIVE; UPDATE urls SET url = 'https://uncommitted.example';\n.print READY\n"))
	require.NoError(t, err)
	require.True(t, bufio.NewScanner(stdout).Scan())
	require.FileExists(t, database+"-journal")

	sites, err := RecentSites(context.Background(), profile, time.Unix(0, 0), 5)
	require.NoError(t, err)
	require.Len(t, sites, 1)
	assert.Equal(t, "github.com", sites[0].Domain, "the copied hot journal must roll back the uncommitted page")
}

func TestExportCookiesFiltersSitesAndPartitionedRows(t *testing.T) {
	profile := sqliteProfileFixture(t)
	database := filepath.Join(profile.Path, "Network", "Cookies")
	require.NoError(t, os.MkdirAll(filepath.Dir(database), 0o755))
	runSQLite(t, database, `
CREATE TABLE cookies (
  host_key TEXT, path TEXT, name TEXT, value TEXT, encrypted_value BLOB,
  expires_utc INTEGER, is_httponly INTEGER, is_secure INTEGER, samesite INTEGER,
  top_frame_site_key TEXT
);
INSERT INTO cookies VALUES
  ('.github.com', '/', 'session', 'selected', X'', 0, 1, 1, 1, ''),
  ('.github.com', '/', 'partitioned', 'exclude', X'', 13400000000000000, 0, 1, 0, 'https://top.example'),
  ('.github.com', '/', 'expired', 'exclude', X'', 13400000000000000, 0, 1, 0, ''),
  ('.unrelated.com', '/', 'other', 'exclude', X'', 13400000000000000, 0, 1, 0, '');
`)

	cookies, err := ExportCookies(context.Background(), profile, []string{"github.com"})
	require.NoError(t, err)
	require.Len(t, cookies, 1)
	assert.Equal(t, "session", cookies[0].Name)
	assert.Equal(t, "selected", cookies[0].Value)
	assert.Equal(t, "lax", cookies[0].SameSite)
}

func TestExportCookiesIncludesParentDomainForSelectedSubdomain(t *testing.T) {
	profile := sqliteProfileFixture(t)
	database := filepath.Join(profile.Path, "Network", "Cookies")
	require.NoError(t, os.MkdirAll(filepath.Dir(database), 0o755))
	runSQLite(t, database, `
CREATE TABLE cookies (
  host_key TEXT, path TEXT, name TEXT, value TEXT, encrypted_value BLOB,
  expires_utc INTEGER, is_httponly INTEGER, is_secure INTEGER, samesite INTEGER
);
INSERT INTO cookies VALUES ('.google.com', '/', 'parent', 'selected', X'', 0, 1, 1, 1);
`)

	cookies, err := ExportCookies(context.Background(), profile, []string{"accounts.google.com"})
	require.NoError(t, err)
	require.Len(t, cookies, 1)
	assert.Equal(t, "parent", cookies[0].Name)
}

func TestExportCookiesWithNoSiteFilterImportsAllEligibleCookies(t *testing.T) {
	profile := sqliteProfileFixture(t)
	database := filepath.Join(profile.Path, "Network", "Cookies")
	require.NoError(t, os.MkdirAll(filepath.Dir(database), 0o755))
	runSQLite(t, database, `
CREATE TABLE cookies (
  host_key TEXT, path TEXT, name TEXT, value TEXT, encrypted_value BLOB,
  expires_utc INTEGER, is_httponly INTEGER, is_secure INTEGER, samesite INTEGER
);
INSERT INTO cookies VALUES
  ('.github.com', '/', 'github', 'one', X'', 0, 1, 1, 1),
  ('.google.com', '/', 'google', 'two', X'', 0, 1, 1, 1),
  ('localhost', '/', 'local-only', 'skip', X'', 0, 1, 1, 1);
`)

	cookies, err := ExportCookies(t.Context(), profile, nil)
	require.NoError(t, err)
	require.Len(t, cookies, 2)
	assert.Equal(t, []string{"github", "google"}, []string{cookies[0].Name, cookies[1].Name})
}

func TestExportBookmarksNormalizesPortableRoots(t *testing.T) {
	profile := sqliteProfileFixture(t)
	payload := `{"roots":{"bookmark_bar":{"children":[{"type":"url","name":"Kernel","url":"https://onkernel.com","date_added":"13400000000000000"},{"type":"url","name":"Local","url":"chrome://settings"}]},"other":{"children":[{"type":"folder","name":"Work","children":[{"type":"url","name":"GitHub","url":"https://github.com"}]}]}}}`
	require.NoError(t, os.WriteFile(filepath.Join(profile.Path, "Bookmarks"), []byte(payload), 0o600))

	document, count, err := ExportBookmarks(profile)
	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.Len(t, document.Roots, 2)
	require.Equal(t, "https://onkernel.com", document.Roots[0].Children[0].URL)
	require.Len(t, document.Roots[0].Children, 1, "non-http bookmarks must not leave the laptop")
}

func TestExportHistoryUsesRequestedWindow(t *testing.T) {
	profile := sqliteProfileFixture(t)
	database := filepath.Join(profile.Path, "History")
	runSQLite(t, database, `
CREATE TABLE urls (id INTEGER PRIMARY KEY, url TEXT, title TEXT, last_visit_time INTEGER, visit_count INTEGER);
CREATE TABLE visits (id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER);
INSERT INTO urls VALUES
  (1, 'https://recent.example', 'Recent', 13400000000000000, 4),
  (2, 'chrome://settings', 'Private', 13400000001000000, 2),
  (3, 'https://old.example', 'Old', 12000000000000000, 1);
INSERT INTO visits VALUES
  (1, 1, 13400000000000000),
  (2, 1, 13400000000000001),
  (3, 2, 13400000001000000),
  (4, 3, 12000000000000000);
`)

	records, err := ExportHistory(t.Context(), profile, time.UnixMicro(13400000000000000-11_644_473_600_000_000))
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "https://recent.example", records[0].URL)
	require.Equal(t, 2, records[0].VisitCount)
	count, err := HistoryCount(t.Context(), profile, time.UnixMicro(13400000000000000-11_644_473_600_000_000))
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestLocalStorageSitesAndExportUseLivePortableRecords(t *testing.T) {
	profile := sqliteProfileFixture(t)
	databasePath := filepath.Join(profile.Path, "Local Storage", "leveldb")
	require.NoError(t, os.MkdirAll(filepath.Dir(databasePath), 0o755))
	database, err := leveldb.OpenFile(databasePath, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.NoError(t, database.Put([]byte("META:https://example.com"), localStorageMetadataFixture(1234), nil))
	require.NoError(t, database.Put(localStorageRecordKeyFixture("https://example.com", "theme"), chromiumStorageStringFixture("dark"), nil))
	require.NoError(t, database.Put(localStorageRecordKeyFixture("https://other.example", "emoji"), chromiumStorageStringFixture("hello 世界"), nil))
	require.NoError(t, database.Put(localStorageRecordKeyFixture("chrome-extension://abcdefghijklmnopabcdefghijklmnop", "private"), chromiumStorageStringFixture("skip"), nil))
	sites, err := LocalStorageSites(t.Context(), profile)
	require.NoError(t, err)
	require.Len(t, sites, 2)
	require.Equal(t, StorageSite{Origin: "https://example.com", Bytes: 1234}, sites[0])
	require.Equal(t, "https://other.example", sites[1].Origin)
	require.Positive(t, sites[1].Bytes)

	records, err := ExportLocalStorage(t.Context(), profile, []string{"https://example.com"})
	require.NoError(t, err)
	require.Equal(t, []StorageRecord{{Origin: "https://example.com", Kind: StorageKindLocal, Key: "theme", Value: "dark"}}, records)

	records, err = ExportLocalStorage(t.Context(), profile, nil)
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Equal(t, "hello 世界", records[1].Value)
}

func TestDiscoverExtensionsUsesPortableAllowlist(t *testing.T) {
	profile := sqliteProfileFixture(t)
	payload := `{
  "browser":{"theme":{"color_scheme2":2},"show_home_button":true},
  "extensions":{"settings":{
	"abcdefghijklmnopabcdefghijklmnop":{"from_webstore":true,"manifest":{"name":"Eligible"}},
    "bcdefghijklmnopabcdefghijklmnopa":{"state":0,"from_webstore":true,"manifest":{"name":"Disabled"}},
    "cdefghijklmnopabcdefghijklmnopab":{"state":1,"from_webstore":false,"manifest":{"name":"Local"}}
  }}
}`
	require.NoError(t, os.WriteFile(filepath.Join(profile.Path, "Preferences"), []byte(payload), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profile.Path, "Secure Preferences"), []byte(payload), 0o600))

	extensions, err := DiscoverExtensions(profile)
	require.NoError(t, err)
	require.Equal(t, []Extension{{ID: "abcdefghijklmnopabcdefghijklmnop", Name: "Eligible", Source: "chrome_web_store"}}, extensions)
}

func TestSelectedCookieFilterEscapesInput(t *testing.T) {
	cte, clause := selectedCookieFilter([]string{"example.com' OR 1=1 --"}, false)
	assert.Contains(t, cte, "example.com'' or 1=1 --")
	assert.NotContains(t, cte, "example.com' OR 1=1 --'")
	assert.Contains(t, clause, "EXISTS")
}

func TestExportCookiesSupportsLargeCustomSelection(t *testing.T) {
	profile := sqliteProfileFixture(t)
	database := filepath.Join(profile.Path, "Network", "Cookies")
	require.NoError(t, os.MkdirAll(filepath.Dir(database), 0o755))
	runSQLite(t, database, `
CREATE TABLE cookies (
  host_key TEXT, path TEXT, name TEXT, value TEXT, encrypted_value BLOB,
  expires_utc INTEGER, is_httponly INTEGER, is_secure INTEGER, samesite INTEGER
);
INSERT INTO cookies VALUES ('.site-1999.com', '/', 'selected', 'yes', X'', 0, 1, 1, 1);
`)
	sites := make([]string, 2000)
	for i := range sites {
		sites[i] = fmt.Sprintf("site-%d.com", i)
	}

	cookies, err := ExportCookies(t.Context(), profile, sites)
	require.NoError(t, err)
	require.Len(t, cookies, 1)
	assert.Equal(t, "selected", cookies[0].Name)
}

func TestCookieSitesFindsEveryImportableDomainWithoutDecryptingValues(t *testing.T) {
	profile := sqliteProfileFixture(t)
	database := filepath.Join(profile.Path, "Network", "Cookies")
	require.NoError(t, os.MkdirAll(filepath.Dir(database), 0o755))
	runSQLite(t, database, `
CREATE TABLE cookies (
  host_key TEXT, path TEXT, name TEXT, value TEXT, encrypted_value BLOB,
  expires_utc INTEGER, is_httponly INTEGER, is_secure INTEGER, samesite INTEGER,
  top_frame_site_key TEXT
);
INSERT INTO cookies VALUES
  ('.github.com', '/', 'one', '', X'DEADBEEF', 0, 1, 1, 1, ''),
  ('api.github.com', '/', 'two', '', X'DEADBEEF', 0, 1, 1, 1, ''),
  ('.older-site.com', '/', 'three', '', X'DEADBEEF', 0, 1, 1, 1, ''),
  ('.partitioned.com', '/', 'skip', '', X'DEADBEEF', 0, 1, 1, 1, 'https://top.example');
`)

	sites, err := CookieSites(t.Context(), profile, []Site{{Domain: "github.com", Visits: 42}})
	require.NoError(t, err)
	require.Len(t, sites, 2)
	assert.Equal(t, Site{Domain: "github.com", Visits: 42, CookieCount: 2}, sites[0])
	assert.Equal(t, Site{Domain: "older-site.com", CookieCount: 1}, sites[1])
}

func writeBrowserFixture(t *testing.T, home, rootSuffix, directory, name string) {
	t.Helper()
	root := filepath.Join(home, "Library/Application Support", rootSuffix)
	require.NoError(t, os.MkdirAll(filepath.Join(root, directory), 0o755))
	state := map[string]any{"profile": map[string]any{"info_cache": map[string]any{directory: map[string]string{"name": name}}}}
	data, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "Local State"), data, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, directory, "History"), nil, 0o600))
}

func sqliteProfileFixture(t *testing.T) Profile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Default")
	require.NoError(t, os.MkdirAll(path, 0o755))
	return Profile{ID: "chrome-default", Name: "Personal", Browser: Browser{ID: "chrome", Name: "Google Chrome"}, Path: path}
}

func runSQLite(t *testing.T, database, statement string) {
	t.Helper()
	command := exec.Command("/usr/bin/sqlite3", database, statement)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
}

func bytesRepeat(value byte, count int) []byte {
	result := make([]byte, count)
	for i := range result {
		result[i] = value
	}
	return result
}

func localStorageRecordKeyFixture(origin, key string) []byte {
	result := append([]byte("_"+origin+"\x00"), chromiumStorageStringFixture(key)...)
	return result
}

func chromiumStorageStringFixture(value string) []byte {
	runes := []rune(value)
	latin1 := true
	for _, value := range runes {
		if value > 255 {
			latin1 = false
			break
		}
	}
	if latin1 {
		result := make([]byte, 1, len(runes)+1)
		result[0] = 1
		for _, value := range runes {
			result = append(result, byte(value))
		}
		return result
	}
	units := utf16.Encode(runes)
	result := make([]byte, 1+len(units)*2)
	for index, value := range units {
		binary.LittleEndian.PutUint16(result[1+index*2:], value)
	}
	return result
}

func localStorageMetadataFixture(size uint64) []byte {
	result := []byte{8, 1, 16}
	buffer := make([]byte, binary.MaxVarintLen64)
	written := binary.PutUvarint(buffer, size)
	return append(result, buffer[:written]...)
}
