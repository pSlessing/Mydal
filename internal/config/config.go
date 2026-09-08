package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// defaultMaxUploadBytes is a generous ceiling for a single audio file - a long
// lossless album side fits well inside it - while still bounding what one
// request can write.
const defaultMaxUploadBytes int64 = 1 << 30 // 1 GiB

type Config struct {
	Addr           string
	DatabaseURL    string
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioUseSSL    bool
	BucketName     string
	LogLevel       string
	Mode           string
	MaxUploadBytes int64
}

// LogValue redacts the credentials so logging the whole config stays safe.
// The access key is a credential too - paired with the secret key, it is
// what authenticates every request to the bucket - so it is redacted the
// same as the secret, not logged in clear at debug.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("Addr", c.Addr),
		slog.String("DatabaseURL", redactURL(c.DatabaseURL)),
		slog.String("MinioEndpoint", c.MinioEndpoint),
		slog.String("MinioAccessKey", "[REDACTED]"),
		slog.String("MinioSecretKey", "[REDACTED]"),
		slog.Bool("MinioUseSSL", c.MinioUseSSL),
		slog.String("BucketName", c.BucketName),
		slog.String("LogLevel", c.LogLevel),
		slog.String("Mode", c.Mode),
		slog.Int64("MaxUploadBytes", c.MaxUploadBytes),
	)
}

// redactURL strips the password from a DSN, leaving the rest readable.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[UNPARSEABLE]"
	}
	if u.User != nil {
		if _, ok := u.User.Password(); ok {
			u.User = url.UserPassword(u.User.Username(), "xxxxx")
		}
	}
	return u.String()
}

// Addr reads just ADDR. It exists for -healthcheck, which only needs to know
// where the local server is listening: routing that through Load would also
// demand DATABASE_URL, MINIO_ENDPOINT and BUCKET_NAME, none of which bear on
// whether the process is answering.
func Addr() (string, error) {
	addr := os.Getenv("ADDR")
	if addr == "" {
		return "", fmt.Errorf("missing required configuration: ADDR")
	}
	return addr, nil
}

// parseBoolEnv parses a boolean env var. An unset variable is false; any
// value that is not a valid boolean is an error rather than a silent false,
// so a typo (e.g. "1" meant as "true") is caught at startup.
func parseBoolEnv(key string) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean, got %q", key, raw)
	}
	return v, nil
}

// Load reads the configuration from the environment, normalises it and
// validates it, so the server fails at startup rather than on first use.
func Load() (Config, error) {
	minioUseSSL, err := parseBoolEnv("MINIO_USE_SSL")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Addr:           os.Getenv("ADDR"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		MinioEndpoint:  os.Getenv("MINIO_ENDPOINT"),
		MinioAccessKey: os.Getenv("MINIO_ACCESS_KEY"),
		MinioSecretKey: os.Getenv("MINIO_SECRET_KEY"),
		MinioUseSSL:    minioUseSSL,
		BucketName:     os.Getenv("BUCKET_NAME"),
		LogLevel:       os.Getenv("LOG_LEVEL"),
		Mode:           os.Getenv("MODE"),
	}

	cfg.MaxUploadBytes = defaultMaxUploadBytes
	if raw := os.Getenv("MAX_UPLOAD_BYTES"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("MAX_UPLOAD_BYTES must be a positive integer, got %q", raw)
		}
		cfg.MaxUploadBytes = parsed
	}

	var missing []string
	for _, required := range []struct {
		key   string
		value string
	}{
		{"ADDR", cfg.Addr},
		{"DATABASE_URL", cfg.DatabaseURL},
		{"MINIO_ENDPOINT", cfg.MinioEndpoint},
		{"BUCKET_NAME", cfg.BucketName},
	} {
		if required.value == "" {
			missing = append(missing, required.key)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}

	// Outside production, default sslmode to disable so a local compose stack
	// works without certificates - but only when the DSN does not already say,
	// so an operator who set sslmode explicitly is not silently overridden.
	if cfg.Mode != "production" {
		u, err := url.Parse(cfg.DatabaseURL)
		if err != nil {
			return Config{}, fmt.Errorf("parse DATABASE_URL: %w", err)
		}
		q := u.Query()
		if q.Get("sslmode") == "" {
			q.Set("sslmode", "disable")
			u.RawQuery = q.Encode()
			cfg.DatabaseURL = u.String()
		}
	}

	return cfg, nil
}
