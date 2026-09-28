// Package store persists gateway records in PostgreSQL. Password helpers hash dashboard passwords so plaintext is never stored.
package store

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"golang.org/x/crypto/bcrypt"
	"sync"
)

var logTraceOncePassword sync.Once

// HashPassword returns a bcrypt hash. On failure the caller must not store the plaintext password.
func HashPassword(plain string) (string, error) {
	logTraceOncePassword.Do(func() { logx.Trace("enter store.HashPassword") })

	if plain == "" {
		return "", nil
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword reports whether plain matches hash. A malformed hash returns false rather than an error.
func CheckPassword(hash, plain string) bool {
	if hash == "" || plain == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
