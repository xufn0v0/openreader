package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

// The published fallbacks of OPENREADER_JWT_SECRET: config.Load uses the
// first, and a server configured with either one also verifies the other.
const (
	publishedCurrentJWTSecret  = "change-this-before-deploy"
	publishedPreviousJWTSecret = "change-me-in-production"
	legacySessionCustomSecret  = "legacy-session-contract-custom-secret"
	legacySessionMarkerKey     = "authenticated-session-v1"
)

func setupLegacySessionServer(t *testing.T, secret string) (*gin.Engine, *Server, *httptest.Server) {
	t.Helper()
	router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.JWTSecret = secret
	})
	httpServer := httptest.NewServer(router)
	t.Cleanup(httpServer.Close)
	return router, server, httpServer
}

// signLegacySessionToken signs the claims an older binary issued: userId and
// iat only, with neither jti nor authVersion.
func signLegacySessionToken(t *testing.T, secret string, userID uint, issuedAt time.Time) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId": userID,
		"iat":    issuedAt.Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func legacySessionAccountCreatedAt(t *testing.T, server *Server, userID uint) time.Time {
	t.Helper()
	var user models.User
	if err := server.db.Select("id", "created_at").First(&user, userID).Error; err != nil {
		t.Fatal(err)
	}
	return user.CreatedAt
}

// rerunSessionMigrationAfterAccounts reproduces an upgrade: the accounts exist
// before the session migration writes its marker.
func rerunSessionMigrationAfterAccounts(t *testing.T, server *Server, userID uint) time.Time {
	t.Helper()
	if err := server.db.Where("key = ?", legacySessionMarkerKey).Delete(&models.SchemaMigration{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(server.db); err != nil {
		t.Fatal(err)
	}
	var marker models.SchemaMigration
	if err := server.db.Where("key = ?", legacySessionMarkerKey).First(&marker).Error; err != nil {
		t.Fatal(err)
	}
	if createdAt := legacySessionAccountCreatedAt(t, server, userID); !createdAt.Before(marker.AppliedAt) {
		t.Fatalf("fixture: account created at %s does not predate marker %s", createdAt, marker.AppliedAt)
	}
	return marker.AppliedAt
}

func countLegacySessionRows(t *testing.T, server *Server) int64 {
	t.Helper()
	var count int64
	if err := server.db.Model(&models.UserSession{}).Where("legacy = ?", true).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func requireSessionEntriesRejected(t *testing.T, router *gin.Engine, httpServer *httptest.Server, label, token string) {
	t.Helper()
	me := sessionContractJSONRequest(router, http.MethodGet, "/api/me", "", token)
	if me.Code != http.StatusUnauthorized {
		t.Errorf("%s: GET /api/me status=%d, want 401: %s", label, me.Code, me.Body.String())
	}
	for _, prefix := range []string{"/webdav/", "/reader3/webdav/"} {
		response := webDAVProtocolRequest(t, router, "PROPFIND", prefix, "Bearer "+token, "", map[string]string{"Depth": "0"})
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s: PROPFIND %s with Bearer status=%d, want 401", label, prefix, response.Code)
		}
	}
	connection, response, err := dialSyncWebSocket(httpServer.URL, token, "")
	if connection != nil {
		_ = connection.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Errorf("%s: /ws/sync handshake status=%v err=%v, want 401", label, responseStatus(response), err)
	}
}

func requireSessionEntriesAccepted(t *testing.T, router *gin.Engine, httpServer *httptest.Server, label, token string, userID uint) {
	t.Helper()
	me := sessionContractJSONRequest(router, http.MethodGet, "/api/me", "", token)
	var user struct {
		ID uint `json:"id"`
	}
	if me.Code != http.StatusOK || json.Unmarshal(me.Body.Bytes(), &user) != nil || user.ID != userID {
		t.Errorf("%s: GET /api/me status=%d user=%d, want 200 for user %d: %s", label, me.Code, user.ID, userID, me.Body.String())
	}
	for _, prefix := range []string{"/webdav/", "/reader3/webdav/"} {
		response := webDAVProtocolRequest(t, router, "PROPFIND", prefix, "Bearer "+token, "", map[string]string{"Depth": "0"})
		if response.Code != http.StatusMultiStatus {
			t.Errorf("%s: PROPFIND %s with Bearer status=%d, want 207: %s", label, prefix, response.Code, response.Body.String())
		}
	}
	connection, response, err := dialSyncWebSocket(httpServer.URL, token, "")
	if connection != nil {
		_ = connection.Close()
	}
	if err != nil {
		t.Errorf("%s: /ws/sync handshake status=%v err=%v, want success", label, responseStatus(response), err)
	}
}

func TestForgedLegacyTokenCannotTakeOverFreshInstallWithPublishedSecret(t *testing.T) {
	for _, signer := range []struct{ name, secret string }{
		{"current-default", publishedCurrentJWTSecret},
		{"previous-default", publishedPreviousJWTSecret},
	} {
		t.Run(signer.name, func(t *testing.T) {
			router, server, httpServer := setupLegacySessionServer(t, publishedCurrentJWTSecret)
			admin := registerSessionContractUser(t, router, "freshadmin", "password8")
			var role struct {
				Role string `json:"role"`
			}
			if me := sessionContractJSONRequest(router, http.MethodGet, "/api/me", "", admin.Token); me.Code != http.StatusOK ||
				json.Unmarshal(me.Body.Bytes(), &role) != nil || role.Role != "admin" {
				t.Fatalf("fixture: first account is not the administrator: %d %s", me.Code, me.Body.String())
			}

			forged := signLegacySessionToken(t, signer.secret, admin.User.ID, time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC))
			requireSessionEntriesRejected(t, router, httpServer, "forged legacy token", forged)
			if count := countLegacySessionRows(t, server); count != 0 {
				t.Errorf("forged legacy token created %d legacy sessions, want 0", count)
			}
		})
	}
}

func TestLegacyTokenUnderPublishedSecretIsNotAdoptedAfterUpgrade(t *testing.T) {
	for _, signer := range []struct{ name, secret string }{
		{"current-default", publishedCurrentJWTSecret},
		{"previous-default", publishedPreviousJWTSecret},
	} {
		t.Run(signer.name, func(t *testing.T) {
			router, server, httpServer := setupLegacySessionServer(t, publishedCurrentJWTSecret)
			account := registerSessionContractUser(t, router, "upgradedpublic", "password8")
			createdAt := legacySessionAccountCreatedAt(t, server, account.User.ID)
			rerunSessionMigrationAfterAccounts(t, server, account.User.ID)

			legacy := signLegacySessionToken(t, signer.secret, account.User.ID, createdAt)
			requireSessionEntriesRejected(t, router, httpServer, "public-secret legacy token", legacy)
			if count := countLegacySessionRows(t, server); count != 0 {
				t.Errorf("public-secret legacy token created %d legacy sessions, want 0", count)
			}
		})
	}
}

func TestLegacyTokenIsNotAdoptedForAccountCreatedAfterMarker(t *testing.T) {
	router, server, httpServer := setupLegacySessionServer(t, legacySessionCustomSecret)
	account := registerSessionContractUser(t, router, "freshcustom", "password8")

	legacy := signLegacySessionToken(t, legacySessionCustomSecret, account.User.ID, time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC))
	requireSessionEntriesRejected(t, router, httpServer, "legacy token for a post-marker account", legacy)
	if count := countLegacySessionRows(t, server); count != 0 {
		t.Errorf("legacy token for a post-marker account created %d legacy sessions, want 0", count)
	}
}

func TestModernSessionTokensRemainValidUnderPublishedAndCustomSecrets(t *testing.T) {
	for _, configured := range []struct{ name, secret string }{
		{"published-default", publishedCurrentJWTSecret},
		{"custom", legacySessionCustomSecret},
	} {
		t.Run(configured.name, func(t *testing.T) {
			router, server, httpServer := setupLegacySessionServer(t, configured.secret)
			account := registerSessionContractUser(t, router, "modernsession", "password8")
			requireSessionEntriesAccepted(t, router, httpServer, "registered session", account.Token, account.User.ID)
			login := loginSessionContractUser(t, router, "modernsession", "password8")
			requireSessionEntriesAccepted(t, router, httpServer, "login session", login.Token, account.User.ID)
			if count := countLegacySessionRows(t, server); count != 0 {
				t.Errorf("modern sessions created %d legacy sessions, want 0", count)
			}
		})
	}
}

func TestForgedModernTokenWithUnknownIdentityIsRejectedUnderPublishedSecret(t *testing.T) {
	router, server, httpServer := setupLegacySessionServer(t, publishedCurrentJWTSecret)
	admin := registerSessionContractUser(t, router, "modernforgery", "password8")
	identity := make([]byte, 32)
	if _, err := rand.Read(identity); err != nil {
		t.Fatal(err)
	}
	forged, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId":      admin.User.ID,
		"authVersion": 1,
		"jti":         base64.RawURLEncoding.EncodeToString(identity),
		"iat":         time.Now().Unix(),
	}).SignedString([]byte(publishedCurrentJWTSecret))
	if err != nil {
		t.Fatal(err)
	}

	requireSessionEntriesRejected(t, router, httpServer, "forged modern token", forged)
	var sessions int64
	if err := server.db.Model(&models.UserSession{}).Where("user_id = ?", admin.User.ID).Count(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	if sessions != 1 {
		t.Errorf("forged modern token changed session rows to %d, want only the registration session", sessions)
	}
}

func TestUpgradedCustomSecretInstanceStillAdoptsLegitimateLegacyToken(t *testing.T) {
	router, server, httpServer := setupLegacySessionServer(t, legacySessionCustomSecret)
	account := registerSessionContractUser(t, router, "upgradedcustom", "password8")
	createdAt := legacySessionAccountCreatedAt(t, server, account.User.ID)
	rerunSessionMigrationAfterAccounts(t, server, account.User.ID)

	// Issued by the older binary while the account already existed.
	legacy := signLegacySessionToken(t, legacySessionCustomSecret, account.User.ID, createdAt)
	requireSessionEntriesAccepted(t, router, httpServer, "upgraded legacy token", legacy, account.User.ID)
	if count := countLegacySessionRows(t, server); count != 1 {
		t.Fatalf("upgraded legacy token adopted into %d legacy sessions, want 1", count)
	}

	logout := sessionContractJSONRequest(router, http.MethodPost, "/api/auth/logout", "", legacy)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout adopted legacy token status=%d, want 204: %s", logout.Code, logout.Body.String())
	}
	requireSessionEntriesRejected(t, router, httpServer, "logged-out legacy token", legacy)
}
