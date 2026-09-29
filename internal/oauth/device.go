package oauth

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/upstream"
)

type deviceProvider interface {
	BeginDevice(context.Context, string) (upstream.DeviceAuthorization, error)
	ExchangeDevice(context.Context, string, string) (accounts.Credential, error)
}

type DeviceResult struct {
	Account  *accounts.Account `json:"account,omitempty"`
	Interval int64             `json:"interval"`
}

func (f *Flow) beginDevice(ctx context.Context, session, state string, entry pending) (Authorization, error) {
	provider, ok := f.provider.(deviceProvider)
	if !ok {
		_ = f.Cancel(session, state)
		return Authorization{}, accounts.ErrInput
	}
	address, err := f.accounts.ProxyURL(ctx, entry.proxyID)
	if err != nil {
		_ = f.Cancel(session, state)
		return Authorization{}, err
	}
	device, err := provider.BeginDevice(upstream.WithProxyURL(ctx, address), entry.provider)
	if err != nil {
		_ = f.Cancel(session, state)
		return Authorization{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	current, ok := f.pending[state]
	if !ok || !current.expires.After(f.now()) {
		delete(f.pending, state)
		return Authorization{}, ErrState
	}
	entry.device = device.DeviceCode
	entry.interval = max(5, device.Interval)
	entry.nextPoll = f.now().Add(time.Duration(entry.interval) * time.Second)
	// Never extend an attempt past the local session-bound ten-minute lifetime.
	entry.expires = minTime(entry.expires, f.now().Add(time.Duration(device.ExpiresIn)*time.Second))
	f.pending[state] = entry
	target := device.URL
	if device.CompleteURL != "" {
		target = device.CompleteURL
	}
	return Authorization{URL: target, State: state, UserCode: device.UserCode, Interval: entry.interval, ExpiresAt: entry.expires.Unix()}, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (f *Flow) PollDevice(ctx context.Context, session, state string) (DeviceResult, error) {
	owner := sha256.Sum256([]byte(session))
	f.mu.Lock()
	entry, ok := f.pending[state]
	if !ok || session == "" || entry.owner != owner || !entry.expires.After(f.now()) || entry.device == "" {
		if ok && !entry.expires.After(f.now()) {
			delete(f.pending, state)
		}
		f.mu.Unlock()
		return DeviceResult{}, ErrState
	}
	if !f.accounts.ProviderEnabled(entry.provider) {
		delete(f.pending, state)
		f.mu.Unlock()
		return DeviceResult{}, accounts.ErrProviderDisabled
	}
	if entry.polling {
		f.mu.Unlock()
		return DeviceResult{}, ErrBusy
	}
	if f.now().Before(entry.nextPoll) {
		f.mu.Unlock()
		return DeviceResult{Interval: entry.interval}, nil
	}
	entry.polling = true
	f.pending[state] = entry
	f.mu.Unlock()
	credential, err := f.exchangeDevice(ctx, entry)
	f.mu.Lock()
	defer f.mu.Unlock()
	// Cancellation, expiry or a replacement login must win even after successful provider IO.
	current, ok := f.pending[state]
	if !ok || !current.expires.After(f.now()) {
		delete(f.pending, state)
		return DeviceResult{}, ErrState
	}
	var pending *upstream.DevicePendingError
	if errors.As(err, &pending) {
		if pending.SlowDown {
			entry.interval = min(600, entry.interval+5)
		}
		entry.polling = false
		entry.nextPoll = f.now().Add(time.Duration(entry.interval) * time.Second)
		f.pending[state] = entry
		return DeviceResult{Interval: entry.interval}, nil
	}
	delete(f.pending, state)
	if errors.Is(err, upstream.ErrDeviceDenied) {
		return DeviceResult{}, ErrDenied
	}
	if errors.Is(err, upstream.ErrDeviceExpired) {
		return DeviceResult{}, ErrState
	}
	if err != nil {
		return DeviceResult{}, err
	}
	if ctx.Err() != nil {
		return DeviceResult{}, ctx.Err()
	}
	if credential.Kind() != entry.provider {
		return DeviceResult{}, accounts.ErrIdentity
	}
	account, err := f.accounts.AuthorizeWithProxy(ctx, entry.name, credential, entry.replaceID, entry.proxyID)
	if err != nil {
		return DeviceResult{}, err
	}
	return DeviceResult{Account: &account}, nil
}

func (f *Flow) exchangeDevice(ctx context.Context, entry pending) (accounts.Credential, error) {
	address, err := f.accounts.ProxyURL(ctx, entry.proxyID)
	if err != nil {
		return accounts.Credential{}, err
	}
	return f.provider.(deviceProvider).ExchangeDevice(upstream.WithProxyURL(ctx, address), entry.provider, entry.device)
}
