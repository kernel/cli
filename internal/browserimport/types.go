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
	Profiles          []ProfileSelection `json:"profiles"`
	CredentialSources []string           `json:"credential_sources"`
}

type AppliedProfile struct {
	SourceID   string `json:"source_id"`
	ProfileID  string `json:"profile_id"`
	TargetName string `json:"target_name"`
}

type ApplyFailure struct {
	SourceID string `json:"source_id,omitempty"`
	Stage    string `json:"stage"`
	Message  string `json:"message"`
}

type Applied struct {
	Profiles            []AppliedProfile `json:"profiles"`
	CredentialsImported int              `json:"credentials_imported"`
	ExtensionsDetected  int              `json:"extensions_detected"`
	Failure             *ApplyFailure    `json:"failure,omitempty"`
}

type Status struct {
	ID        string     `json:"id"`
	Phase     string     `json:"phase"`
	Inventory *Inventory `json:"inventory,omitempty"`
	Selection *Selection `json:"selection,omitempty"`
	Applied   *Applied   `json:"applied,omitempty"`
}

type CreateResponse struct {
	ID                   string    `json:"id"`
	HelperToken          string    `json:"helper_token"`
	HelperTokenExpiresAt time.Time `json:"helper_token_expires_at"`
}
