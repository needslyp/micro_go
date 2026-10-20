package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestIssueToken(t *testing.T) {
	secret := "test-secret"
	response, err := issueToken(42, secret, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if response.TokenType != "Bearer" || response.ExpiresIn != 1800 {
		t.Fatalf("unexpected token metadata: %+v", response)
	}
	claims := &jwt.RegisteredClaims{}
	_, err = jwt.ParseWithClaims(response.AccessToken, claims, func(*jwt.Token) (any, error) { return []byte(secret), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("auth-service"), jwt.WithAudience("blog-service"), jwt.WithExpirationRequired())
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "42" || claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.ExpiresAt.Sub(claims.IssuedAt.Time) != 30*time.Minute {
		t.Fatal("unexpected token lifetime")
	}
	if _, err := jwt.Parse(response.AccessToken, func(*jwt.Token) (any, error) { return []byte("wrong-secret"), nil }, jwt.WithValidMethods([]string{"HS256"})); err == nil {
		t.Fatal("accepted wrong signing key")
	}
	if _, err := jwt.Parse(response.AccessToken, func(*jwt.Token) (any, error) { return []byte(secret), nil }, jwt.WithTimeFunc(func() time.Time { return time.Now().Add(30*time.Minute + time.Second) })); err == nil {
		t.Fatal("accepted expired token")
	}
}

func TestIssueTokenRequiresSecret(t *testing.T) {
	if _, err := issueToken(42, " ", defaultAccessTokenTTL); err == nil {
		t.Fatal("accepted empty secret")
	}
}

func TestValidateToken(t *testing.T) {
	const secret = "test-secret"
	service := &Service{jwtSecret: secret}
	response, err := issueToken(42, secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := service.ValidateToken(response.AccessToken); err != nil || id != 42 {
		t.Fatalf("got ID %d, error %v", id, err)
	}
	for _, tt := range []struct {
		name   string
		change func(jwt.MapClaims)
		method jwt.SigningMethod
		key    any
	}{
		{name: "wrong signature", key: []byte("other-secret")},
		{name: "wrong algorithm", method: jwt.SigningMethodHS384},
		{name: "unsigned", method: jwt.SigningMethodNone, key: jwt.UnsafeAllowNoneSignatureType},
		{name: "expired", change: func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Minute).Unix() }},
		{name: "missing expiration", change: func(c jwt.MapClaims) { delete(c, "exp") }},
		{name: "wrong issuer", change: func(c jwt.MapClaims) { c["iss"] = "other" }},
		{name: "missing issuer", change: func(c jwt.MapClaims) { delete(c, "iss") }},
		{name: "wrong audience", change: func(c jwt.MapClaims) { c["aud"] = "other" }},
		{name: "missing audience", change: func(c jwt.MapClaims) { delete(c, "aud") }},
		{name: "future issued at", change: func(c jwt.MapClaims) { c["iat"] = time.Now().Add(time.Hour).Unix() }},
		{name: "not yet valid", change: func(c jwt.MapClaims) { c["nbf"] = time.Now().Add(time.Hour).Unix() }},
		{name: "missing subject", change: func(c jwt.MapClaims) { delete(c, "sub") }},
		{name: "zero subject", change: func(c jwt.MapClaims) { c["sub"] = "0" }},
		{name: "negative subject", change: func(c jwt.MapClaims) { c["sub"] = "-1" }},
		{name: "non numeric subject", change: func(c jwt.MapClaims) { c["sub"] = "user" }},
		{name: "overflow subject", change: func(c jwt.MapClaims) { c["sub"] = "9223372036854775808" }},
		{name: "non canonical subject", change: func(c jwt.MapClaims) { c["sub"] = "+42" }},
		{name: "wrong subject type", change: func(c jwt.MapClaims) { c["sub"] = 42 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			claims := jwt.MapClaims{"sub": "42", "iss": "auth-service", "aud": "blog-service", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix()}
			if tt.change != nil {
				tt.change(claims)
			}
			method := tt.method
			if method == nil {
				method = jwt.SigningMethodHS256
			}
			key := tt.key
			if key == nil {
				key = []byte(secret)
			}
			raw, err := jwt.NewWithClaims(method, claims).SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			if id, err := service.ValidateToken(raw); !errors.Is(err, ErrInvalidToken) || id != 0 {
				t.Fatalf("got ID %d, error %v", id, err)
			}
		})
	}
	for _, raw := range []string{"", "broken", response.AccessToken + "corrupted"} {
		if _, err := service.ValidateToken(raw); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("invalid token accepted: %v", err)
		}
	}
	if _, err := (&Service{}).ValidateToken(response.AccessToken); err == nil {
		t.Fatal("accepted missing secret")
	}
}

func TestIssueTokenRejectsInvalidTTL(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Second, time.Millisecond, time.Second + time.Nanosecond} {
		if _, err := issueToken(42, "secret", ttl); err == nil {
			t.Fatalf("accepted TTL %s", ttl)
		}
	}
}
