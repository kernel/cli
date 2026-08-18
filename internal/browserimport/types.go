package browserimport

import "time"

const BundleVersion = 1

type Browser struct {
	ID       string
	Name     string
	Root     string
	Keychain KeychainIdentity
}

type KeychainIdentity struct {
	Service string
	Account string
}

type Profile struct {
	ID        string
	Name      string
	Browser   Browser
	Path      string
	Directory string
}

func (p Profile) DisplayName() string {
	return p.Browser.Name + " / " + p.Name
}

type Site struct {
	Domain      string
	Visits      int
	LastVisited time.Time
	CookieCount int
}

type Cookie struct {
	Domain    string     `json:"domain"`
	Path      string     `json:"path"`
	Name      string     `json:"name"`
	Value     string     `json:"value"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	HTTPOnly  bool       `json:"http_only,omitempty"`
	Secure    bool       `json:"secure,omitempty"`
	SameSite  string     `json:"same_site,omitempty"`
}

type BookmarkDocument struct {
	Roots []BookmarkRoot `json:"roots"`
}

type BookmarkRoot struct {
	Name     string         `json:"name"`
	Children []BookmarkNode `json:"children"`
}

type BookmarkNode struct {
	Title        string         `json:"title"`
	URL          string         `json:"url,omitempty"`
	Children     []BookmarkNode `json:"children"`
	DateAdded    *time.Time     `json:"date_added,omitempty"`
	DateLastUsed *time.Time     `json:"date_last_used,omitempty"`
}

type HistoryRecord struct {
	URL        string    `json:"url"`
	Title      string    `json:"title,omitempty"`
	VisitedAt  time.Time `json:"visited_at"`
	VisitCount int       `json:"visit_count,omitempty"`
}

const (
	StorageKindLocal       = "local_storage"
	MaxPortableStorageSize = 64 << 20
)

type StorageRecord struct {
	Origin string `json:"origin"`
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	Value  string `json:"value"`
}

type StorageSite struct {
	Origin string
	Bytes  int64
}

// StorageExport contains portable records and records excluded by size limits.
type StorageExport struct {
	Records        []StorageRecord
	SkippedRecords int
	SkippedOrigins int
}

type ProfileData struct {
	Cookies               []Cookie
	Storage               []StorageRecord
	StorageRecordsSkipped int
	StorageOriginsSkipped int
	Bookmarks             *BookmarkDocument
	History               []HistoryRecord
}

type Source struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name"`
	Browser    string         `json:"browser,omitempty"`
	DataTypes  []string       `json:"data_types"`
	ItemCounts map[string]int `json:"item_counts,omitempty"`
}

type Inventory struct {
	Sources []Source `json:"sources"`
}

type ProfileSelection struct {
	SourceID   string   `json:"source_id"`
	TargetName string   `json:"target_name"`
	Categories []string `json:"categories"`
}

type Selection struct {
	Profiles []ProfileSelection `json:"profiles"`
}

type AppliedProfile struct {
	SourceID               string `json:"source_id"`
	ProfileID              string `json:"profile_id"`
	TargetName             string `json:"target_name"`
	StorageOriginsImported *int   `json:"storage_origins_imported,omitempty"`
	StorageEntriesImported *int   `json:"storage_entries_imported,omitempty"`
	StorageOriginsSkipped  *int   `json:"storage_origins_skipped,omitempty"`
	StorageEntriesSkipped  *int   `json:"storage_entries_skipped,omitempty"`
}

type ApplyFailure struct {
	SourceID string `json:"source_id,omitempty"`
	Stage    string `json:"stage"`
	Message  string `json:"message"`
}

type Applied struct {
	Profiles            []AppliedProfile `json:"profiles"`
	CredentialsImported int              `json:"credentials_imported"`
	Failure             *ApplyFailure    `json:"failure,omitempty"`
}

type Status struct {
	ID        string            `json:"id"`
	Phase     string            `json:"phase"`
	Inventory *Inventory        `json:"inventory,omitempty"`
	Selection *Selection        `json:"selection,omitempty"`
	Applied   *Applied          `json:"applied,omitempty"`
	Client    *ClientCompletion `json:"client,omitempty"`
}

type CreateResponse struct {
	ID                   string    `json:"id"`
	HelperToken          string    `json:"helper_token"`
	HelperTokenExpiresAt time.Time `json:"helper_token_expires_at"`
}

type HelperGrant struct {
	HelperToken          string    `json:"helper_token"`
	HelperTokenExpiresAt time.Time `json:"helper_token_expires_at"`
}

type ManagedAuthConnection struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
}

type ClientCounts struct {
	Cookies        int `json:"cookies"`
	Bookmarks      int `json:"bookmarks"`
	History        int `json:"history"`
	StorageOrigins int `json:"storage_origins"`
}

type ClientFailure struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type ClientCompletion struct {
	Outcome                string                  `json:"outcome"`
	Counts                 ClientCounts            `json:"counts"`
	ManagedAuthConnections []ManagedAuthConnection `json:"managed_auth_connections"`
	Failure                *ClientFailure          `json:"failure,omitempty"`
}
