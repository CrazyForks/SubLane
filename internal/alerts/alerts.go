// Package alerts owns optional workspace notifications, separate from inference execution.
package alerts

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

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/capacity"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

type Input struct {
	QuotaThreshold int    `json:"quota_threshold"`
	ModelAlerts    bool   `json:"model_alerts"`
	Enabled        bool   `json:"enabled"`
	URL            string `json:"url"`
	Clear          bool   `json:"clear"`
}
type configuration struct {
	QuotaThreshold int    `json:"quota_threshold,omitempty"`
	ModelAlerts    bool   `json:"model_alerts,omitempty"`
	TestedAt       int64  `json:"tested_at,omitempty"`
	Enabled        bool   `json:"enabled"`
	Endpoint       []byte `json:"endpoint,omitempty"`
}
type Incident struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Since   int64  `json:"since"`
}
type State struct {
	QuotaThreshold  int        `json:"quota_threshold"`
	ModelAlerts     bool       `json:"model_alerts"`
	Enabled         bool       `json:"enabled"`
	Configured      bool       `json:"configured"`
	Destination     string     `json:"destination"`
	LastDeliveredAt int64      `json:"last_delivered_at"`
	NextRetryAt     int64      `json:"next_retry_at"`
	DeliveryFailed  bool       `json:"delivery_failed"`
	Incidents       []Incident `json:"incidents"`
}
type Event struct {
	ID          string `json:"id"`
	WorkspaceID int64  `json:"workspace_id"`
	Kind        string `json:"kind"`
	Subject     string `json:"subject"`
	Status      string `json:"status"`
	ObservedAt  int64  `json:"observed_at"`
}
type Service struct {
	conn             *sql.DB
	queries          *db.Queries
	vault            *vault.Vault
	sender           Sender
	now              func() time.Time
	mu               sync.Mutex
	checking         sync.Mutex
	cancel           context.CancelFunc
	done             chan struct{}
	wake             chan struct{}
	flights          map[int64]context.CancelFunc
	testFlights      map[int64]context.CancelFunc
	capacityObserver func(context.Context, int64) ([]capacity.Pool, error)
}

func New(conn *sql.DB, cipher *vault.Vault, sender Sender) *Service {
	if sender == nil {
		sender = newSender()
	}
	return &Service{conn: conn, queries: db.New(conn), vault: cipher, sender: sender, now: time.Now, wake: make(chan struct{}, 1), flights: make(map[int64]context.CancelFunc), testFlights: make(map[int64]context.CancelFunc)}
}

func (s *Service) Verify(ctx context.Context) error {
	rows, err := s.queries.ListAlertConfigs(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		var cfg configuration
		if json.Unmarshal([]byte(row.AlertConfig), &cfg) != nil {
			return ErrInput
		}
		if len(cfg.Endpoint) > 0 {
			raw, err := s.vault.OpenWebhook(row.ID, cfg.Endpoint)
			if err != nil {
				return err
			}
			if _, err := destination(string(raw)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) State(ctx context.Context, tenantID int64) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state(ctx, tenantID)
}
func (s *Service) state(ctx context.Context, tenantID int64) (State, error) {
	result := State{Incidents: []Incident{}}
	raw, err := s.queries.GetAlertConfig(ctx, tenantID)
	if err != nil {
		return result, err
	}
	var cfg configuration
	if json.Unmarshal([]byte(raw), &cfg) != nil {
		return result, ErrInput
	}
	result.Enabled = cfg.Enabled
	result.QuotaThreshold = cfg.QuotaThreshold
	result.ModelAlerts = cfg.ModelAlerts
	result.Configured = len(cfg.Endpoint) > 0
	if result.Configured {
		plain, err := s.vault.OpenWebhook(tenantID, cfg.Endpoint)
		if err != nil {
			return result, err
		}
		u, err := destination(string(plain))
		if err != nil {
			return result, err
		}
		result.Destination = u.Host
	}
	states, err := s.queries.ListAlertStates(ctx, tenantID)
	if err != nil {
		return result, err
	}
	for _, state := range states {
		result.LastDeliveredAt = max(result.LastDeliveredAt, state.DeliveredAt)
		if state.Active != state.DeliveredActive && state.RetryAt > 0 {
			if result.NextRetryAt == 0 || state.RetryAt < result.NextRetryAt {
				result.NextRetryAt = state.RetryAt
			}
		}
		result.DeliveryFailed = result.DeliveryFailed || state.DeliveryFailed != 0
		if state.Active != 0 {
			result.Incidents = append(result.Incidents, Incident{Kind: state.Kind, Subject: state.Subject, Since: state.ChangedAt})
		}
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, tenantID int64, input Input) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.Clear && (input.Enabled || input.URL != "") {
		return State{}, ErrInput
	}
	if input.QuotaThreshold < 0 || input.QuotaThreshold > 99 {
		return State{}, ErrInput
	}
	raw, err := s.queries.GetAlertConfig(ctx, tenantID)
	if err != nil {
		return State{}, err
	}
	var cfg configuration
	if json.Unmarshal([]byte(raw), &cfg) != nil {
		return State{}, ErrInput
	}
	cfg.Enabled = input.Enabled
	cfg.QuotaThreshold = input.QuotaThreshold
	cfg.ModelAlerts = input.ModelAlerts
	if input.Clear {
		cfg.Endpoint = nil
	}
	if input.URL != "" {
		if _, err := destination(input.URL); err != nil {
			return State{}, err
		}
		cfg.Endpoint, err = s.vault.SealWebhook(tenantID, []byte(input.URL))
		if err != nil {
			return State{}, err
		}
	}
	if cfg.Enabled && len(cfg.Endpoint) == 0 {
		return State{}, ErrInput
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return State{}, err
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if _, err := q.SaveAlertConfig(ctx, db.SaveAlertConfigParams{TenantID: tenantID, Config: string(encoded)}); err != nil {
		return State{}, err
	}
	// A new destination or enablement starts a new notification history; old pending events must not escape to it.
	if input.URL != "" || input.Clear || !input.Enabled {
		if err := q.ClearAlertStates(ctx, tenantID); err != nil {
			return State{}, err
		}
	}
	if err := audit.Record(ctx, q, "settings.update", "settings", "alerts"); err != nil {
		return State{}, err
	}
	if err := tx.Commit(); err != nil {
		return State{}, err
	}
	if cancel := s.flights[tenantID]; cancel != nil {
		cancel()
	}
	if cancel := s.testFlights[tenantID]; cancel != nil {
		cancel()
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return s.state(ctx, tenantID)
}

func newEventID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func (s *Service) observe(ctx context.Context, tenantID int64, extra []db.ListAlertSignalsRow, unknown map[string]bool) error {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	signals, err := q.ListAlertSignals(ctx, db.ListAlertSignalsParams{TenantID: tenantID, Since: s.now().Unix() - 300, Now: s.now().Unix()})
	if err != nil {
		return err
	}
	signals = append(signals, extra...)
	states, err := q.ListAlertStates(ctx, tenantID)
	if err != nil {
		return err
	}
	current := map[string]bool{}
	existing := map[string]db.AlertState{}
	for _, signal := range signals {
		key := signal.Kind + ":" + signal.Subject
		current[key] = true
		existing[key] = db.AlertState{TenantID: tenantID, Kind: signal.Kind, Subject: signal.Subject}
	}
	for _, state := range states {
		existing[state.Kind+":"+state.Subject] = state
	}
	for key, state := range existing {
		// Missing or stale observations cannot prove an incident has recovered.
		uncertain := unknown[key] || (state.Kind == "model_unavailable" && unknown["model_unavailable:"+strings.SplitN(state.Subject, ":", 2)[0]+":*"]) || (state.Kind == "quota_low" && unknown["quota_low:*"]) || (unknown["capacity_unavailable"] && (state.Kind == "model_unavailable" || state.Kind == "quota_low"))
		if uncertain && !current[key] {
			continue
		}
		active := int64(0)
		if current[key] {
			active = 1
		}
		if state.EventID != "" && state.Active == active {
			continue
		}
		state.Active = active
		state.EventID = newEventID()
		state.ChangedAt = s.now().Unix()
		state.Attempts = 0
		state.RetryAt = 0
		state.DeliveryFailed = 0
		if err := q.SaveAlertState(ctx, db.SaveAlertStateParams(state)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) Check(ctx context.Context) error {
	s.checking.Lock()
	defer s.checking.Unlock()
	rows, err := s.queries.ListAlertConfigs(ctx)
	if err != nil {
		return err
	}
	remaining := 10
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.checkWorkspace(ctx, row.ID, &remaining); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func (s *Service) checkWorkspace(ctx context.Context, tenantID int64, remaining *int) error {
	s.mu.Lock()
	raw, err := s.queries.GetAlertConfig(ctx, tenantID)
	var cfg configuration
	if err == nil {
		err = json.Unmarshal([]byte(raw), &cfg)
	}
	if err != nil || !cfg.Enabled {
		s.mu.Unlock()
		return err
	}
	plain, err := s.vault.OpenWebhook(tenantID, cfg.Endpoint)
	observer := s.capacityObserver
	s.mu.Unlock()
	extra, unknown, errCapacity := observeCapacity(ctx, observer, tenantID, cfg)
	s.mu.Lock()
	latest, changedErr := s.queries.GetAlertConfig(ctx, tenantID)
	if changedErr != nil || latest != raw {
		s.mu.Unlock()
		return changedErr
	}
	// Capacity observation failures retain those incidents while base signals still run.
	if errCapacity != nil {
		extra = nil
		unknown = map[string]bool{"capacity_unavailable": true}
	}
	if err == nil {
		err = s.observe(ctx, tenantID, extra, unknown)
	}
	var states []db.AlertState
	if err == nil {
		states, err = s.queries.ListAlertStates(ctx, tenantID)
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	for _, state := range states {
		if *remaining == 0 || state.Active == state.DeliveredActive || state.RetryAt > s.now().Unix() {
			continue
		}
		s.mu.Lock()
		latest, err := s.queries.GetAlertConfig(ctx, tenantID)
		if err != nil || latest != raw {
			s.mu.Unlock()
			return err
		}
		state.Attempts = min(state.Attempts+1, 10)
		state.RetryAt = s.now().Unix() + min(int64(60)<<(state.Attempts-1), 1800)
		err = s.queries.SaveAlertState(ctx, db.SaveAlertStateParams(state))
		if err != nil {
			s.mu.Unlock()
			return err
		}
		call, cancel := context.WithTimeout(ctx, 5*time.Second)
		s.flights[tenantID] = cancel
		s.mu.Unlock()
		*remaining = *remaining - 1
		status := "resolved"
		if state.Active != 0 {
			status = "firing"
		}
		err = deliver(call, s.sender, string(plain), Event{ID: state.EventID, WorkspaceID: tenantID, Kind: state.Kind, Subject: state.Subject, Status: status, ObservedAt: state.ChangedAt})
		cancel()
		s.mu.Lock()
		delete(s.flights, tenantID)
		// A configuration change clears the old event IDs. Late delivery results cannot acknowledge newer incidents.
		if err == nil {
			err = s.queries.FinishAlertDelivery(ctx, db.FinishAlertDeliveryParams{TenantID: tenantID, EventID: state.EventID, Now: s.now().Unix()})
		} else {
			err = s.queries.FailAlertDelivery(ctx, db.FailAlertDeliveryParams{TenantID: tenantID, EventID: state.EventID})
		}
		s.mu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Start(parent context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			_ = s.Check(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-s.wake:
			}
		}
	}()
}
func (s *Service) Close() {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	for _, stop := range s.testFlights {
		stop()
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	if closer, ok := s.sender.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
