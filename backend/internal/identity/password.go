package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"tolerance/internal/platform/metrics"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 4
	argonKeyLen  = 32
)

// hashSem bounds how many argon2id operations (each allocating argonMemory
// KiB) run at once, so a burst of signups/logins cannot OOM the process.
// Default is runtime.NumCPU(); SetHashConcurrency overrides it, and must be
// called before serving traffic, not concurrently with hashing.
var hashSem = make(chan struct{}, max(1, runtime.NumCPU()))

// SetHashConcurrency replaces the default concurrency bound (runtime.NumCPU()).
func SetHashConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	hashSem = make(chan struct{}, n)
}

// acquireHashSlot blocks until a hashing slot is free, recording how long
// that took and how many operations are in flight.
func acquireHashSlot() {
	start := time.Now()
	hashSem <- struct{}{}
	metrics.PasswordHashWaitSeconds.Observe(time.Since(start).Seconds())
	metrics.PasswordHashInflight.Inc()
}

func releaseHashSlot() {
	metrics.PasswordHashInflight.Dec()
	<-hashSem
}

// HashPassword returns a self-describing argon2id string:
// $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash> (base64 without padding).
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	acquireHashSlot()
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	releaseHashSlot()
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads,
		enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches encoded. Malformed input
// is simply "does not match"; the caller never learns why.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil {
		return false
	}
	acquireHashSlot()
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	releaseHashSlot()
	return subtle.ConstantTimeCompare(got, want) == 1
}
