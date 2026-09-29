package accounts

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"maps"
	"strings"
)

func prepareClaudeIdentity(c Credential, deviceID string) (Credential, bool, error) {
	if c.Kind() != "claude" {
		return c, false, nil
	}
	if deviceID == "" {
		deviceID = claudeDeviceID(c.Metadata)
	}
	if deviceID == "" {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return Credential{}, false, err
		}
		deviceID = hex.EncodeToString(raw)
	}
	// The pinned executor uses one canonical device per credential. Store it on
	// the account so temporary execution Auth values cannot create new devices.
	encoded, err := json.Marshal([]string{deviceID})
	if err != nil {
		return Credential{}, false, err
	}
	if bytes.Equal(c.Metadata["claude_device_ids"], encoded) {
		return c, false, nil
	}
	c.Metadata = maps.Clone(c.Metadata)
	if c.Metadata == nil {
		c.Metadata = make(map[string]json.RawMessage)
	}
	c.Metadata["claude_device_ids"] = encoded
	return c, true, nil
}

func claudeDeviceID(metadata map[string]json.RawMessage) string {
	var devices []string
	if json.Unmarshal(metadata["claude_device_ids"], &devices) != nil {
		return ""
	}
	for _, value := range devices {
		value = strings.ToLower(strings.TrimSpace(value))
		if len(value) != 64 {
			continue
		}
		if raw, err := hex.DecodeString(value); err == nil && len(raw) == 32 {
			return value
		}
	}
	return ""
}
