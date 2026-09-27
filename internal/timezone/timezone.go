// Package timezone owns the instance-wide civil time zone setting.
package timezone

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	_ "time/tzdata"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
)

var ErrInput = errors.New("invalid_time_zone")

type Service struct {
	conn    *sql.DB
	queries *db.Queries
	mu      sync.Mutex
	current atomic.Pointer[time.Location]
}

func location(name string) (*time.Location, error) {
	if name == "" || name == "Local" || len(name) > 64 || strings.TrimSpace(name) != name {
		return nil, ErrInput
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, ErrInput
	}
	return loc, nil
}

func New(ctx context.Context, conn *sql.DB) (*Service, error) {
	s := &Service{conn: conn, queries: db.New(conn)}
	name, err := s.queries.GetTimeZone(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		name, err = "UTC", nil
	}
	if err != nil {
		return nil, err
	}
	loc, err := location(name)
	if err != nil {
		return nil, err
	}
	s.current.Store(loc)
	return s, nil
}

func (s *Service) Location() *time.Location { return s.current.Load() }
func (s *Service) Name() string             { return s.Location().String() }

func (s *Service) Save(ctx context.Context, name string) (string, error) {
	loc, err := location(name)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == s.Name() {
		return name, nil
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if err := q.SaveTimeZone(ctx, name); err != nil {
		return "", err
	}
	if err := audit.Record(ctx, q, "settings.update", "settings", "timezone"); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	// A request sees either the old or new fully persisted setting.
	s.current.Store(loc)
	return name, nil
}
