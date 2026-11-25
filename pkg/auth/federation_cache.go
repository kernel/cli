package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk/lib/credentials"
	"github.com/zalando/go-keyring"
)

// EnvFederationTokenCache turns the on-disk cache of workload identity access
// tokens off when set to "off", "false", "0" or "no".
const EnvFederationTokenCache = "KERNEL_FEDERATION_TOKEN_CACHE"

// federationCacheMargin is how much lifetime a cached token needs to be
// reused. With less left, a new one is exchanged.
const federationCacheMargin = 60 * time.Second

// federationCacheEntry is a cached workload identity access token.
type federationCacheEntry struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	// Binding ties the token to the workload that obtained it; see
	// federationCache.
	Binding string `json:"binding"`
}

// federationCache is where the current workload keeps its access token
// between CLI processes.
//
// The entry name is a digest of the federation rule ID, the authorization
// server, the API origin, the identity token audience and source, the identity
// token file path, and the GitHub Actions repository, run, attempt and job.
// The entry also records a binding, a digest of what the workload gets its
// identity token with: the GitHub Actions token request URL and credential,
// which differ for every job, or the identity token itself for a token from a
// file or the environment. An entry is used only by a process that computes
// the same binding, so a token never reaches a workload that could not have
// obtained it.
type federationCache struct {
	name    string
	binding string
	// dir holds the entry as a file when set, and the keychain is not used.
	// It is the per-job temporary directory of a GitHub Actions runner.
	dir string
}

// federationCacheFromEnv returns the token cache of the current workload, or
// nil and the reason when its tokens are not cached.
func federationCacheFromEnv(ruleID string) (*federationCache, string) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvFederationTokenCache))) {
	case "off", "false", "0", "no":
		return nil, "turned off by " + EnvFederationTokenCache
	}
	apiURL, err := url.Parse(util.GetBaseURL())
	if err != nil || apiURL.Host == "" {
		return nil, "the API URL is not valid"
	}

	// The source is chosen the way credentials.FederationFromEnv chooses it.
	source := os.Getenv(credentials.EnvIdentityTokenSource)
	tokenFile := os.Getenv(credentials.EnvIdentityTokenFile)
	if source == "" {
		switch {
		case tokenFile != "":
			source = credentials.SourceFile
		case os.Getenv(credentials.EnvIdentityToken) != "":
			source = credentials.SourceEnv
		case os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL") != "":
			source = credentials.SourceGitHubActions
		}
	}

	cache := &federationCache{}
	if os.Getenv("GITHUB_ACTIONS") == "true" && os.Getenv("RUNNER_TEMP") != "" {
		cache.dir = filepath.Join(os.Getenv("RUNNER_TEMP"), "kernel-cli")
	}
	var fileToken string
	switch source {
	case credentials.SourceGitHubActions:
		if cache.dir == "" {
			return nil, "there is no GitHub Actions job directory (RUNNER_TEMP) to keep it in"
		}
	case credentials.SourceFile:
		data, err := os.ReadFile(tokenFile)
		if fileToken = strings.TrimSpace(string(data)); err != nil || fileToken == "" {
			return nil, "the identity token file could not be read"
		}
		if abs, err := filepath.Abs(tokenFile); err == nil {
			tokenFile = abs
		}
	case credentials.SourceEnv:
	default:
		// The identity comes from the machine's surroundings (an AWS role, a
		// Google Cloud service account), which this process cannot tell apart
		// from another workload's without asking the provider.
		return nil, fmt.Sprintf("the %s identity token source has nothing to tie a cached token to this workload", source)
	}

	cache.name = "federation-token-" + digest(
		ruleID, authBaseURLFromEnv(), strings.ToLower(apiURL.Scheme+"://"+apiURL.Host),
		os.Getenv(credentials.EnvIdentityTokenAudience), source, tokenFile,
		os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_RUN_ATTEMPT"), os.Getenv("GITHUB_JOB"),
	)
	cache.binding = digest(
		os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"), os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
		fileToken, strings.TrimSpace(os.Getenv(credentials.EnvIdentityToken)),
	)
	return cache, ""
}

// digest hashes parts so that no two lists of parts collide.
func digest(parts ...string) string {
	var b strings.Builder
	for _, part := range parts {
		b.WriteString(strconv.Itoa(len(part)) + ":" + part)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func (c *federationCache) filePath() (string, error) {
	dir := c.dir
	if dir == "" {
		var err error
		if dir, err = getConfigDir(); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, c.name+".json"), nil
}

// copies returns the entry from each store that holds one.
func (c *federationCache) copies() (fromKeyring, fromFile *federationCacheEntry) {
	parse := func(data []byte) *federationCacheEntry {
		var entry federationCacheEntry
		if json.Unmarshal(data, &entry) != nil {
			return nil
		}
		return &entry
	}
	if c.dir == "" {
		if data, err := keyring.Get(KeyringService, c.name); err == nil {
			fromKeyring = parse([]byte(data))
		}
	}
	if path, err := c.filePath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			fromFile = parse(data)
		}
	}
	return fromKeyring, fromFile
}

// load returns the cached token when this workload may still use it.
func (c *federationCache) load() *federationCacheEntry {
	var usable *federationCacheEntry
	fromKeyring, fromFile := c.copies()
	for _, entry := range []*federationCacheEntry{fromKeyring, fromFile} {
		if entry == nil || entry.AccessToken == "" || entry.Binding != c.binding ||
			time.Until(entry.ExpiresAt) <= federationCacheMargin {
			continue
		}
		if usable == nil || entry.ExpiresAt.After(usable.ExpiresAt) {
			usable = entry
		}
	}
	return usable
}

// save stores the token where saveTokens stores a login: in the keychain, or
// in a file when the keychain refuses it. Under a GitHub Actions runner it is
// always a file in the job's temporary directory.
func (c *federationCache) save(token *credentials.Token) error {
	data, err := json.Marshal(federationCacheEntry{
		AccessToken: token.AccessToken,
		ExpiresAt:   token.ExpiresAt,
		Binding:     c.binding,
	})
	if err != nil {
		return err
	}
	path, pathErr := c.filePath()
	if c.dir == "" {
		if err := setKeyring(KeyringService, c.name, string(data)); err == nil {
			if pathErr == nil {
				_ = os.Remove(path)
			}
			return nil
		}
	}
	if pathErr != nil {
		return pathErr
	}
	if err := writeFileAtomically(path, data); err != nil {
		return err
	}
	if c.dir == "" {
		_ = keyring.Delete(KeyringService, c.name)
	}
	return nil
}

// writeFileAtomically replaces path with data, readable by the owner only. A
// process reading path sees the old content or the new, never part of either.
func writeFileAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	// CreateTemp creates the file with mode 0600.
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

// forget removes the cached token if it is still accessToken, which the API
// rejected. A token another process has stored since then is kept.
func (c *federationCache) forget(accessToken string) {
	fromKeyring, fromFile := c.copies()
	if fromKeyring != nil && fromKeyring.AccessToken == accessToken {
		_ = keyring.Delete(KeyringService, c.name)
	}
	if fromFile != nil && fromFile.AccessToken == accessToken {
		if path, err := c.filePath(); err == nil {
			_ = os.Remove(path)
		}
	}
}
