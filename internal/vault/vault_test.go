package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"os"
	"path/filepath"
	"testing"
)

func TestWebhookEncryptionBindsWorkspaceAndResource(t *testing.T) {
	v, err := Open(filepath.Join(t.TempDir(), "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := v.SealWebhook(1, []byte("https://hooks.example.test/synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.OpenWebhook(1, sealed); err != nil {
		t.Fatal(err)
	}
	if _, err := v.OpenWebhook(2, sealed); err == nil {
		t.Fatal("workspace ciphertext moved")
	}
	if _, err := v.Open("1", sealed); err == nil {
		t.Fatal("webhook decoded as account")
	}
}

func TestEncryptionBindsCredentialsToAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.key")
	v, err := Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte(`{"access_token":"synthetic-access","refresh_token":"synthetic-refresh"}`)
	encrypted, err := v.Seal("account-test", plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, plain) {
		t.Fatal("plaintext persisted")
	}
	second, err := v.Seal("account-test", plain)
	if err != nil || bytes.Equal(encrypted, second) {
		t.Fatal("nonce reused")
	}
	reopened, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := reopened.Open("account-test", encrypted)
	if err != nil || !bytes.Equal(decoded, plain) {
		t.Fatal("credentials did not survive reopen", err)
	}
	if _, err := reopened.Open("other-account", encrypted); err == nil {
		t.Fatal("credentials moved to another account")
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, err := reopened.Open("account-test", encrypted); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("key permissions", err)
	}
}

func TestMissingOrCorruptKeyFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.key")
	if _, err := Open(path, false); err == nil {
		t.Fatal("missing key replaced despite existing credentials")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("missing key must not be created")
	}
	if err := os.WriteFile(path, []byte("invalid-key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, true); err == nil {
		t.Fatal("corrupt key accepted or replaced")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "invalid-key" {
		t.Fatal("corrupt key was overwritten")
	}
}

func TestAPIKeysBindOwnerRecordAndDomain(t *testing.T) {
	v, err := Open(filepath.Join(t.TempDir(), "credentials.key"), true)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("synthetic-key")
	encrypted, err := v.SealAPIKey(2, 3, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.OpenAPIKey(2, 3, encrypted)
	if err != nil || !bytes.Equal(plain, got) {
		t.Fatal(err)
	}
	for _, ids := range [][2]int64{{1, 3}, {2, 4}, {0, 3}} {
		if _, err := v.OpenAPIKey(ids[0], ids[1], encrypted); err == nil {
			t.Fatal("moved key accepted")
		}
	}
	if _, err := v.Open("api-key:2:3", encrypted); err == nil {
		t.Fatal("ciphertext crossed credential domains")
	}
	if _, err := v.SealAPIKey(0, 3, plain); err == nil {
		t.Fatal("invalid owner accepted")
	}
}

func TestExistingAccountEnvelopeRemainsReadable(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	nonce := bytes.Repeat([]byte{9}, 12)
	path := filepath.Join(t.TempDir(), "credentials.key")
	if err := os.WriteFile(path, key, 0600); err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	// Construct the previously released envelope independently of Vault's helpers.
	encrypted := aead.Seal(append([]byte{1}, nonce...), nonce, []byte("synthetic-credential"), []byte("sublane:account:synthetic-account"))
	reopened, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := reopened.Open("synthetic-account", encrypted)
	if err != nil || string(plain) != "synthetic-credential" {
		t.Fatal("existing credential format changed", err)
	}
}
