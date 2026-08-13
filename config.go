package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all settings needed to run the absence bot, loaded from a
// local YAML file (see config.example.yaml).
type Config struct {
	Homeserver  string `yaml:"homeserver"`
	UserID      string `yaml:"user_id"`
	AccessToken string `yaml:"access_token"`
	DeviceID    string `yaml:"device_id"`

	// PickleKey encrypts the local crypto store at rest. Keep it secret and
	// stable across restarts - changing it makes the existing crypto.db
	// unreadable.
	PickleKey string `yaml:"pickle_key"`

	// RecoveryKey is the account's SSSS recovery key. Optional, but without
	// it the bot's device may never be trusted by your other devices, which
	// means it may never receive the keys needed to decrypt messages in
	// encrypted rooms.
	RecoveryKey string `yaml:"recovery_key"`

	CryptoDBPath string `yaml:"crypto_db_path"`
	StatePath    string `yaml:"state_path"`

	ReplyMessage string `yaml:"reply_message"`

	// Presence is sent as set_presence on every /sync request, so it's what
	// makes you show up as "away" to others while the bot runs - without it,
	// the mere act of the bot polling /sync marks you "online" on every
	// request, regardless of what the auto-reply says. One of "unavailable"
	// (shown as "Away" in most clients - the default), "offline", or
	// "online" (to leave your real presence alone and only use the
	// auto-reply).
	Presence string `yaml:"presence"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	cfg := &Config{
		CryptoDBPath: "crypto.db",
		StatePath:    "state.json",
		ReplyMessage: "I'm currently away and will get back to you as soon as I can.",
		Presence:     "unavailable",
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	missing := func(name, val string) string {
		if val == "" {
			return name + " "
		}
		return ""
	}
	problems := missing("homeserver", c.Homeserver) +
		missing("user_id", c.UserID) +
		missing("access_token", c.AccessToken) +
		missing("device_id", c.DeviceID) +
		missing("pickle_key", c.PickleKey) +
		missing("reply_message", c.ReplyMessage)
	if problems != "" {
		return fmt.Errorf("config is missing required field(s): %s", problems)
	}
	switch c.Presence {
	case "online", "offline", "unavailable":
	default:
		return fmt.Errorf("presence must be one of \"online\", \"offline\", \"unavailable\", got %q", c.Presence)
	}
	return nil
}
