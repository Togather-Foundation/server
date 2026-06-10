package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/stretchr/testify/require"
)

func TestAdminAuthCookieRejectsBearerHeader(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	h := AdminAuthCookie(manager, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer token")
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestAdminAuthCookieRequiresCookie(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	h := AdminAuthCookie(manager, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestAdminAuthCookieRejectsInvalidToken(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	h := AdminAuthCookie(manager, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: AdminAuthCookieName, Value: "invalid"})
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestAdminAuthCookieSetsClaims(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	token, err := manager.Generate("admin", "admin")
	require.NoError(t, err)

	h := AdminAuthCookie(manager, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := AdminClaims(r)
		require.NotNil(t, claims)
		require.Equal(t, "admin", claims.Subject)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: AdminAuthCookieName, Value: token})
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
}

func TestAdminAuthCookieRedirectsOnAuthFailure(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	h := AdminAuthCookie(manager, "/admin/login")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusFound, res.Code)
	require.Equal(t, "/admin/login?redirect=%2Fadmin%2Fdashboard", res.Header().Get("Location"))
}

func TestAdminAuthCookieNoRedirectWhenEmpty(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	h := AdminAuthCookie(manager, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusUnauthorized, res.Code)
	require.Empty(t, res.Header().Get("Location"))
}

func TestAdminAuthCookieRedirectsWithInvalidToken(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	h := AdminAuthCookie(manager, "/admin/login")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: AdminAuthCookieName, Value: "invalid"})
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusFound, res.Code)
	require.Equal(t, "/admin/login", res.Header().Get("Location"))
}

func TestAdminAuthCookieRedirectsWithBearerHeader(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	h := AdminAuthCookie(manager, "/admin/login")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusFound, res.Code)
	require.Equal(t, "/admin/login", res.Header().Get("Location"))
}

func TestJWTAuthRejectsMissingHeader(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	h := JWTAuth(manager, "test")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestJWTAuthRejectsInvalidFormat(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	h := JWTAuth(manager, "test")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Token abc")
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestJWTAuthRejectsNonAdminRole(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	token, err := manager.Generate("user", "agent")
	require.NoError(t, err)

	h := JWTAuth(manager, "test")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusForbidden, res.Code)
}

func TestJWTAuthAcceptsAdminRole(t *testing.T) {
	manager := auth.NewJWTManager("secret", time.Hour, "test")
	token, err := manager.Generate("admin", "admin")
	require.NoError(t, err)

	h := JWTAuth(manager, "test")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := AdminClaims(r)
		require.NotNil(t, claims)
		require.Equal(t, "admin", claims.Subject)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
}
