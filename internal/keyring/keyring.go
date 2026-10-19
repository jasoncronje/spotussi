// Package keyring holds the instance key (DATA_DIR/key) and encrypts small
// secrets with it (D12: the Spotify refresh token is encrypted at rest).
//
// The key is 32 random bytes created on first run with mode 0600. It lives
// next to the database rather than in it, so a copy of the database file
// (a backup, a Datasette exposure) does not reveal the secrets. Losing the
// key only means reconnecting Spotify; nothing else depends on it.
package keyring

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// FileName is the key file inside the data directory.
const FileName = "key"

const (
	keySize = 32
	version = 1 // first byte of every ciphertext
)

// ErrDecrypt means a ciphertext could not be opened (wrong key, tampering,
// or a different purpose).
var ErrDecrypt = errors.New("keyring: decryption failed")

// Keyring encrypts and authenticates with the instance key.
type Keyring struct {
	aead cipher.AEAD
	mac  []byte // subkey for MACs, independent of the encryption subkey
}

// LoadOrCreate reads DATA_DIR/key, creating it on first run.
func LoadOrCreate(dataDir string) (*Keyring, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	path := filepath.Join(dataDir, FileName)
	key, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		key, err = create(path)
	}
	if err != nil {
		return nil, err
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("%s: expected %d bytes, found %d; restore the original key or delete it (Spotify must then be reconnected)", path, keySize, len(key))
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode().Perm()&0o077 != 0 {
		slog.Warn("key file is readable by other users; tightening to 0600", "path", path, "mode", fi.Mode().Perm())
		_ = os.Chmod(path, 0o600)
	}
	return New(key)
}

func create(path string) ([]byte, error) {
	key := make([]byte, keySize)
	rand.Read(key)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) { // lost a race with another process
		return os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("create key: %w", err)
	}
	if _, err := f.Write(key); err != nil {
		f.Close()
		return nil, fmt.Errorf("write key: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, fmt.Errorf("sync key: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	slog.Info("generated instance key", "path", path)
	return key, nil
}

// New builds a Keyring from a 32-byte master key.
func New(master []byte) (*Keyring, error) {
	if len(master) != keySize {
		return nil, fmt.Errorf("keyring: key must be %d bytes", keySize)
	}
	block, err := aes.NewCipher(derive(master, "spotussi/encrypt/v1"))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Keyring{aead: aead, mac: derive(master, "spotussi/mac/v1")}, nil
}

func derive(master []byte, label string) []byte {
	m := hmac.New(sha256.New, master)
	m.Write([]byte(label))
	return m.Sum(nil)
}

// Seal encrypts plaintext with AES-256-GCM. purpose is bound as additional
// data, so a ciphertext sealed for one purpose cannot be opened as another.
// Layout: version (1 byte) | nonce (12) | ciphertext+tag.
func (k *Keyring) Seal(purpose string, plaintext []byte) []byte {
	out := make([]byte, 1+k.aead.NonceSize(), 1+k.aead.NonceSize()+len(plaintext)+k.aead.Overhead())
	out[0] = version
	rand.Read(out[1:])
	return k.aead.Seal(out, out[1:], plaintext, []byte(purpose))
}

// Open decrypts a value produced by Seal with the same purpose.
func (k *Keyring) Open(purpose string, ciphertext []byte) ([]byte, error) {
	ns := k.aead.NonceSize()
	if len(ciphertext) < 1+ns+k.aead.Overhead() || ciphertext[0] != version {
		return nil, ErrDecrypt
	}
	pt, err := k.aead.Open(nil, ciphertext[1:1+ns], ciphertext[1+ns:], []byte(purpose))
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// MAC returns HMAC-SHA256 of data under a purpose-specific key.
func (k *Keyring) MAC(purpose string, data []byte) []byte {
	m := hmac.New(sha256.New, k.mac)
	m.Write([]byte(purpose))
	m.Write([]byte{0})
	m.Write(data)
	return m.Sum(nil)
}
