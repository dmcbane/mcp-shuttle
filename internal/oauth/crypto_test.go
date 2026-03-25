//go:build mcp_go_client_oauth

package oauth

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := deriveKey("test-secret", "test-salt")
	plaintext := []byte(`{"access_token":"secret-token-123","token_type":"Bearer"}`)

	ciphertext, err := encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext should differ from plaintext")
	}

	decrypted, err := decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted != plaintext\ngot:  %s\nwant: %s", decrypted, plaintext)
	}
}

func TestEncryptDecrypt_WrongKey(t *testing.T) {
	key1 := deriveKey("secret-1", "salt")
	key2 := deriveKey("secret-2", "salt")
	plaintext := []byte("sensitive data")

	ciphertext, err := encrypt(key1, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	_, err = decrypt(key2, ciphertext)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}
}

func TestEncryptDecrypt_DifferentCiphertextsEachTime(t *testing.T) {
	key := deriveKey("test", "salt")
	plaintext := []byte("same input")

	ct1, _ := encrypt(key, plaintext)
	ct2, _ := encrypt(key, plaintext)

	if bytes.Equal(ct1, ct2) {
		t.Fatal("encrypting the same plaintext twice should produce different ciphertexts (random nonce)")
	}
}

func TestDeriveKey_Deterministic(t *testing.T) {
	key1 := deriveKey("secret", "salt")
	key2 := deriveKey("secret", "salt")

	if !bytes.Equal(key1, key2) {
		t.Fatal("deriveKey should be deterministic for same inputs")
	}
}

func TestDeriveKey_DifferentInputs(t *testing.T) {
	key1 := deriveKey("secret-a", "salt")
	key2 := deriveKey("secret-b", "salt")

	if bytes.Equal(key1, key2) {
		t.Fatal("different secrets should produce different keys")
	}
}

func TestMachineSecret_NotEmpty(t *testing.T) {
	secret := machineSecret()
	if secret == "" {
		t.Fatal("machineSecret() should return a non-empty string")
	}
}
