package keyring

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreatePersistsKey(t *testing.T) {
	dir := t.TempDir()
	k1, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 || fi.Size() != keySize {
		t.Fatalf("key file mode %v size %d", fi.Mode().Perm(), fi.Size())
	}
	ct := k1.Seal("refresh", []byte("secret-token"))
	k2, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	pt, err := k2.Open("refresh", ct)
	if err != nil || string(pt) != "secret-token" {
		t.Fatalf("round trip after reload: %q %v", pt, err)
	}
}

func TestSealOpen(t *testing.T) {
	k, _ := New(bytes.Repeat([]byte{7}, 32))
	other, _ := New(bytes.Repeat([]byte{8}, 32))
	ct := k.Seal("a", []byte("hello"))
	if bytes.Contains(ct, []byte("hello")) {
		t.Fatal("plaintext visible in ciphertext")
	}
	if ct2 := k.Seal("a", []byte("hello")); bytes.Equal(ct, ct2) {
		t.Fatal("nonce reuse: identical ciphertexts")
	}
	if pt, err := k.Open("a", ct); err != nil || string(pt) != "hello" {
		t.Fatalf("open: %q %v", pt, err)
	}
	if _, err := k.Open("b", ct); !errors.Is(err, ErrDecrypt) {
		t.Error("wrong purpose must fail")
	}
	if _, err := other.Open("a", ct); !errors.Is(err, ErrDecrypt) {
		t.Error("wrong key must fail")
	}
	bad := bytes.Clone(ct)
	bad[len(bad)-1] ^= 1
	if _, err := k.Open("a", bad); !errors.Is(err, ErrDecrypt) {
		t.Error("tampering must fail")
	}
	if _, err := k.Open("a", ct[:5]); !errors.Is(err, ErrDecrypt) {
		t.Error("short input must fail")
	}
	if bytes.Equal(k.MAC("x", []byte("d")), k.MAC("y", []byte("d"))) {
		t.Error("MAC must depend on purpose")
	}
}

func TestRejectsWrongSizedKeyFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte("short"), 0o600)
	if _, err := LoadOrCreate(dir); err == nil {
		t.Fatal("expected error")
	}
}
