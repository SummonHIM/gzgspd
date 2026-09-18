package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateRejects(t *testing.T) {
	base := func() *Config {
		return &Config{Instance: []ConfigInstance{{
			Username: "u", Password: "p", KeepAlive: 5, RetryTime: 5,
		}}}
	}

	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"no instance", func(c *Config) { c.Instance = nil }},
		{"empty username", func(c *Config) { c.Instance[0].Username = "" }},
		{"empty password", func(c *Config) { c.Instance[0].Password = "" }},
		{"keep_alive zero", func(c *Config) { c.Instance[0].KeepAlive = 0 }},
		{"retry_max negative", func(c *Config) { c.Instance[0].RetryMax = -1 }},
		{"retry_time zero", func(c *Config) { c.Instance[0].RetryTime = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mut(c)
			if err := c.Validate(); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestValidateAccepts(t *testing.T) {
	c := &Config{Instance: []ConfigInstance{{Username: "u", Password: "p", KeepAlive: 5, RetryTime: 5}}}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadIgnoresUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"log_level":0,"instance":[{"username":"u","password":"p","keep_alive":5,"retry_time":5,"max_retry":3}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Instance[0].RetryMax != 0 {
		t.Fatalf("expected misspelled max_retry to be ignored, got %d", c.Instance[0].RetryMax)
	}
	if c.FilePath() != path {
		t.Fatalf("expected FilePath %q, got %q", path, c.FilePath())
	}
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	c := &Config{
		LogLevel: 0,
		LogPath:  "d.log",
		Instance: []ConfigInstance{{Username: "u", Password: "p", KeepAlive: 5, RetryTime: 7, RetryMax: 3}},
	}
	c.SetFilePath(path)
	if err := c.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Instance[0].RetryTime != 7 || got.Instance[0].RetryMax != 3 || got.LogPath != "d.log" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestPauseDurationDefault(t *testing.T) {
	c := &Config{}
	if got := c.PauseDuration(); got != 10*time.Minute {
		t.Fatalf("expected 10m default, got %v", got)
	}
	c.PauseDurationSeconds = 1
	if got := c.PauseDuration(); got != time.Second {
		t.Fatalf("expected 1s, got %v", got)
	}
}
