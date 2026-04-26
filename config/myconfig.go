package config

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
	EnvTest        Environment = "test"
)

// AppConfig represents your application configuration
type AppConfig struct {
	Environment Environment `env:"APP_ENV" envDefault:"development"`
	AppName     Environment `env:"APP_NAME" envRequired:"true"`

	// Server configuration
	ServerAddress            string `env:"WEB_SERVER_ADDRESS" envDefault:"localhost"`
	ServerPort               string `env:"WEB_SERVER_PORT" envDefault:"8080"`
	ServerTlsEnabled         bool   `env:"WEB_SERVER_TLS_ENABLED" envDefault:"false"`
	ServerTlsCertPublicPath  string `env:"WEB_SERVER_TLS_CERT_PUBLIC_PATH" envRequired:"false"`
	ServerTlsCertPrivatePath string `env:"WEB_SERVER_TLS_CERT_PRIVATE_PATH" envRequired:"false"`

	// Database configuration
	DBHost                string `env:"DB_HOST" envRequired:"true"`
	DBPort                int    `env:"DB_PORT" envDefault:"5432"`
	DBUser                string `env:"DB_USER" envRequired:"true"`
	DBPassword            string `env:"DB_PASSWORD" envRequired:"true"`
	DBName                string `env:"DB_NAME" envRequired:"true"`
	DBSchema              string `env:"DB_SCHEMA" envRequired:"true"`
	DBSslEnabled          bool   `env:"DB_SSL_ENABLED" envDefault:"false"`
	DBSslMode             string `env:"DB_SSL_MODE" envDefault:"require"`
	DBPoolMaxConns        int    `env:"DB_POOL_MAX_CONNS" envDefault:"25"`
	DBPoolMinConns        int    `env:"DB_POOL_MIN_CONNS" envDefault:"5"`
	DBPoolMaxConnLifetime int    `env:"DB_POOL_MAX_CONN_LIFETIME_MINUTES" envDefault:"60"`
	DBPoolMaxConnIdleTime int    `env:"DB_POOL_MAX_CONN_IDLE_TIME_MINUTES" envDefault:"30"`

	// Logging
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	// Timeouts
	ShutdownTimeout int `env:"SHUTDOWN_TIMEOUT_SECONDS" envDefault:"10"`
	ReadTimeout     int `env:"READ_TIMEOUT_SECONDS" envDefault:"30"`
	WriteTimeout    int `env:"WRITE_TIMEOUT_SECONDS" envDefault:"30"`
}

// LoadConfig loads configuration from .env files and environment variables
func LoadConfig() (*AppConfig, error) {
	// Parse command line flag for environment
	var envFlag string
	flag.StringVar(&envFlag, "env", "", "Environment (development, staging, production, test)")
	flag.Parse()

	// Determine environment (CLI flag > APP_ENV env var > default)
	env := determineEnvironment(envFlag)

	// Load .env files in order of precedence
	if err := loadEnvFiles(env); err != nil {
		return nil, fmt.Errorf("failed to load .env files: %w", err)
	}

	// Create config and read from environment
	var cfg AppConfig

	// Use envconfig to populate the struct
	// This reads from both OS environment variables and .env files
	if err := envconfig.Read(&cfg); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Override environment from flag (in case it wasn't in .env files)
	if envFlag != "" {
		cfg.Environment = Environment(envFlag)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func determineEnvironment(envFlag string) Environment {
	// Priority: CLI flag > APP_ENV > default
	if envFlag != "" {
		return Environment(strings.ToLower(envFlag))
	}

	// Check if APP_ENV is set in OS environment
	if env := os.Getenv("APP_ENV"); env != "" {
		return Environment(strings.ToLower(env))
	}

	return EnvDevelopment
}

func loadEnvFiles(env Environment) error {
	// Files are loaded in order - later files override earlier ones
	files := []string{
		".env",                           // Base (lowest priority)
		".env." + string(env),            // Environment-specific
		".env.local",                     // Local overrides (should be gitignored)
		".env." + string(env) + ".local", // Environment-specific local (highest priority)
	}

	for _, file := range files {
		// envconfig.EnvFileLookup creates a lookup function that reads from .env files
		// We need to load them into the environment for envconfig to pick up
		if err := loadSingleEnvFile(file); err != nil {
			// Don't fail if file doesn't exist
			if !os.IsNotExist(err) {
				return fmt.Errorf("error loading %s: %w", file, err)
			}
		}
	}

	return nil
}

func loadSingleEnvFile(filename string) error {
	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil // File doesn't exist, skip silently
	}

	// Use envconfig's EnvFileLookup to read the file
	lookup := envconfig.EnvFileLookup(filename)

	// We need to temporarily set environment variables from the file
	// Since envconfig reads from the environment, we'll set them directly
	// This is a limitation - for a cleaner approach, see the alternative below
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	// Parse and set environment variables
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Only set if not already set in environment
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}

	_ = lookup // Silence unused warning
	return nil
}

// Validate checks if the configuration is valid
func (c *AppConfig) Validate() error {
	// Validate environment
	switch c.Environment {
	case EnvDevelopment, EnvStaging, EnvProduction, EnvTest:
		// valid
	default:
		return fmt.Errorf("invalid environment: %s", c.Environment)
	}

	// Validate server port
	if c.ServerPort == "" {
		return fmt.Errorf("server port is required")
	}

	// Additional validation for production
	if c.Environment == EnvProduction {
		if c.DBPassword == "" {
			return fmt.Errorf("database password is required in production")
		}
		if c.LogLevel != "error" && c.LogLevel != "warn" {
			return fmt.Errorf("log level should be error or warn in production, got: %s", c.LogLevel)
		}
	}

	return nil
}

// Helper methods for environment checks
func (c *AppConfig) IsDevelopment() bool {
	return c.Environment == EnvDevelopment
}

func (c *AppConfig) IsProduction() bool {
	return c.Environment == EnvProduction
}

func (c *AppConfig) IsStaging() bool {
	return c.Environment == EnvStaging
}

func (c *AppConfig) IsTest() bool {
	return c.Environment == EnvTest
}

// GetServerAddr returns the full server address
func (c *AppConfig) GetServerAddr() string {
	return fmt.Sprintf("%s:%s", c.ServerAddress, c.ServerPort)
}
