package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk/lib/credentials"
	// Registers the aws_sts identity token source.
	_ "github.com/kernel/kernel-go-sdk/lib/credentials/awssts"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
)

// federationPlaceholder is the Authorization header every request starts
// with. The SDK's own federation support, which reads the same environment
// variables and has no cache, leaves a request that has the header alone. The
// middleware replaces it with the access token on requests to the API and
// removes it from any other.
const federationPlaceholder = "Bearer workload-identity"

// federation authenticates requests with workload identity federation: the
// workload's identity token is exchanged for a short-lived Kernel access
// token, which is kept on disk between CLI processes (see federationCache).
type federation struct {
	ruleID   string
	apiURL   *url.URL
	exchange credentials.AccessTokenProvider
	// tokens holds the token for the life of this process and renews it
	// before it expires.
	tokens *credentials.TokenCache

	mu       sync.Mutex
	rejected string
}

// exchangedAssertionKey is the context key of a *string that receives the
// identity token an exchange sends.
type exchangedAssertionKey struct{}

func newFederation() (*federation, error) {
	source, opts, err := credentials.FederationFromEnv()
	if err != nil {
		return nil, err
	}
	identityToken := func(ctx context.Context) (string, error) {
		assertion, err := source(ctx)
		if sent, _ := ctx.Value(exchangedAssertionKey{}).(*string); sent != nil {
			*sent = strings.TrimSpace(assertion)
		}
		return assertion, err
	}
	apiBaseURL := util.GetBaseURL()
	apiURL, err := url.Parse(apiBaseURL)
	if err != nil {
		return nil, errors.New("KERNEL_BASE_URL is not a valid URL")
	}
	f := &federation{
		ruleID: opts.FederationRuleID,
		apiURL: apiURL,
		exchange: credentials.NewFederationProvider(credentials.FederationConfig{
			IdentityTokenProvider: identityToken,
			FederationRuleID:      opts.FederationRuleID,
			APIBaseURL:            apiBaseURL,
		}),
	}
	f.tokens = credentials.NewTokenCache(f.token)
	return f, nil
}

// token returns the workload's cached access token, or exchanges an identity
// token for a new one and caches it. force is set after the API rejected the
// previous token, whose cache entry has been removed by then.
func (f *federation) token(ctx context.Context, force bool) (*credentials.Token, error) {
	cache, reason := federationCacheFromEnv(f.ruleID)
	if cache == nil {
		pterm.Debug.Printfln("Workload identity token is not cached: %s", reason)
	} else if !force {
		if entry := cache.load(); entry != nil {
			pterm.Debug.Printfln("Using the cached workload identity token (valid for %s)", time.Until(entry.ExpiresAt).Round(time.Second))
			// The expiry is brought forward by the margin, so this process
			// comes back for another token before the cached one has less
			// than the margin left.
			return &credentials.Token{AccessToken: entry.AccessToken, ExpiresAt: entry.ExpiresAt.Add(-federationCacheMargin)}, nil
		}
	}
	var assertion string
	token, err := f.exchange(context.WithValue(ctx, exchangedAssertionKey{}, &assertion), force)
	if err != nil {
		return nil, err
	}
	pterm.Debug.Printfln("Exchanged an identity token for a Kernel access token (valid for %s)", time.Until(token.ExpiresAt).Round(time.Second))
	if cache != nil {
		if cache.fromFile {
			// The file may have changed since the cache was looked up. The
			// token belongs to the identity that was exchanged for it.
			cache.binding = federationBinding(assertion)
		}
		if err := cache.save(token); err != nil {
			pterm.Debug.Printfln("Could not cache the workload identity token: %v", err)
		}
	}
	return token, nil
}

// reject drops an access token the API refused, so the next one comes from a
// new exchange. Requests refused with the same token drop it once.
func (f *federation) reject(accessToken string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rejected == accessToken {
		return
	}
	f.rejected = accessToken
	f.tokens.Invalidate()
	if cache, _ := federationCacheFromEnv(f.ruleID); cache != nil {
		cache.forget(accessToken)
	}
}

// middleware authenticates each API request with the current access token.
// When the API answers 401 it drops that token, exchanges a new one and sends
// the request once more.
func (f *federation) middleware(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	// The token is for the API only.
	if !sameOrigin(req.URL, f.apiURL) {
		if req.Header.Get("Authorization") == federationPlaceholder {
			req.Header.Del("Authorization")
		}
		return next(req)
	}
	token, err := f.tokens.Token(req.Context())
	if err != nil {
		return unauthenticated(req, err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	resp, err := next(req)
	// A later middleware can send the request to a browser VM instead. A 401
	// from there says nothing about this token.
	if err != nil || resp.StatusCode != http.StatusUnauthorized || !sameOrigin(req.URL, f.apiURL) {
		return resp, err
	}

	f.reject(token.AccessToken)
	if req.Body != nil && req.GetBody == nil {
		return resp, nil
	}
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return resp, nil
		}
		req.Body = body
	}
	_ = resp.Body.Close()
	token, err = f.tokens.Token(req.Context())
	if err != nil {
		return unauthenticated(req, err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	resp, err = next(req)
	// The request is sent again only this once, whatever comes back.
	if err != nil {
		return unauthenticated(req, err)
	}
	resp.Header.Set("X-Should-Retry", "false")
	if resp.StatusCode == http.StatusUnauthorized && sameOrigin(req.URL, f.apiURL) {
		f.reject(token.AccessToken)
	}
	return resp, nil
}

// unauthenticated fails a request with err. The response tells the client
// not to send the request again.
func unauthenticated(req *http.Request, err error) (*http.Response, error) {
	return &http.Response{
		Status:  "credentials unavailable",
		Header:  http.Header{"X-Should-Retry": {"false"}},
		Body:    http.NoBody,
		Request: req,
	}, err
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

// FederationStatus describes workload identity federation as configured in
// the environment.
type FederationStatus struct {
	RuleID string
	// Err is why no identity token source could be set up.
	Err error
	// CachedTokenExpiresAt is when the cached access token expires. It is
	// zero when the next command exchanges a new one.
	CachedTokenExpiresAt time.Time
	// NotCached is why this workload's tokens are not cached, if they are not.
	NotCached string
}

// DescribeFederation reports how workload identity federation is set up, or
// nil when KERNEL_FEDERATION_RULE_ID is not set. It makes no network request.
func DescribeFederation() *FederationStatus {
	ruleID := os.Getenv(credentials.EnvFederationRuleID)
	if ruleID == "" {
		return nil
	}
	status := &FederationStatus{RuleID: ruleID}
	if _, _, status.Err = credentials.FederationFromEnv(); status.Err != nil {
		return status
	}
	cache, reason := federationCacheFromEnv(ruleID)
	if cache == nil {
		status.NotCached = reason
	} else if entry := cache.load(); entry != nil {
		status.CachedTokenExpiresAt = entry.ExpiresAt
	}
	return status
}
