// Package allocations owns exclusive pool schemes and their accounting.
package allocations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/pricing"
	"github.com/murongg/SubLane/internal/storage/db"
)

var (
	ErrInput                 = errors.New("invalid_allocation_input")
	ErrNotFound              = errors.New("allocation_not_found")
	ErrPoolConflict          = errors.New("allocation_pool_conflict")
	ErrUnavailable           = errors.New("allocation_unavailable")
	ErrQuota                 = errors.New("allocation_exhausted")
	ErrPending               = errors.New("allocation_pending")
	ErrRisk                  = errors.New("allocation_risk_limit")
	ErrUnpriced              = errors.New("allocation_model_unpriced")
	ErrUnknownWindowOverride = errors.New("allocation_unknown_window_override")
	ErrSettlement            = errors.New("invalid_allocation_settlement")
)

type Share struct {
	UserID int64 `json:"user_id"`
	// Limit is a percentage in hundredths for share mode; otherwise it
	// uses the selected accounting unit's smallest increment. Time-window
	// members use shared limits unless their duration appears in WindowOverrides.
	Limit           int64               `json:"limit"`
	WindowOverrides []MemberWindowLimit `json:"window_overrides,omitempty"`
}

type MemberWindowLimit struct {
	DurationSeconds int64 `json:"duration_seconds"`
	Limit           int64 `json:"limit"`
}

type WindowCondition struct {
	DurationSeconds int64 `json:"duration_seconds"`
	Limit           int64 `json:"limit"`
}

// Rates are micro-USD per million tokens.
type Rate struct {
	Model  string `json:"model"`
	Input  int64  `json:"input"`
	Cached int64  `json:"cached"`
	Output int64  `json:"output"`
}
type Config struct {
	Mode               string            `json:"mode"`
	Period             string            `json:"period"`
	ResetTime          string            `json:"reset_time,omitempty"`
	ResetDay           int               `json:"reset_day,omitempty"`
	Members            []Share           `json:"members"`
	Rates              []Rate            `json:"rates"`
	RatioUnit          string            `json:"ratio_unit,omitempty"`
	Windows            []WindowCondition `json:"windows,omitempty"`
	MissingPriceModels []string          `json:"missing_price_models,omitempty"`
	CatalogUnavailable bool              `json:"catalog_unavailable,omitempty"`
	// Total is in tokens or micro-USD according to RatioUnit.
	Total int64 `json:"total,omitempty"`
}
type SchemeInput struct {
	Name      string `json:"name"`
	GroupID   int64  `json:"group_id"`
	Enabled   bool   `json:"enabled"`
	StartNext bool   `json:"start_next"`
	Config    Config `json:"config"`
}
type Revision struct {
	ID          int64  `json:"id"`
	EffectiveAt int64  `json:"effective_at"`
	Config      Config `json:"config"`
}
type Scheme struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	GroupID         int64  `json:"group_id"`
	GroupName       string `json:"group_name"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       int64  `json:"created_at"`
	EditEffectiveAt int64  `json:"edit_effective_at"`
	Revision
	Next *Revision `json:"next"`
}
type Service struct {
	conn     *sql.DB
	tenantID int64
	now      func() time.Time
	pricing  *pricing.Service
	location func() *time.Location
}

func New(conn *sql.DB) *Service { return NewForTenant(conn, 1) }
func NewForTenant(conn *sql.DB, tenantID int64) *Service {
	return &Service{conn: conn, tenantID: tenantID, now: time.Now, location: func() *time.Location { return time.UTC }}
}
func NewWithPricing(conn *sql.DB, catalog *pricing.Service) *Service {
	return NewForTenantWithPricing(conn, 1, catalog)
}
func NewForTenantWithPricing(conn *sql.DB, tenantID int64, catalog *pricing.Service) *Service {
	return &Service{conn: conn, tenantID: tenantID, now: time.Now, pricing: catalog, location: func() *time.Location { return time.UTC }}
}

func (s *Service) SetLocation(provider func() *time.Location) { s.location = provider }
func validName(v string) bool {
	return v == strings.TrimSpace(v) && utf8.ValidString(v) && utf8.RuneCountInString(v) > 0 && utf8.RuneCountInString(v) <= 64 && strings.IndexFunc(v, unicode.IsControl) < 0
}
func bit(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
func ratioLimit(total, share int64) int64 {
	// Round down so member limits never sum beyond the configured total.
	return total * share / 10000
}
func validMemberLimit(c Config, m Share) bool {
	if m.UserID <= 0 || m.Limit < 0 || m.Limit > 1_000_000_000_000 {
		return false
	}
	switch c.Mode {
	case "ratio":
		return m.Limit > 0 && m.Limit <= 10000 && ratioLimit(c.Total, m.Limit) > 0 && len(m.WindowOverrides) == 0
	case "windows":
		if m.Limit != 0 {
			return false
		}
		seen := map[int64]bool{}
		for _, override := range m.WindowOverrides {
			if seen[override.DurationSeconds] || override.Limit < 0 || override.Limit > 1_000_000_000_000 {
				return false
			}
			seen[override.DurationSeconds] = true
		}
		return true
	default:
		return m.Limit > 0 && len(m.WindowOverrides) == 0
	}
}
func normalize(c *Config) error {
	// Match the gateway's pool catalog limit: automatic pricing may include every supported model.
	if (c.Mode != "tokens" && c.Mode != "amount" && c.Mode != "ratio" && c.Mode != "windows") || len(c.Members) == 0 || len(c.Members) > 100 || len(c.Rates) > 4096 {
		return ErrInput
	}
	if (c.Period != "day" && c.Period != "month" && c.Period != "durations") || !validResetSchedule(*c) || (c.Period == "durations") != (c.Mode == "windows") {
		return ErrInput
	}
	if c.Mode == "ratio" {
		if (c.RatioUnit != "tokens" && c.RatioUnit != "amount") || c.Total <= 0 || c.Total > 1_000_000_000_000 {
			return ErrInput
		}
		if c.RatioUnit == "tokens" && len(c.Rates) > 0 {
			return ErrInput
		}
	} else if c.Total != 0 || c.RatioUnit != "" {
		return ErrInput
	}
	if c.Mode == "windows" {
		if len(c.Windows) == 0 || len(c.Windows) > 8 {
			return ErrInput
		}
		seen := map[int64]bool{}
		for _, condition := range c.Windows {
			if condition.DurationSeconds < 3600 || condition.DurationSeconds > 365*86400 || condition.DurationSeconds%3600 != 0 || seen[condition.DurationSeconds] || condition.Limit < 0 || condition.Limit > 1_000_000_000_000 {
				return ErrInput
			}
			seen[condition.DurationSeconds] = true
		}
		for _, member := range c.Members {
			for _, override := range member.WindowOverrides {
				if !seen[override.DurationSeconds] {
					return ErrUnknownWindowOverride
				}
			}
		}
	} else if len(c.Windows) > 0 {
		return ErrInput
	}
	seen := map[int64]bool{}
	var total int64
	for _, m := range c.Members {
		if seen[m.UserID] || !validMemberLimit(*c, m) {
			return ErrInput
		}
		seen[m.UserID] = true
		total += m.Limit
	}
	if c.Mode == "ratio" && total > 10000 {
		return ErrInput
	}
	if (c.Mode == "amount" || c.Mode == "windows" || c.Mode == "ratio" && c.RatioUnit == "amount") && len(c.Rates) == 0 {
		return ErrInput
	}
	models := map[string]bool{}
	for i, r := range c.Rates {
		_, model := groups.SplitModel(r.Model)
		if model == "" || len(model) > 128 || models[model] || strings.ContainsAny(model, " \t\n*") {
			return ErrInput
		}
		models[model] = true
		c.Rates[i].Model = model
		if r.Input <= 0 || r.Cached < 0 || r.Output <= 0 || r.Input > 1_000_000_000 || r.Cached > 1_000_000_000 || r.Output > 1_000_000_000 {
			return ErrInput
		}
	}
	if c.Rates == nil {
		c.Rates = []Rate{}
	}
	slices.SortFunc(c.Members, func(a, b Share) int {
		if a.UserID < b.UserID {
			return -1
		}
		if a.UserID > b.UserID {
			return 1
		}
		return 0
	})
	return nil
}
func Current(ctx context.Context, q *db.Queries, id, now int64) (Revision, error) {
	r, err := q.CurrentAllocationRevision(ctx, db.CurrentAllocationRevisionParams{SchemeID: id, EffectiveAt: now})
	if errors.Is(err, sql.ErrNoRows) {
		return Revision{}, ErrUnavailable
	}
	if err != nil {
		return Revision{}, err
	}
	return decodeRevision(r.ID, r.EffectiveAt, r.Config)
}
func decodeRevision(id, at int64, raw string) (Revision, error) {
	r := Revision{ID: id, EffectiveAt: at}
	if err := json.Unmarshal([]byte(raw), &r.Config); err != nil {
		return r, err
	}
	if (r.Config.Period != "day" && r.Config.Period != "month" && r.Config.Period != "durations") || !validResetSchedule(r.Config) || (r.Config.Period == "durations") != (r.Config.Mode == "windows") {
		return r, ErrInput
	}
	return r, nil
}
func (s *Service) Schemes(ctx context.Context) ([]Scheme, error) {
	q := db.New(s.conn)
	rows, err := q.ListAllocationSchemes(ctx, s.tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]Scheme, 0, len(rows))
	for _, r := range rows {
		v, err := readScheme(ctx, q, s.tenantID, r.ID, s.now().Unix(), s.location())
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
func readScheme(ctx context.Context, q *db.Queries, tenantID, id, now int64, location *time.Location) (Scheme, error) {
	r, err := q.GetTenantAllocationScheme(ctx, db.GetTenantAllocationSchemeParams{ID: id, TenantID: tenantID})
	if err != nil {
		return Scheme{}, ErrNotFound
	}
	out := Scheme{ID: r.ID, Name: r.Name, GroupID: r.GroupID, GroupName: r.GroupName, Enabled: r.Enabled == 1, CreatedAt: r.CreatedAt}
	out.Revision, err = Current(ctx, q, id, now)
	if err != nil && !errors.Is(err, ErrUnavailable) {
		return out, err
	}
	if err == nil {
		out.EditEffectiveAt = nextEffective(out.Config, now, location, out.EffectiveAt)
	}
	next, e := q.NextAllocationRevision(ctx, db.NextAllocationRevisionParams{SchemeID: id, EffectiveAt: now})
	if e == nil {
		rev, e := decodeRevision(next.ID, next.EffectiveAt, next.Config)
		if e != nil {
			return out, e
		}
		out.Next = &rev
		if out.Revision.ID == 0 {
			out.Revision = rev
			out.EditEffectiveAt = rev.EffectiveAt
		}
	} else if !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	return out, nil
}
func (s *Service) SaveScheme(ctx context.Context, id int64, in SchemeInput) (Scheme, error) {
	if id < 0 || !validName(in.Name) || in.GroupID <= 0 {
		return Scheme{}, ErrInput
	}
	if in.Config.Mode == "ratio" {
		var total int64
		for _, member := range in.Config.Members {
			total += member.Limit
		}
		if total > 10000 {
			return Scheme{}, ErrInput
		}
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return Scheme{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	now := s.now().Unix()
	if _, err := q.GetTenantGroup(ctx, db.GetTenantGroupParams{ID: in.GroupID, TenantID: s.tenantID}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Scheme{}, ErrInput
		}
		return Scheme{}, err
	}
	for _, m := range in.Config.Members {
		allowed, err := q.CanUseGroup(ctx, db.CanUseGroupParams{UserID: m.UserID, GroupID: in.GroupID})
		if err != nil {
			return Scheme{}, err
		}
		if !allowed {
			return Scheme{}, ErrInput
		}
	}
	accounts, err := q.AllocationPoolAccounts(ctx, in.GroupID)
	if err != nil {
		return Scheme{}, err
	}
	if len(accounts) == 0 {
		return Scheme{}, ErrInput
	}
	for _, a := range accounts {
		if a.Shared > 0 {
			return Scheme{}, ErrPoolConflict
		}
	}
	var previousRates []Rate
	if id != 0 {
		previous, err := readScheme(ctx, q, s.tenantID, id, now, s.location())
		if err != nil {
			return Scheme{}, err
		}
		if previous.GroupID != in.GroupID {
			return Scheme{}, ErrInput
		}
		previousRates = append(previousRates, previous.Config.Rates...)
		if previous.Next != nil {
			previousRates = append(previousRates, previous.Next.Config.Rates...)
		}
	}
	if err = s.applyPrices(ctx, q, in.GroupID, &in.Config, previousRates); err != nil {
		return Scheme{}, err
	}
	if err := normalize(&in.Config); err != nil {
		return Scheme{}, err
	}
	effective := now
	if id == 0 {
		if _, err = q.GetPoolAllocation(ctx, in.GroupID); err == nil {
			return Scheme{}, ErrPoolConflict
		} else if !errors.Is(err, sql.ErrNoRows) {
			return Scheme{}, err
		}
		rows, e := q.ListAllocationSchemes(ctx, s.tenantID)
		if e != nil {
			return Scheme{}, e
		}
		if len(rows) >= 32 {
			return Scheme{}, ErrInput
		}
		id, err = q.CreateAllocationScheme(ctx, db.CreateAllocationSchemeParams{Name: in.Name, GroupID: in.GroupID, Enabled: bit(in.Enabled), CreatedAt: now})
		if err != nil {
			return Scheme{}, err
		}
		if in.StartNext {
			effective = nextEffective(in.Config, now, s.location(), now)
		}
	} else {
		if err = q.UpdateAllocationScheme(ctx, db.UpdateAllocationSchemeParams{ID: id, Name: in.Name, Enabled: bit(in.Enabled)}); err != nil {
			return Scheme{}, err
		}
		current, err := Current(ctx, q, id, now)
		if err == nil {
			effective = nextEffective(current.Config, now, s.location(), current.EffectiveAt)
		} else if errors.Is(err, ErrUnavailable) {
			next, e := q.NextAllocationRevision(ctx, db.NextAllocationRevisionParams{SchemeID: id, EffectiveAt: now})
			if e != nil {
				return Scheme{}, e
			}
			effective = next.EffectiveAt
		} else {
			return Scheme{}, err
		}
		if err = q.DeleteNextAllocationRevisions(ctx, db.DeleteNextAllocationRevisionsParams{SchemeID: id, EffectiveAt: now}); err != nil {
			return Scheme{}, err
		}
	}
	raw, err := json.Marshal(in.Config)
	if err != nil {
		return Scheme{}, err
	}
	if _, err = q.SaveAllocationRevision(ctx, db.SaveAllocationRevisionParams{SchemeID: id, EffectiveAt: effective, Config: string(raw)}); err != nil {
		return Scheme{}, err
	}
	if err = audit.Record(ctx, q, "allocation.save", "allocation", audit.ID(id)); err != nil {
		return Scheme{}, err
	}
	out, err := readScheme(ctx, q, s.tenantID, id, now, s.location())
	if err != nil {
		return Scheme{}, err
	}
	return out, tx.Commit()
}

// Priced revisions use the current catalog, then their previous saved rates when a price disappears.
// Client-supplied rates never override catalog or previously saved prices in production.
func (s *Service) applyPrices(ctx context.Context, q *db.Queries, groupID int64, config *Config, previous []Rate) error {
	config.MissingPriceModels = nil
	config.CatalogUnavailable = false
	if config.Mode == "tokens" || config.Mode == "ratio" && config.RatioUnit == "tokens" {
		config.Rates = []Rate{}
		return nil
	}
	if s.pricing == nil {
		if len(config.Rates) == 0 {
			config.Rates = append([]Rate(nil), previous...)
		}
		if len(config.Rates) > 0 {
			return nil
		}
		return ErrUnpriced
	}
	group, err := q.GetGroup(ctx, groupID)
	if err != nil {
		return err
	}
	allowed, err := q.ListGroupModels(ctx, groupID)
	if err != nil {
		return err
	}
	policy := groups.ModelPolicy{Restricted: group.RestrictedModels, Models: allowed}
	rows, err := q.ListGroupCatalogs(ctx, groupID)
	if err != nil {
		return err
	}
	previousByModel := make(map[string]Rate, len(previous))
	for _, rate := range previous {
		previousByModel[rate.Model] = rate
	}
	models := map[string]bool{}
	for _, row := range rows {
		catalog, err := accounts.DecodeCatalog(row.ModelsSnapshot, row.ModelsRevision)
		if err != nil || catalog.UpdatedAt == 0 {
			config.CatalogUnavailable = true
			continue
		}
		for _, model := range catalog.Models {
			_, native := groups.SplitModel(model)
			// The policy needs the account provider to enforce provider-scoped allowlists.
			if policy.Allows(row.Provider + "/" + native) {
				models[native] = true
			}
		}
	}
	// If a catalog is temporarily unknown, retain existing prices while its models cannot be rediscovered.
	if config.CatalogUnavailable {
		for model := range previousByModel {
			if policy.Allows(model) {
				models[model] = true
			}
		}
	}
	ids := make([]string, 0, len(models))
	for model := range models {
		ids = append(ids, model)
	}
	sort.Strings(ids)
	config.Rates = make([]Rate, 0, len(ids))
	for _, model := range ids {
		if price, ok := s.pricing.Lookup(model); ok {
			config.Rates = append(config.Rates, Rate{Model: model, Input: price.Input, Cached: price.Cached, Output: price.Output})
			continue
		}
		config.MissingPriceModels = append(config.MissingPriceModels, model)
		if rate, ok := previousByModel[model]; ok {
			config.Rates = append(config.Rates, rate)
		}
	}
	if len(config.Rates) == 0 {
		return ErrUnpriced
	}
	return nil
}
func Cost(r Rate, input, output, cached int64) (int64, error) {
	if input < 0 || output < 0 || cached < 0 || cached > input || input > 1_000_000_000 || output > 1_000_000_000 {
		return 0, ErrInput
	}
	// Round only after summing all token categories; even sub-micro charges remain nonzero.
	sum := new(big.Int)
	for _, v := range [][2]int64{{input - cached, r.Input}, {cached, r.Cached}, {output, r.Output}} {
		if v[1] < 0 {
			return 0, ErrInput
		}
		sum.Add(sum, new(big.Int).Mul(big.NewInt(v[0]), big.NewInt(v[1])))
	}
	sum.Add(sum, big.NewInt(999999))
	sum.Div(sum, big.NewInt(1000000))
	if !sum.IsInt64() {
		return 0, ErrInput
	}
	return sum.Int64(), nil
}

// Access toggles never depend on upstream availability or rewrite the policy revision.
func (s *Service) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	row, err := q.GetTenantAllocationScheme(ctx, db.GetTenantAllocationSchemeParams{ID: id, TenantID: s.tenantID})
	if err != nil {
		return ErrNotFound
	}
	if err = q.UpdateAllocationScheme(ctx, db.UpdateAllocationSchemeParams{ID: id, Name: row.Name, Enabled: bit(enabled)}); err != nil {
		return err
	}
	if err = audit.Record(ctx, q, "allocation.save", "allocation", audit.ID(id)); err != nil {
		return err
	}
	return tx.Commit()
}
