package auth

import (
	"testing"
	"time"
)

func TestLoadConfigJWTLifetime(t *testing.T) {
	t.Setenv("AUTH_DATABASE_URL", "postgres://unused")
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	for _, tt := range []struct {
		value   string
		want    time.Duration
		invalid bool
	}{
		{value: "", want: 15 * time.Minute},
		{value: "30m", want: 30 * time.Minute},
		{value: " 1h ", want: time.Hour},
		{value: "1s", want: time.Second},
		{value: "0s", invalid: true},
		{value: "-1m", invalid: true},
		{value: "500ms", invalid: true},
		{value: "1.5s", invalid: true},
		{value: "invalid", invalid: true},
		{value: "999999999999999999h", invalid: true},
	} {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("AUTH_JWT_TTL", tt.value)
			cfg, err := loadConfig()
			if tt.invalid {
				if err == nil {
					t.Fatal("invalid TTL accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.JWTTokenTTL != tt.want {
				t.Fatalf("got %s, want %s", cfg.JWTTokenTTL, tt.want)
			}
		})
	}
}

func TestLoadConfigGRPCAddress(t *testing.T) {
	t.Setenv("AUTH_DATABASE_URL", "postgres://unused")
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	t.Setenv("AUTH_JWT_TTL", "15m")
	for _, tt := range []struct{ value, want string }{
		{"", ":9091"}, {"   ", ":9091"}, {"127.0.0.1:19091", "127.0.0.1:19091"},
	} {
		t.Setenv("AUTH_GRPC_ADDR", tt.value)
		cfg, err := loadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.GRPCAddr != tt.want {
			t.Fatalf("got %q, want %q", cfg.GRPCAddr, tt.want)
		}
	}
}
