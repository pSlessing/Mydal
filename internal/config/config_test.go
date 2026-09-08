package config

import (
	"strings"
	"testing"
)

// setRequired sets the four env vars Load cannot start without, so a test can
// focus on the one edge it is checking.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("ADDR", ":8080")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("MINIO_ENDPOINT", "localhost:9000")
	t.Setenv("BUCKET_NAME", "mydal")
}

// MINIO_USE_SSL used to be true only for the literal string "true", so any
// other truthy spelling ("1", "TRUE") silently meant false, and garbage was
// never reported.
func TestMinioUseSSLIsAStrictBoolean(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		want    bool
		wantErr bool
	}{
		{"unset", "", false, false},
		{"true", "true", true, false},
		{"false", "false", false, false},
		{"uppercase TRUE", "TRUE", true, false},
		{"garbage", "yes", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setRequired(t)
			if tc.raw != "" {
				t.Setenv("MINIO_USE_SSL", tc.raw)
			}
			cfg, err := Load()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("MINIO_USE_SSL=%q: want an error, got none", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("MINIO_USE_SSL=%q: %v", tc.raw, err)
			}
			if cfg.MinioUseSSL != tc.want {
				t.Errorf("MINIO_USE_SSL=%q: MinioUseSSL = %v, want %v", tc.raw, cfg.MinioUseSSL, tc.want)
			}
		})
	}
}

// Outside production, Load used to force sslmode=disable even when the DSN
// already named one, silently discarding an operator's explicit choice.
func TestSSLModeDefaultsWithoutOverwriting(t *testing.T) {
	t.Run("absent sslmode gets disable outside production", func(t *testing.T) {
		setRequired(t)
		t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cfg.DatabaseURL, "sslmode=disable") {
			t.Errorf("DatabaseURL = %q, want sslmode=disable added", cfg.DatabaseURL)
		}
	})

	t.Run("explicit sslmode survives outside production", func(t *testing.T) {
		setRequired(t)
		t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db?sslmode=require")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cfg.DatabaseURL, "sslmode=require") {
			t.Errorf("DatabaseURL = %q, want the explicit sslmode=require kept", cfg.DatabaseURL)
		}
	})

	t.Run("production leaves the DSN alone", func(t *testing.T) {
		setRequired(t)
		t.Setenv("MODE", "production")
		t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(cfg.DatabaseURL, "sslmode") {
			t.Errorf("DatabaseURL = %q, want no sslmode added in production", cfg.DatabaseURL)
		}
	})
}

// The access key is a credential, paired with the secret key to authenticate
// every request to the bucket, and used to be logged in clear at debug.
func TestLogValueRedactsBothMinioCredentials(t *testing.T) {
	setRequired(t)
	t.Setenv("MINIO_ACCESS_KEY", "super-secret-access-key")
	t.Setenv("MINIO_SECRET_KEY", "super-secret-secret-key")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	rendered := cfg.LogValue().String()
	if strings.Contains(rendered, "super-secret-access-key") {
		t.Errorf("access key leaked into the logged config: %s", rendered)
	}
	if strings.Contains(rendered, "super-secret-secret-key") {
		t.Errorf("secret key leaked into the logged config: %s", rendered)
	}
}

// -healthcheck must not need DATABASE_URL, MINIO_ENDPOINT or BUCKET_NAME: none
// of them bear on whether the local server is answering.
func TestAddrNeedsOnlyItself(t *testing.T) {
	t.Setenv("ADDR", ":8080")
	addr, err := Addr()
	if err != nil {
		t.Fatalf("Addr with only ADDR set: %v", err)
	}
	if addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", addr)
	}
}

func TestAddrRequiresADDR(t *testing.T) {
	// Explicitly clear ADDR rather than relying on it being absent from the
	// process environment: some environments (e.g. CI) set ADDR globally,
	// which would make this test fail for reasons unrelated to Addr().
	t.Setenv("ADDR", "")
	if _, err := Addr(); err == nil {
		t.Fatal("Addr with no ADDR set: want an error")
	}
}
