package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params are argon2id cost parameters.
type Params struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
}

// DefaultParams follow the OWASP minimum for argon2id (19 MiB, t=2, p=1):
// ~30 ms per hash on a small server, and a modest memory peak given the
// < 60 MB idle budget. Hashes record their parameters, so these can be
// raised later without invalidating stored passwords.
var DefaultParams = Params{MemoryKiB: 19 * 1024, Time: 2, Threads: 1}

const (
	saltLen = 16
	hashLen = 32

	// MinPasswordLen and MaxPasswordLen bound owner passwords (bytes).
	MinPasswordLen = 8
	MaxPasswordLen = 1024
)

var errMalformedHash = errors.New("auth: malformed password hash")

// hashPassword returns a PHC-format argon2id string:
// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>
func hashPassword(password string, p Params) string {
	salt := make([]byte, saltLen)
	rand.Read(salt)
	sum := argon2.IDKey([]byte(password), salt, p.Time, p.MemoryKiB, p.Threads, hashLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.MemoryKiB, p.Time, p.Threads, enc.EncodeToString(salt), enc.EncodeToString(sum))
}

// verifyPassword checks password against a hash from hashPassword in
// constant time.
func verifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errMalformedHash
	}
	var v int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil || v != argon2.Version {
		return false, errMalformedHash
	}
	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.MemoryKiB, &p.Time, &p.Threads); err != nil ||
		p.MemoryKiB == 0 || p.Time == 0 || p.Threads == 0 {
		return false, errMalformedHash
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false, errMalformedHash
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, errMalformedHash
	}
	got := argon2.IDKey([]byte(password), salt, p.Time, p.MemoryKiB, p.Threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
