package protocol

import (
	"bytes"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	var ks [KeySize]byte
	ks[0] = 7
	sel := []byte{0xAA, 0xBB, 0xCC}
	pt := []byte("hello chat body")
	sealed := SealChat(ks, sel, 5, pt)
	if len(sealed) != len(pt)+ChatSealTagSize {
		t.Fatalf("sealed len = %d", len(sealed))
	}
	got, err := OpenChat(ks, sel, 5, sealed)
	if err != nil || !bytes.Equal(got, pt) {
		t.Fatalf("open: %v %q", err, got)
	}
	// Deterministic for the same inputs.
	if !bytes.Equal(sealed, SealChat(ks, sel, 5, pt)) {
		t.Fatal("seal not deterministic")
	}
	// Wrong counter, wrong selector, tampered byte, wrong key all reject.
	if _, err := OpenChat(ks, sel, 6, sealed); err == nil {
		t.Fatal("wrong counter accepted")
	}
	if _, err := OpenChat(ks, []byte{1, 2, 3}, 5, sealed); err == nil {
		t.Fatal("wrong selector accepted")
	}
	bad := append([]byte(nil), sealed...)
	bad[0] ^= 1
	if _, err := OpenChat(ks, sel, 5, bad); err == nil {
		t.Fatal("tamper accepted")
	}
	var ks2 [KeySize]byte
	ks2[0] = 9
	if _, err := OpenChat(ks2, sel, 5, sealed); err == nil {
		t.Fatal("wrong key accepted")
	}
}

func TestSessionKeyAgreement(t *testing.T) {
	eph, _ := GenerateEphemeralKey()
	ek, _ := GenerateEphemeralKey()
	var qk [KeySize]byte
	qk[1] = 3
	cli, err := ChatSessionKey(eph, ek.PublicKey().Bytes(), ChatProtocolVersion, qk)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ChatSessionKey(ek, eph.PublicKey().Bytes(), ChatProtocolVersion, qk)
	if err != nil {
		t.Fatal(err)
	}
	if cli != srv {
		t.Fatal("client/server session keys differ")
	}
	// A different passphrase yields a different session key (passphrase gate).
	var qk2 [KeySize]byte
	qk2[1] = 4
	other, _ := ChatSessionKey(ek, eph.PublicKey().Bytes(), ChatProtocolVersion, qk2)
	if other == srv {
		t.Fatal("passphrase not mixed into the session key")
	}
}

// TestSessionKeyVersionBinding proves the handshake's cleartext protocol-version
// byte is tamper-evident: it is bound into the session key, so a flipped byte
// (a passphrase-knowing on-path attacker trying to downgrade) yields a different
// key and the sealed bootstrap fails to open — fail-closed, not exploitable.
func TestSessionKeyVersionBinding(t *testing.T) {
	eph, _ := GenerateEphemeralKey()
	ek, _ := GenerateEphemeralKey()
	var qk [KeySize]byte
	qk[2] = 9
	k1, _ := ChatSessionKey(eph, ek.PublicKey().Bytes(), 1, qk)
	k2, _ := ChatSessionKey(eph, ek.PublicKey().Bytes(), 2, qk)
	if k1 == k2 {
		t.Fatal("protocol version not mixed into the session key")
	}
	// Client sealed its bootstrap under version 1; an attacker flips the wire
	// byte to 2, so the server derives under version 2. It must NOT open.
	sel := []byte{0x99, 0x01, 0x02}
	sealed := SealChat(k1, sel, ChatBootstrapCounter(), []byte("bootstrap"))
	if _, err := OpenChat(k2, sel, ChatBootstrapCounter(), sealed); err == nil {
		t.Fatal("version downgrade opened a mismatched-version bootstrap")
	}
}
