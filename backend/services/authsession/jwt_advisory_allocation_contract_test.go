package authsession

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTAdvisorySeparatorAllocationBudget(t *testing.T) {
	for _, secret := range []struct{ name, value string }{
		{"custom", "advisory-test-only-custom-secret"},
		{"legacy-default-compatibility", legacyDefaultKey},
	} {
		t.Run(secret.name, func(t *testing.T) {
			// No real credential and no database: malformed framing must be
			// rejected before any identity lookup or session mutation.
			input := strings.Repeat(".", 32<<10)
			service := New(nil, secret.value)
			for i := 0; i < 4; i++ {
				if _, err := service.Authenticate(context.Background(), input); !errors.Is(err, ErrInvalidSession) {
					t.Fatalf("malformed framing was not safely rejected: %v", err)
				}
			}
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			const attempts = 16
			for i := 0; i < attempts; i++ {
				identity, err := service.Authenticate(context.Background(), input)
				if !errors.Is(err, ErrInvalidSession) || identity.UserID != 0 {
					t.Fatalf("malformed framing reached identity work: %v", err)
				}
			}
			runtime.ReadMemStats(&after)
			allocated := after.TotalAlloc - before.TotalAlloc
			// A generous four input-sized allocations per call tolerates
			// runtime noise but rejects a slice entry per untrusted separator.
			budget := uint64(4 * len(input) * attempts)
			t.Logf("inputBytes=%d attempts=%d allocated=%d budget=%d", len(input), attempts, allocated, budget)
			if allocated > budget {
				t.Errorf("malformed JWT separator parsing exceeded bounded allocation: %d > %d", allocated, budget)
			}
		})
	}
}

func TestJWTAdvisoryNormalClaimsControls(t *testing.T) {
	for _, fixture := range []struct {
		name, sign, verify string
		method             jwt.SigningMethod
		wantValid          bool
	}{
		{"custom-HS256", "test-custom-secret", "test-custom-secret", jwt.SigningMethodHS256, true},
		{"reject-HS384", "test-custom-secret", "test-custom-secret", jwt.SigningMethodHS384, false},
		{"legacy-reads-current-default", currentDefaultKey, legacyDefaultKey, jwt.SigningMethodHS256, true},
		{"current-reads-legacy-default", legacyDefaultKey, currentDefaultKey, jwt.SigningMethodHS256, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			claims := Claims{UserID: 41, AuthVersion: 2, RegisteredClaims: jwt.RegisteredClaims{
				ID: "test-only-session-identity", IssuedAt: jwt.NewNumericDate(time.Unix(1700000000, 0)),
			}}
			token, err := jwt.NewWithClaims(fixture.method, claims).SignedString([]byte(fixture.sign))
			if err != nil {
				t.Fatal(err)
			}
			actual, err := parseClaims(fixture.verify, token)
			if !fixture.wantValid {
				if err == nil {
					t.Fatal("unsupported signing method accepted")
				}
				return
			}
			if err != nil || actual.UserID != claims.UserID || actual.AuthVersion != claims.AuthVersion || actual.ID != claims.ID || actual.IssuedAt == nil || !actual.IssuedAt.Equal(claims.IssuedAt.Time) {
				t.Fatalf("normal claims changed: %v", err)
			}
		})
	}
}
