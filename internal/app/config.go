package app

import (
	"fmt"
	"strings"

	"github.com/d9042n/telekube/internal/config"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// LoadConfig loads configuration from file and environment variables.
func LoadConfig() (*config.Settings, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("telegram.rate_limit", 30)
	v.SetDefault("storage.backend", "sqlite")
	v.SetDefault("storage.sqlite.path", "telekube.db")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout_ms", 15000)
	v.SetDefault("server.write_timeout_ms", 15000)
	v.SetDefault("server.idle_timeout_ms", 60000)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("rbac.default_role", "viewer")
	v.SetDefault("modules.kubernetes.enabled", true)

	// Config file
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		// Config file not found is OK — rely on env vars
	}

	// Environment variables
	v.SetEnvPrefix("TELEKUBE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg config.Settings
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// Validate
	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}
