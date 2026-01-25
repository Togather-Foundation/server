package auth

import (
	"errors"
	"testing"
	"time"
)

func TestJWTGenerateValidate(t *testing.T) {
	manager := NewJWTManager("secret", time.Hour, "issuer")
	jwtToken, err := manager.Generate("user-1", "admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	claims, err := manager.Validate(jwtToken)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if claims.Subject != "user-1" || claims.Role != "admin" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

func TestJWTGenerateInvalid(t *testing.T) {
	manager := NewJWTManager("secret", time.Hour, "issuer")
	if _, err := manager.Generate("", "admin"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token error, got %v", err)
	}
}

func TestJWTValidateMissing(t *testing.T) {
	manager := NewJWTManager("secret", time.Hour, "issuer")
	if _, err := manager.Validate(""); !errors.Is(err, ErrMissingToken) {
		t.Fatalf("expected missing token error, got %v", err)
	}
}

func TestTokenFromHeader(t *testing.T) {
	if _, err := TokenFromHeader("nope"); !errors.Is(err, ErrMissingToken) {
		t.Fatalf("expected missing token error, got %v", err)
	}
	if token, err := TokenFromHeader("Bearer token"); err != nil || token != "token" {
		t.Fatalf("expected token, got %s err %v", token, err)
	}
}
