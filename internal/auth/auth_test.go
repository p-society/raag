package auth

import (
	"testing"
	"time"

	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func mustPeerID(t *testing.T) peer.ID {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate identity key: %v", err)
	}

	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("derive peer id: %v", err)
	}
	return id
}

func TestGenerateKeyPair(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	if len(pub) != 32 {
		t.Errorf("public key length = %d, want 32", len(pub))
	}
	if len(priv) != 64 {
		t.Errorf("private key length = %d, want 64", len(priv))
	}
}

func TestDeriveKeyDeterministic(t *testing.T) {
	input := []byte("test-secret")
	salt := "test-salt"

	key1, err := DeriveKey(input, salt)
	if err != nil {
		t.Fatalf("DeriveKey() error = %v", err)
	}

	key2, err := DeriveKey(input, salt)
	if err != nil {
		t.Fatalf("DeriveKey() error = %v", err)
	}
	if string(key1) != string(key2) {
		t.Error("DeriveKey() should be deterministic for same input")
	}
}

func TestDeriveKeyDifferentSalts(t *testing.T) {
	input := []byte("test-secret")
	key1, err := DeriveKey(input, "salt1")
	if err != nil {
		t.Fatalf("DeriveKey() error = %v", err)
	}

	key2, err := DeriveKey(input, "salt2")
	if err != nil {
		t.Fatalf("DeriveKey() error = %v", err)
	}
	if string(key1) == string(key2) {
		t.Error("DeriveKey() should produce different keys for different salts")
	}
}

func TestDeriveKeyProducesValidKey(t *testing.T) {
	key, err := DeriveKey([]byte("secret"), "salt")
	if err != nil {
		t.Fatalf("DeriveKey() error = %v", err)
	}
	if len(key) != 64 {
		t.Errorf("derived key length = %d, want 64", len(key))
	}

	pub := key.Public()
	if pub == nil {
		t.Error("derived key should have a public key")
	}
}

func TestGetPublicKeyHex(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	hex := GetPublicKeyHex(priv)
	if len(hex) != 64 {
		t.Errorf("hex length = %d, want 64", len(hex))
	}
}

func TestGenerateToken(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	id := mustPeerID(t)
	token, err := GenerateToken(id, priv)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if token.PeerID != id.String() {
		t.Errorf("token.PeerID = %q, want %q", token.PeerID, id.String())
	}
	if token.Nonce == "" {
		t.Error("token.Nonce should not be empty")
	}
	if token.Signature == "" {
		t.Error("token.Signature should not be empty")
	}
	if token.ExpiresAt <= token.Timestamp {
		t.Error("token.ExpiresAt should be after token.Timestamp")
	}
}

func TestGenerateTokenUniqueNonces(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	id := mustPeerID(t)
	nonces := make(map[string]bool)
	for i := range 100 {
		token, err := GenerateToken(id, priv)
		if err != nil {
			t.Fatalf("GenerateToken() error = %v", err)
		}
		if nonces[token.Nonce] {
			t.Errorf("GenerateToken() produced duplicate nonce at iteration %d", i)
		}
		nonces[token.Nonce] = true
	}
}

func TestVerifyTokenValid(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	id := mustPeerID(t)
	token, err := GenerateToken(id, priv)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	valid, err := VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken() error = %v", err)
	}
	if !valid {
		t.Error("VerifyToken() should return true for valid token")
	}
}

func TestVerifyTokenEmptyPeerID(t *testing.T) {
	token := &AuthToken{
		PeerID:    "",
		PublicKey: "abc123",
		Signature: "def456",
	}

	_, err := VerifyToken(token)
	if err == nil {
		t.Error("VerifyToken() should error on empty peer ID")
	}
}

func TestVerifyTokenEmptySignature(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	token := &AuthToken{
		PeerID:    "test",
		PublicKey: GetPublicKeyHex(priv),
		Signature: "",
	}

	_, err = VerifyToken(token)
	if err == nil {
		t.Error("VerifyToken() should error on empty signature")
	}
}

func TestVerifyTokenExpired(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	pubHex := GetPublicKeyHex(priv)
	now := time.Now()
	token := &AuthToken{
		PeerID:    "test",
		PublicKey: pubHex,
		Signature: "abc",
		Timestamp: now.Add(-48 * time.Hour).Unix(),
		ExpiresAt: now.Add(-24 * time.Hour).Unix(),
		Nonce:     "nonce",
	}

	_, err = VerifyToken(token)
	if err == nil {
		t.Error("VerifyToken() should error on expired token")
	}
}

func TestVerifyTokenFutureTimestamp(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	pubHex := GetPublicKeyHex(priv)
	now := time.Now()
	token := &AuthToken{
		PeerID:    "test",
		PublicKey: pubHex,
		Signature: "abc",
		Timestamp: now.Add(2 * time.Minute).Unix(),
		ExpiresAt: now.Add(24 * time.Hour).Unix(),
		Nonce:     "nonce",
	}

	_, err = VerifyToken(token)
	if err == nil {
		t.Error("VerifyToken() should error on token from the future")
	}
}

func TestVerifyTokenInvalidSignature(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	pubHex := GetPublicKeyHex(priv)
	now := time.Now()
	token := &AuthToken{
		PeerID:    "test",
		PublicKey: pubHex,
		Signature: "invalid-signature-that-is-not-base64",
		Timestamp: now.Unix(),
		ExpiresAt: now.Add(24 * time.Hour).Unix(),
		Nonce:     "nonce",
	}

	_, err = VerifyToken(token)
	if err == nil {
		t.Error("VerifyToken() should error on invalid signature")
	}
}

func TestVerifyTokenWrongPublicKey(t *testing.T) {
	_, priv1, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	_, priv2, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	id := mustPeerID(t)
	token, err := GenerateToken(id, priv1)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	token.PublicKey = GetPublicKeyHex(priv2)
	_, err = VerifyToken(token)
	if err == nil {
		t.Error("VerifyToken() should error when public key doesn't match signature")
	}
}

func TestSerializeDeserializeTokenRoundTrip(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	id := mustPeerID(t)
	token, err := GenerateToken(id, priv)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	serialized, err := SerializeToken(token)
	if err != nil {
		t.Fatalf("SerializeToken() error = %v", err)
	}
	if serialized == "" {
		t.Error("SerializeToken() should not return empty string")
	}

	deserialized, err := DeserializeToken(serialized)
	if err != nil {
		t.Fatalf("DeserializeToken() error = %v", err)
	}
	if deserialized.PeerID != token.PeerID {
		t.Errorf("PeerID = %q, want %q", deserialized.PeerID, token.PeerID)
	}
	if deserialized.Nonce != token.Nonce {
		t.Errorf("Nonce = %q, want %q", deserialized.Nonce, token.Nonce)
	}
	if deserialized.Signature != token.Signature {
		t.Errorf("Signature = %q, want %q", deserialized.Signature, token.Signature)
	}
}

func TestDeserializeTokenInvalidBase64(t *testing.T) {
	_, err := DeserializeToken("not-valid-base64!!!")
	if err == nil {
		t.Error("DeserializeToken() should error on invalid base64")
	}
}

func TestDeserializeTokenInvalidJSON(t *testing.T) {
	invalid := "YWJjZGVm"
	_, err := DeserializeToken(invalid)
	if err == nil {
		t.Error("DeserializeToken() should error on invalid JSON")
	}
}

func TestTrustedKeyManagerAddAndIsTrusted(t *testing.T) {
	mgr := NewTrustedKeyManager()
	key1 := "abc123"
	mgr.AddKey(key1)

	if !mgr.IsTrusted(key1) {
		t.Error("IsTrusted() should return true for added key")
	}

	key2 := "xyz789"
	if mgr.IsTrusted(key2) {
		t.Error("IsTrusted() should return false for non-added key")
	}

	mgr.AddKey(key1)
	mgr.AddKey(key1)
	if len(mgr.trustedKeys) != 1 {
		t.Errorf("AddKey() should not add duplicate keys, got %d want 1", len(mgr.trustedKeys))
	}
}

func TestTrustedKeyManagerVerifyAndCheckTrust(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	pubHex := GetPublicKeyHex(priv)
	mgr := NewTrustedKeyManager()
	mgr.AddKey(pubHex)

	id := mustPeerID(t)
	token, err := GenerateToken(id, priv)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	valid, err := mgr.VerifyAndCheckTrust(token)
	if err != nil {
		t.Fatalf("VerifyAndCheckTrust() error = %v", err)
	}
	if !valid {
		t.Error("VerifyAndCheckTrust() should return true for valid trusted token")
	}
}

func TestTrustedKeyManagerUntrustedKey(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	id := mustPeerID(t)
	token, err := GenerateToken(id, priv)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	mgr := NewTrustedKeyManager()
	_, err = mgr.VerifyAndCheckTrust(token)
	if err == nil {
		t.Error("VerifyAndCheckTrust() should error for untrusted key")
	}
}

func TestTrustedKeyManagerMultipleKeys(t *testing.T) {
	_, priv1, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	_, priv2, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	mgr := NewTrustedKeyManager()
	mgr.AddKey(GetPublicKeyHex(priv1))
	mgr.AddKey(GetPublicKeyHex(priv2))

	id1 := mustPeerID(t)
	id2 := mustPeerID(t)

	token1, _ := GenerateToken(id1, priv1)
	token2, _ := GenerateToken(id2, priv2)

	valid1, _ := mgr.VerifyAndCheckTrust(token1)
	valid2, _ := mgr.VerifyAndCheckTrust(token2)
	if !valid1 || !valid2 {
		t.Error("Both keys should be trusted")
	}
}

func TestTokenAuthIntegration(t *testing.T) {
	_, issuerPriv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	peerID := mustPeerID(t)
	trustedKeyHex := GetPublicKeyHex(issuerPriv)
	mgr := NewTrustedKeyManager()
	mgr.AddKey(trustedKeyHex)

	token, err := GenerateToken(peerID, issuerPriv)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	valid, err := mgr.VerifyAndCheckTrust(token)
	if err != nil {
		t.Fatalf("VerifyAndCheckTrust() error = %v", err)
	}
	if !valid {
		t.Fatal("Full auth flow should succeed")
	}
}
