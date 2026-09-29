package accounts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

var (
	ErrAllocated        = errors.New("allocation_pool_locked")
	ErrInput            = errors.New("invalid_account_input")
	ErrDuplicate        = errors.New("account_exists")
	ErrIdentity         = errors.New("account_identity_mismatch")
	ErrNotFound         = errors.New("account_not_found")
	ErrDisabled         = errors.New("account_disabled")
	ErrProviderDisabled = errors.New("provider_disabled")
	ErrReauthorize      = errors.New("account_reauthorization_required")
	ErrRefresh          = errors.New("account_refresh_failed")
	ErrLimit            = errors.New("account_limit")
)

type Account struct {
	GroupCount     *int64 `json:"group_count,omitempty"`
	ProxyID        string `json:"proxy_id,omitempty"`
	ID             string `json:"id"`
	Provider       string `json:"provider"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	Plan           string `json:"plan"`
	Enabled        bool   `json:"enabled"`
	Status         string `json:"status"`
	ExpiresAt      int64  `json:"expires_at"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
	MaxConcurrency int64  `json:"max_concurrency"`
}

type Service struct {
	db        *sql.DB
	queries   *db.Queries
	vault     *vault.Vault
	tenantID  int64
	now       func() time.Time
	codexOnly bool
	refreshes map[string]refreshState
	// Refresh and administrator mutations share one owner; no model stream holds this lock.
	mu sync.Mutex
}

// RestrictToCodex is set during runtime construction, before the service handles requests.
func (s *Service) RestrictToCodex() { s.codexOnly = true }

func (s *Service) ProviderEnabled(provider string) bool {
	return !s.codexOnly || provider == "codex"
}

func New(connection *sql.DB, cipher *vault.Vault) *Service {
	return NewForTenant(connection, cipher, 1)
}

func NewForTenant(connection *sql.DB, cipher *vault.Vault, tenantID int64) *Service {
	return &Service{db: connection, queries: db.New(connection), vault: cipher, tenantID: tenantID, now: time.Now}
}

func (s *Service) List(ctx context.Context) ([]Account, error) {
	rows, err := s.queries.ListAccounts(ctx, s.tenantID)
	if err != nil {
		return nil, err
	}
	result := make([]Account, 0, len(rows))
	for _, row := range rows {
		account := Account{ID: row.ID, Provider: row.Provider, Name: row.Name, Email: row.Email, Plan: row.Plan, Enabled: row.Enabled, Status: row.Status, ExpiresAt: row.ExpiresAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, MaxConcurrency: row.MaxConcurrency, GroupCount: &row.GroupCount}
		if row.ProxyID != nil {
			account.ProxyID = *row.ProxyID
		}
		result = append(result, account)
	}
	return result, nil
}

func (s *Service) Import(ctx context.Context, name string, raw []byte, replaceID string) (Account, error) {
	return s.ImportProvider(ctx, "codex", name, raw, replaceID)
}
func (s *Service) ImportProvider(ctx context.Context, provider, name string, raw []byte, replaceID string) (Account, error) {
	return s.ImportProviderWithProxy(ctx, provider, name, raw, replaceID, "")
}
func (s *Service) ImportProviderWithProxy(ctx context.Context, provider, name string, raw []byte, replaceID, proxyID string) (Account, error) {
	if provider == "" {
		provider = "codex"
	}
	if !s.ProviderEnabled(provider) {
		return Account{}, ErrProviderDisabled
	}
	credential, err := ParseFor(provider, raw)
	if err != nil {
		return Account{}, err
	}
	return s.save(ctx, name, credential, replaceID, proxyID, "unverified")
}

func (s *Service) Authorize(ctx context.Context, name string, credential Credential, replaceID string) (Account, error) {
	return s.AuthorizeWithProxy(ctx, name, credential, replaceID, "")
}

func (s *Service) AuthorizeWithProxy(ctx context.Context, name string, credential Credential, replaceID, proxyID string) (Account, error) {
	return s.save(ctx, name, credential, replaceID, proxyID, "ready")
}

func (s *Service) save(ctx context.Context, name string, credential Credential, replaceID, proxyID, status string) (Account, error) {
	if !s.ProviderEnabled(credential.Kind()) {
		return Account{}, ErrProviderDisabled
	}
	if err := credential.validate(); err != nil {
		return Account{}, err
	}
	var err error
	name, err = NormalizeName(name)
	if err != nil {
		return Account{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	credential.Rejected = false
	now := s.now().Unix()
	if replaceID != "" {
		row, err := s.get(ctx, replaceID)
		if err != nil {
			return Account{}, err
		}
		// Existing continuations must never be rebound to a different upstream identity.
		if row.AccountID != credential.AccountID || row.Provider != credential.Kind() {
			return Account{}, ErrIdentity
		}
		if proxyID != "" && (row.ProxyID == nil || *row.ProxyID != proxyID) {
			return Account{}, ErrProxyInput
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return Account{}, err
		}
		defer tx.Rollback()
		q := s.queries.WithTx(tx)
		if err := s.persist(ctx, q, replaceID, credential, status); err != nil {
			return Account{}, err
		}
		if err := q.InvalidateAccountCatalog(ctx, replaceID); err != nil {
			return Account{}, err
		}
		if err := audit.Record(ctx, q, "account.authorize", "account", replaceID); err != nil {
			return Account{}, err
		}
		if err := tx.Commit(); err != nil {
			return Account{}, err
		}
		delete(s.refreshes, replaceID)
		row.Email, row.Plan, row.Status, row.ExpiresAt, row.UpdatedAt = credential.Email, credential.Plan, status, credential.ExpiresAt, now
		return metadata(row), nil
	}
	random := make([]byte, 16)
	_, _ = rand.Read(random)
	id := hex.EncodeToString(random)
	plaintext, err := json.Marshal(credential)
	if err != nil {
		return Account{}, err
	}
	encrypted, err := s.vault.Seal(id, plaintext)
	if err != nil {
		return Account{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	var selectedProxy *string
	if proxyID != "" {
		if _, err := queries.GetProxy(ctx, db.GetProxyParams{ID: proxyID, TenantID: s.tenantID}); errors.Is(err, sql.ErrNoRows) {
			return Account{}, ErrProxyNotFound
		} else if err != nil {
			return Account{}, err
		}
		selectedProxy = &proxyID
	}
	count, err := queries.CountAccounts(ctx, s.tenantID)
	if err != nil {
		return Account{}, err
	}
	if count >= 100 {
		return Account{}, ErrLimit
	}
	n, err := queries.CreateAccount(ctx, db.CreateAccountParams{ID: id, TenantID: s.tenantID, Provider: credential.Kind(), Name: name, AccountID: credential.AccountID, Email: credential.Email, Plan: credential.Plan, Status: status, Credential: encrypted, ExpiresAt: credential.ExpiresAt, CreatedAt: now, UpdatedAt: now, ProxyID: selectedProxy})
	if err != nil {
		return Account{}, err
	}
	if n == 0 {
		return Account{}, ErrDuplicate
	}
	if err := audit.Record(ctx, queries, "account.create", "account", id); err != nil {
		return Account{}, err
	}
	if err := tx.Commit(); err != nil {
		return Account{}, err
	}
	return Account{ID: id, Provider: credential.Kind(), Name: name, Email: credential.Email, Plan: credential.Plan, Enabled: true, Status: status, ExpiresAt: credential.ExpiresAt, CreatedAt: now, UpdatedAt: now, MaxConcurrency: 30, ProxyID: proxyID}, nil
}

func (s *Service) SetEnabled(ctx context.Context, id string, enabled bool) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.get(ctx, id)
	if err != nil {
		return Account{}, err
	}
	if enabled && !s.ProviderEnabled(row.Provider) {
		return Account{}, ErrProviderDisabled
	}
	row.Enabled = enabled
	row.UpdatedAt = s.now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if err := q.SetAccountEnabled(ctx, db.SetAccountEnabledParams{ID: id, Enabled: enabled, UpdatedAt: row.UpdatedAt}); err != nil {
		return Account{}, err
	}
	if err := audit.Record(ctx, q, "account.update", "account", id); err != nil {
		return Account{}, err
	}
	return metadata(row), tx.Commit()
}

func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if _, err := q.GetAccount(ctx, db.GetAccountParams{ID: id, TenantID: s.tenantID}); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	locked, err := q.AccountHasAllocation(ctx, id)
	if err != nil {
		return err
	}
	if locked {
		return ErrAllocated
	}
	n, err := q.DeleteAccount(ctx, db.DeleteAccountParams{ID: id, TenantID: s.tenantID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := audit.Record(ctx, q, "account.delete", "account", id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	delete(s.refreshes, id)
	return nil
}

func (s *Service) persist(ctx context.Context, q *db.Queries, id string, c Credential, status string) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	encrypted, err := s.vault.Seal(id, raw)
	if err != nil {
		return err
	}
	return q.UpdateAccountCredential(ctx, db.UpdateAccountCredentialParams{ID: id, Credential: encrypted, Email: c.Email, Plan: c.Plan, Status: status, ExpiresAt: c.ExpiresAt, UpdatedAt: s.now().Unix()})
}

func (s *Service) get(ctx context.Context, id string) (db.Account, error) {
	row, err := s.queries.GetAccount(ctx, db.GetAccountParams{ID: id, TenantID: s.tenantID})
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

func metadata(row db.Account) Account {
	account := Account{ID: row.ID, Provider: row.Provider, Name: row.Name, Email: row.Email, Plan: row.Plan, Enabled: row.Enabled, Status: row.Status, ExpiresAt: row.ExpiresAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, MaxConcurrency: row.MaxConcurrency}
	if row.ProxyID != nil {
		account.ProxyID = *row.ProxyID
	}
	return account
}

func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", ErrInput
	}
	return name, nil
}

func (s *Service) Get(ctx context.Context, id string) (Account, error) {
	row, err := s.get(ctx, id)
	return metadata(row), err
}

func (s *Service) decrypt(row db.Account) (Credential, error) {
	plaintext, err := s.vault.Open(row.ID, row.Credential)
	if err != nil {
		return Credential{}, err
	}
	var credential Credential
	if json.Unmarshal(plaintext, &credential) != nil || credential.AccountID != row.AccountID || credential.Kind() != row.Provider {
		return Credential{}, vault.ErrDecrypt
	}
	return credential, nil
}

func (s *Service) RecordUse(ctx context.Context, id, usedToken string, accepted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	credential, err := s.decrypt(row)
	if err != nil {
		return err
	}
	// Results from older in-flight requests must not overwrite a later refresh or reauthorization.
	// A late success cannot repair a refresh token that requires explicit reauthorization.
	if credential.AccessToken != usedToken || (accepted && row.Status == "reauth_required") {
		return nil
	}
	status := "reauth_required"
	if accepted {
		status = "ready"
	}
	if row.Status == status {
		return nil
	}
	return s.queries.SetAccountStatus(ctx, db.SetAccountStatusParams{ID: id, Status: status, UpdatedAt: s.now().Unix()})
}

// Verify fails startup before serving traffic if the encryption key no longer matches persisted credentials.
func (s *Service) Verify(ctx context.Context) error {
	rows, err := s.queries.ListAccounts(ctx, s.tenantID)
	if err != nil {
		return err
	}
	for _, metadata := range rows {
		row, err := s.get(ctx, metadata.ID)
		if err != nil {
			return err
		}
		if _, err := s.decrypt(row); err != nil {
			return err
		}
	}
	proxies, err := s.queries.ListProxies(ctx, s.tenantID)
	if err != nil {
		return err
	}
	for _, proxy := range proxies {
		if _, err := s.openProxy(proxy.ID, proxy.Address); err != nil {
			return err
		}
	}
	return nil
}
