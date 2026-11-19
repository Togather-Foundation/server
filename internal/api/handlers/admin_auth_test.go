package handlers

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type adminLoginDB struct {
	t          *testing.T
	role, hash string
	updated    bool
}

func (db *adminLoginDB) QueryRow(context.Context, string, ...any) pgx.Row { return adminLoginRow{db} }
func (db *adminLoginDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	db.t.Fatal("unexpected Query")
	return nil, nil
}
func (db *adminLoginDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	db.updated = true
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

type adminLoginRow struct{ db *adminLoginDB }

func (row adminLoginRow) Scan(dest ...any) error {
	*(dest[0].(*pgtype.UUID)) = pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	*(dest[1].(*string)) = "user"
	*(dest[2].(*string)) = "user@example.org"
	*(dest[3].(*string)) = row.db.hash
	*(dest[4].(*string)) = row.db.role
	*(dest[5].(*bool)) = true
	return nil
}

func TestAdminLoginRequiresAdminRole(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	require.NoError(t, err)
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	for _, role := range []string{"admin", "viewer", "editor", "unknown", ""} {
		t.Run(role, func(t *testing.T) {
			db := &adminLoginDB{t: t, role: role, hash: string(hash)}
			h := NewAdminAuthHandler(postgres.New(db), manager, nil, "test", nil)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader(`{"username":"user","password":"password"}`))
			res := httptest.NewRecorder()
			h.Login(res, req)
			if role == "admin" {
				require.Equal(t, http.StatusOK, res.Code)
				require.True(t, db.updated)
				require.Len(t, res.Result().Cookies(), 1)
				claims, err := manager.Validate(res.Result().Cookies()[0].Value)
				require.NoError(t, err)
				require.Equal(t, "admin", claims.Role)
				return
			}
			require.Equal(t, http.StatusForbidden, res.Code)
			require.False(t, db.updated)
			require.Empty(t, res.Result().Cookies())
			require.Equal(t, "application/problem+json", res.Header().Get("Content-Type"))
			var body map[string]any
			require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
			require.Equal(t, float64(http.StatusForbidden), body["status"])
			require.NotContains(t, body, "token")
		})
	}
}

func TestAdminLoginChecksPasswordBeforeRole(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	require.NoError(t, err)
	db := &adminLoginDB{t: t, role: "viewer", hash: string(hash)}
	h := NewAdminAuthHandler(postgres.New(db), auth.NewJWTManager("secret", time.Hour, "test"), nil, "test", nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader(`{"username":"user","password":"wrong"}`))
	res := httptest.NewRecorder()
	h.Login(res, req)
	require.Equal(t, http.StatusUnauthorized, res.Code)
	require.False(t, db.updated)
	require.Empty(t, res.Result().Cookies())
}

func TestAdminLoginPageRequiresAdminRole(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	templates := template.Must(template.New("login.html").Parse("login page"))
	h := NewAdminAuthHandler(nil, manager, nil, "test", templates)
	for _, role := range []string{"admin", "viewer", "editor", "unknown", ""} {
		t.Run(role, func(t *testing.T) {
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.Claims{
				Role:             role,
				RegisteredClaims: jwt.RegisteredClaims{Subject: "user", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
			}).SignedString([]byte("secret"))
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
			req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
			res := httptest.NewRecorder()
			h.LoginPage(res, req)
			if role == "admin" {
				require.Equal(t, http.StatusFound, res.Code)
				require.Equal(t, "/admin/dashboard", res.Header().Get("Location"))
				return
			}
			require.Equal(t, http.StatusForbidden, res.Code)
			require.Empty(t, res.Header().Get("Location"))
			require.NotContains(t, res.Body.String(), "login page")
		})
	}
}
