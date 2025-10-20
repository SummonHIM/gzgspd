package src

import (
	"encoding/json"
	"fmt"
	"os"
)

// ConfigInstance 单个实例配置
type ConfigInstance struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	Interface  string `json:"interface"`
	UserAgent  string `json:"user_agent"`
	KeepAlive  int    `json:"keep_alive"`
	KAliveLink string `json:"keep_alive_link"`
	RetryMax   int    `json:"retry_max"`
	RetryTime  int    `json:"retry_time"`
}

// Config 总配置
type Config struct {
	LogLevel int              `json:"log_level"`
	Instance []ConfigInstance `json:"instance"`
}

// LoadConfig 从文件读取并解析配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("an error occurred while loading the configuration file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("an error occurred while parsing the configuration file: %w", err)
	}

	// 校验配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration file: %w", err)
	}

	return &cfg, nil
}

// Validate 校验配置内容
func (c *Config) Validate() error {
	if len(c.Instance) == 0 {
		return fmt.Errorf("at least one instance configuration is required")
	}

	for i, inst := range c.Instance {
		if inst.Username == "" {
			return fmt.Errorf("instance[%d]: username cannot be empty", i)
		}
		if inst.Password == "" {
			return fmt.Errorf("instance[%d]: password cannot be empty", i)
		}
		if inst.KeepAlive <= 0 {
			return fmt.Errorf("instance[%d]: keep_alive must be greater than 0", i)
		}
		if inst.RetryMax < 0 {
			return fmt.Errorf("instance[%d]: retry_max may not be negative", i)
		}
		if inst.RetryTime <= 0 {
			return fmt.Errorf("instance[%d]: retry_time must be greater than 0", i)
		}
	}
	return nil
}
