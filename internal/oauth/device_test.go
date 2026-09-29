package oauth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/upstream"
	"github.com/murongg/SubLane/internal/vault"
)

type fakeDeviceProvider struct {
	fakeProvider
	polls            int
	result           error
	entered, release chan struct{}
}

func (p *fakeDeviceProvider) BeginDevice(context.Context, string) (upstream.DeviceAuthorization, error) {
	return upstream.DeviceAuthorization{DeviceCode: "synthetic-private-device", UserCode: "MOCK-CODE", URL: "https://auth.x.ai/device", ExpiresIn: 600, Interval: 5}, nil
}
func (p *fakeDeviceProvider) ExchangeDevice(context.Context, string, string) (accounts.Credential, error) {
	p.polls++
	if p.entered != nil {
		close(p.entered)
		<-p.release
	}
	return accounts.Credential{Provider: "xai", AccountID: "synthetic-subject", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}, p.result
}
func deviceFlow(t *testing.T) (*Flow, *fakeDeviceProvider) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	identity, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "synthetic-admin", "synthetic-password", "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	key, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	p := &fakeDeviceProvider{}
	return New(accounts.New(db, key), p), p
}
func TestDeviceFlowSessionBoundPacedAndSingleUse(t *testing.T) {
	f, p := deviceFlow(t)
	ctx := context.Background()
	now := time.Now()
	f.now = func() time.Time { return now }
	a, err := f.BeginProvider(ctx, "xai", "synthetic-session", "Synthetic Grok", "")
	if err != nil || a.UserCode != "MOCK-CODE" || a.CallbackURL != "" {
		t.Fatal("device begin", err)
	}
	if _, err = f.PollDevice(ctx, "other-session", a.State); !errors.Is(err, ErrState) {
		t.Fatal("foreign session accepted", err)
	}
	pending, err := f.PollDevice(ctx, "synthetic-session", a.State)
	if err != nil || pending.Account != nil || p.polls != 0 {
		t.Fatal("early poll reached provider", err)
	}
	now = now.Add(5 * time.Second)
	p.result = &upstream.DevicePendingError{SlowDown: true}
	pending, err = f.PollDevice(ctx, "synthetic-session", a.State)
	if err != nil || pending.Interval != 10 || p.polls != 1 {
		t.Fatal("slow_down ignored", err)
	}
	now = now.Add(10 * time.Second)
	p.result = nil
	result, err := f.PollDevice(ctx, "synthetic-session", a.State)
	if err != nil || result.Account == nil || result.Account.Provider != "xai" {
		t.Fatal("account not saved", err)
	}
	if _, err = f.PollDevice(ctx, "synthetic-session", a.State); !errors.Is(err, ErrState) {
		t.Fatal("device replay accepted", err)
	}
}
func TestDeviceCancellationDuringExchangeCannotSaveAccount(t *testing.T) {
	f, p := deviceFlow(t)
	ctx := context.Background()
	now := time.Now()
	f.now = func() time.Time { return now }
	a, err := f.BeginProvider(ctx, "xai", "synthetic-session", "Synthetic Grok", "")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Second)
	p.entered = make(chan struct{})
	p.release = make(chan struct{})
	done := make(chan error, 1)
	go func() { _, err := f.PollDevice(ctx, "synthetic-session", a.State); done <- err }()
	<-p.entered
	if _, err = f.PollDevice(ctx, "synthetic-session", a.State); !errors.Is(err, ErrBusy) {
		t.Fatal("concurrent exchange allowed", err)
	}
	if err = f.Cancel("synthetic-session", a.State); err != nil {
		t.Fatal(err)
	}
	close(p.release)
	if err = <-done; !errors.Is(err, ErrState) {
		t.Fatal("cancelled device published", err)
	}
	rows, err := f.accounts.List(ctx)
	if err != nil || len(rows) != 0 {
		t.Fatal("cancelled account saved", err)
	}
}

func TestDeviceSlowDownKeepsIncreasingInterval(t *testing.T) {
	f, p := deviceFlow(t)
	ctx := context.Background()
	a, err := f.BeginProvider(ctx, "xai", "synthetic-session", "Synthetic Grok", "")
	if err != nil {
		t.Fatal(err)
	}
	entry := f.pending[a.State]
	entry.interval = 60
	entry.nextPoll = time.Time{}
	f.pending[a.State] = entry
	p.result = &upstream.DevicePendingError{SlowDown: true}
	result, err := f.PollDevice(ctx, "synthetic-session", a.State)
	if err != nil || result.Interval != 65 {
		t.Fatal("slow_down stopped increasing", result.Interval, err)
	}
}

func TestDeviceTerminalFailureAndExpiryDiscardAttempts(t *testing.T) {
	for _, failure := range []error{upstream.ErrDeviceDenied, upstream.ErrDeviceExpired} {
		f, p := deviceFlow(t)
		ctx := context.Background()
		now := time.Now()
		f.now = func() time.Time { return now }
		a, err := f.BeginProvider(ctx, "xai", "synthetic-session", "Synthetic Grok", "")
		if err != nil {
			t.Fatal(err)
		}
		now = now.Add(5 * time.Second)
		p.result = failure
		_, err = f.PollDevice(ctx, "synthetic-session", a.State)
		expected := ErrState
		if errors.Is(failure, upstream.ErrDeviceDenied) {
			expected = ErrDenied
		}
		if !errors.Is(err, expected) {
			t.Fatal("wrong terminal error", err)
		}
		if _, exists := f.pending[a.State]; exists {
			t.Fatal("terminal attempt retained")
		}
	}
	f, p := deviceFlow(t)
	ctx := context.Background()
	now := time.Now()
	f.now = func() time.Time { return now }
	a, err := f.BeginProvider(ctx, "xai", "synthetic-session", "Synthetic Grok", "")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(11 * time.Minute)
	if _, err := f.PollDevice(ctx, "synthetic-session", a.State); !errors.Is(err, ErrState) || p.polls != 0 {
		t.Fatal("expired attempt reached upstream", err)
	}
}
