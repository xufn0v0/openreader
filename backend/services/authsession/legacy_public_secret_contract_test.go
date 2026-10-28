package authsession

import (
	"context"
	"errors"
	"testing"
	"time"

	"openreader/backend/models"
)

// Legacy-format tokens carry neither jti nor authVersion, so a valid signature
// is the only evidence that this instance issued them. A signature made with a
// published key is no evidence at all, and an account created after the
// session migration marker never held a token from an older binary.

func countLegacySessions(t *testing.T, fixture serviceFixture) int64 {
	t.Helper()
	var count int64
	if err := fixture.db.Model(&models.UserSession{}).Where("legacy = ?", true).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestLegacyTokenUnderPublicSecretIsRejectedBeforeAccountLookup(t *testing.T) {
	for _, configured := range []struct {
		name    string
		secret  string
		signers []string
	}{
		{"current-default", currentDefaultKey, []string{currentDefaultKey, legacyDefaultKey}},
		{"legacy-default", legacyDefaultKey, []string{legacyDefaultKey, currentDefaultKey}},
		{"empty", "", []string{""}},
		{"whitespace", " \t ", []string{" \t "}},
	} {
		for index, signer := range configured.signers {
			name := configured.name + "/signer-" + []string{"same", "other-default"}[index]
			t.Run(name, func(t *testing.T) {
				fixture := newServiceFixture(t)
				ctx := context.Background()
				service := New(fixture.db, configured.secret)
				service.now = fixture.service.now
				token := signLegacyToken(t, signer, fixture.user.ID, fixture.marker.Add(-time.Hour))

				identity, err := service.Authenticate(ctx, token)
				if !errors.Is(err, ErrInvalidSession) {
					t.Errorf("legacy token signed with a public secret: identity=%+v err=%v, want ErrInvalidSession", identity, err)
				}
				if count := countLegacySessions(t, fixture); count != 0 {
					t.Errorf("legacy token signed with a public secret created %d legacy sessions, want 0", count)
				}

				sqlDB, err := fixture.db.DB()
				if err != nil {
					t.Fatal(err)
				}
				if err := sqlDB.Close(); err != nil {
					t.Fatal(err)
				}
				if _, err := service.Authenticate(ctx, token); !errors.Is(err, ErrInvalidSession) {
					t.Errorf("legacy token under a public secret reached the database: err=%v, want ErrInvalidSession", err)
				}
			})
		}
	}
}

func TestLegacyTokenRequiresAccountCreatedBeforeMarker(t *testing.T) {
	for _, account := range []struct {
		name      string
		createdAt func(marker time.Time) any
		adopted   bool
	}{
		{"created-after-marker", func(marker time.Time) any { return marker.Add(time.Minute) }, false},
		{"created-at-marker", func(marker time.Time) any { return marker }, false},
		{"zero-creation-time", func(time.Time) any { return time.Time{} }, false},
		{"null-creation-time", func(time.Time) any { return nil }, false},
		{"created-before-marker", func(marker time.Time) any { return marker.Add(-time.Minute) }, true},
	} {
		t.Run(account.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			ctx := context.Background()
			if err := fixture.db.Model(&models.User{}).Where("id = ?", fixture.user.ID).
				UpdateColumn("created_at", account.createdAt(fixture.marker)).Error; err != nil {
				t.Fatal(err)
			}
			token := signLegacyToken(t, fixture.service.secret, fixture.user.ID, fixture.marker.Add(-time.Hour))

			identity, err := fixture.service.Authenticate(ctx, token)
			if account.adopted {
				if err != nil || identity.UserID != fixture.user.ID || !identity.Legacy {
					t.Fatalf("pre-marker account legacy token: identity=%+v err=%v", identity, err)
				}
				if count := countLegacySessions(t, fixture); count != 1 {
					t.Fatalf("pre-marker account legacy sessions = %d, want 1", count)
				}
				return
			}
			if !errors.Is(err, ErrInvalidSession) {
				t.Errorf("legacy token for an account not created before the marker: identity=%+v err=%v, want ErrInvalidSession", identity, err)
			}
			if count := countLegacySessions(t, fixture); count != 0 {
				t.Errorf("legacy token for an account not created before the marker created %d legacy sessions, want 0", count)
			}
		})
	}
}

func TestModernSessionsUnaffectedByPublicSecretLegacyRule(t *testing.T) {
	for _, pair := range []struct {
		name, issue, verify string
	}{
		{"current-default", currentDefaultKey, currentDefaultKey},
		{"current-reads-legacy-default", legacyDefaultKey, currentDefaultKey},
		{"legacy-reads-current-default", currentDefaultKey, legacyDefaultKey},
	} {
		t.Run(pair.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			ctx := context.Background()
			issuer := New(fixture.db, pair.issue)
			issuer.now = fixture.service.now
			issuer.random = &sequenceReader{}
			verifier := New(fixture.db, pair.verify)
			verifier.now = fixture.service.now

			token, err := issuer.Issue(ctx, fixture.user)
			if err != nil {
				t.Fatal(err)
			}
			identity, err := verifier.Authenticate(ctx, token)
			if err != nil || identity.UserID != fixture.user.ID || identity.Legacy || identity.Key == "" {
				t.Fatalf("modern session under a public default secret: identity=%+v err=%v", identity, err)
			}
		})
	}
}
