package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/upstream"
)

var (
	ErrState    = errors.New("oauth_state_invalid")
	ErrCallback = errors.New("oauth_callback_invalid")
	ErrDenied   = errors.New("oauth_access_denied")
	ErrBusy     = errors.New("oauth_busy")
)

type Provider interface {
	AuthorizationURL(state, challenge string) string
	Exchange(ctx context.Context, code, verifier string) (accounts.Credential, error)
}

type namedProvider interface {
	AuthorizationURLFor(string, string, string) (string, error)
	ExchangeFor(context.Context, string, string, string, string) (accounts.Credential, error)
}

type Authorization struct {
	UserCode    string `json:"user_code,omitempty"`
	Interval    int64  `json:"interval,omitempty"`
	CallbackURL string `json:"callback_url,omitempty"`
	URL         string `json:"url"`
	State       string `json:"state"`
	ExpiresAt   int64  `json:"expires_at"`
}

type pending struct {
	device                                       string
	interval                                     int64
	nextPoll                                     time.Time
	polling                                      bool
	owner                                        [32]byte
	verifier, name, replaceID, provider, proxyID string
	expires                                      time.Time
}

type Flow struct {
	accounts *accounts.Service
	provider Provider
	mu       sync.Mutex
	pending  map[string]pending
	now      func() time.Time
}

func New(service *accounts.Service, provider Provider) *Flow {
	return &Flow{accounts: service, provider: provider, pending: make(map[string]pending), now: time.Now}
}

func (f *Flow) Begin(ctx context.Context, session, name, replaceID string) (Authorization, error) {
	return f.BeginProvider(ctx, "codex", session, name, replaceID)
}
func (f *Flow) BeginProvider(ctx context.Context, provider, session, name, replaceID string) (Authorization, error) {
	return f.BeginProviderWithProxy(ctx, provider, session, name, replaceID, "")
}
func (f *Flow) BeginProviderWithProxy(ctx context.Context, provider, session, name, replaceID, proxyID string) (Authorization, error) {
	if provider == "" {
		provider = "codex"
	}
	if !accounts.ValidProvider(provider) {
		return Authorization{}, accounts.ErrInput
	}
	if !f.accounts.ProviderEnabled(provider) {
		return Authorization{}, accounts.ErrProviderDisabled
	}
	if session == "" {
		return Authorization{}, ErrState
	}
	name, err := accounts.NormalizeName(name)
	if err != nil {
		return Authorization{}, err
	}
	if replaceID != "" {
		account, err := f.accounts.Get(ctx, replaceID)
		if err != nil {
			return Authorization{}, err
		}
		if account.Provider != provider {
			return Authorization{}, accounts.ErrIdentity
		}
		if proxyID != "" && proxyID != account.ProxyID {
			return Authorization{}, accounts.ErrProxyInput
		}
		proxyID = account.ProxyID
	}
	if _, err := f.accounts.ProxyURL(ctx, proxyID); err != nil {
		return Authorization{}, err
	}
	owner := sha256.Sum256([]byte(session))
	state, verifier := randomToken(), randomToken()
	expires := f.now().Add(10 * time.Minute)
	entry := pending{provider: provider, owner: owner, verifier: verifier, name: name, replaceID: replaceID, proxyID: proxyID, expires: expires}
	if err := f.reserve(state, entry); err != nil {
		return Authorization{}, err
	}
	if provider == "xai" {
		return f.beginDevice(ctx, session, state, entry)
	}
	challenge := sha256.Sum256([]byte(verifier))
	url := f.provider.AuthorizationURL(state, base64.RawURLEncoding.EncodeToString(challenge[:]))
	if provider != "codex" {
		named, ok := f.provider.(namedProvider)
		if !ok {
			_ = f.Cancel(session, state)
			return Authorization{}, accounts.ErrInput
		}
		var err error
		url, err = named.AuthorizationURLFor(provider, state, base64.RawURLEncoding.EncodeToString(challenge[:]))
		if err != nil {
			_ = f.Cancel(session, state)
			return Authorization{}, err
		}
	}
	return Authorization{CallbackURL: upstream.RedirectURI(provider), URL: url, State: state, ExpiresAt: expires.Unix()}, nil
}

func (f *Flow) Finish(ctx context.Context, session, state, callback string) (accounts.Account, error) {
	owner := sha256.Sum256([]byte(session))
	f.mu.Lock()
	entry, ok := f.pending[state]
	if !ok || entry.owner != owner || !entry.expires.After(f.now()) {
		if ok && !entry.expires.After(f.now()) {
			delete(f.pending, state)
		}
		f.mu.Unlock()
		return accounts.Account{}, ErrState
	}
	if !f.accounts.ProviderEnabled(entry.provider) {
		delete(f.pending, state)
		f.mu.Unlock()
		return accounts.Account{}, accounts.ErrProviderDisabled
	}
	if entry.provider == "xai" {
		f.mu.Unlock()
		return accounts.Account{}, ErrCallback
	}
	code, err := callbackCodeFor(entry.provider, callback, state)
	if errors.Is(err, ErrCallback) {
		f.mu.Unlock()
		return accounts.Account{}, err
	}
	// Consume before network IO: concurrent submissions and retries must never exchange the same code twice.
	delete(f.pending, state)
	f.mu.Unlock()
	if err != nil {
		return accounts.Account{}, err
	}
	if entry.replaceID != "" {
		account, err := f.accounts.Get(ctx, entry.replaceID)
		if err != nil {
			return accounts.Account{}, err
		}
		entry.proxyID = account.ProxyID
	}
	address, err := f.accounts.ProxyURL(ctx, entry.proxyID)
	if err != nil {
		return accounts.Account{}, err
	}
	ctx = upstream.WithProxyURL(ctx, address)
	var credential accounts.Credential
	if entry.provider == "codex" {
		credential, err = f.provider.Exchange(ctx, code, entry.verifier)
	} else {
		credential, err = f.provider.(namedProvider).ExchangeFor(ctx, entry.provider, code, state, entry.verifier)
	}
	if err != nil {
		return accounts.Account{}, err
	}
	return f.accounts.AuthorizeWithProxy(ctx, entry.name, credential, entry.replaceID, entry.proxyID)
}

func (f *Flow) Cancel(session, state string) error {
	owner := sha256.Sum256([]byte(session))
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.pending[state]
	if !ok {
		return nil
	}
	if entry.owner != owner {
		return ErrState
	}
	delete(f.pending, state)
	return nil
}

func callbackCode(callback, state string) (string, error) {
	return callbackCodeFor("codex", callback, state)
}
func callbackCodeFor(provider, callback, state string) (string, error) {
	if len(callback) > 8192 {
		return "", ErrCallback
	}
	target, _ := url.Parse(upstream.RedirectURI(provider))
	parsed, err := url.Parse(callback)
	if err != nil || parsed.Scheme != "http" || parsed.Host != target.Host || parsed.Path != target.Path || parsed.User != nil || parsed.Fragment != "" {
		return "", ErrCallback
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query["state"]) != 1 || query.Get("state") != state {
		return "", ErrCallback
	}
	if query.Get("error") != "" {
		return "", ErrDenied
	}
	if len(query["code"]) != 1 || query.Get("code") == "" {
		return "", ErrCallback
	}
	return query.Get("code"), nil
}

func randomToken() string {
	value := make([]byte, 32)
	_, _ = rand.Read(value)
	return base64.RawURLEncoding.EncodeToString(value)
}

func (f *Flow) reserve(state string, entry pending) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, value := range f.pending {
		if !value.expires.After(f.now()) || value.owner == entry.owner {
			delete(f.pending, key)
		}
	}
	if len(f.pending) >= 8 {
		return ErrBusy
	}
	f.pending[state] = entry
	return nil
}
