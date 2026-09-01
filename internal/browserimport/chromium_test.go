package browserimport

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
