// Package capacity defines local, observed pool capacity without provider IO.
package capacity

type Reason struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}
type Account struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Provider         string   `json:"provider"`
	Reason           string   `json:"reason"`
	RetryAt          int64    `json:"retry_at"`
	QuotaState       string   `json:"quota_state"`
	QuotaUsedPercent *float64 `json:"quota_used_percent,omitempty"`
	InFlight         int64    `json:"in_flight"`
	MaxConcurrency   int64    `json:"max_concurrency"`
}
type Model struct {
	Model      string    `json:"model"`
	State      string    `json:"state"`
	Available  int       `json:"available"`
	Unknown    int       `json:"unknown"`
	RetryAt    int64     `json:"retry_at"`
	ServerTime int64     `json:"server_time"`
	Reasons    []Reason  `json:"reasons"`
	Accounts   []Account `json:"accounts,omitempty"`
}
type Pool struct {
	ID      int64
	Models  []Model
	Unknown bool
}
