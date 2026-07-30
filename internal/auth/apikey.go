package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	APIKeyPrefix = "pde_live_"
	// publicIDLen is the lookup prefix stored in DB (after APIKeyPrefix).
	publicIDLen = 12
	secretBytes = 24
)

// GenerateAPIKey returns plaintext secret, public prefix for DB lookup, and SHA-256 hex hash.
func GenerateAPIKey() (plaintext, prefix, hash string, err error) {
	id := make([]byte, publicIDLen/2)
	sec := make([]byte, secretBytes)
	if _, err = rand.Read(id); err != nil {
		return "", "", "", err
	}
	if _, err = rand.Read(sec); err != nil {
		return "", "", "", err
	}
	publicID := hex.EncodeToString(id) // 12 hex chars
	secret := hex.EncodeToString(sec)
	plaintext = APIKeyPrefix + publicID + "_" + secret
	prefix = APIKeyPrefix + publicID
	hash = HashAPIKey(plaintext)
	return plaintext, prefix, hash, nil
}

func HashAPIKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func LookupPrefix(plaintext string) (string, bool) {
	if !strings.HasPrefix(plaintext, APIKeyPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(plaintext, APIKeyPrefix)
	// expected: {12 hex}_{secret}
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) != 2 || len(parts[0]) != publicIDLen {
		return "", false
	}
	return APIKeyPrefix + parts[0], true
}

func ParseScopesCSV(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{ScopeExtractWrite}, nil
	}
	var out []string
	seen := map[string]struct{}{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch part {
		case ScopeExtractWrite, ScopeKeysManage, ScopeAdmin:
		default:
			return nil, fmt.Errorf("unknown scope %q", part)
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one scope is required")
	}
	return out, nil
}
