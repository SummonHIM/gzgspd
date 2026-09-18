package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
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
	LogPath  string           `json:"log_path"`
	Instance []ConfigInstance `json:"instance"`

	// PauseDurationSeconds 覆盖连续失败达到上限后的暂停时长（秒），为 0 时使用默认 10 分钟。
	PauseDurationSeconds int `json:"-"`

	// filePath 记录配置的加载来源路径，供 Save 使用，不参与序列化。
	filePath string
}

// FilePath 返回该配置的加载来源路径，可能为空。
func (c *Config) FilePath() string { return c.filePath }

// SetFilePath 记录配置来源路径，供 Save 使用。
func (c *Config) SetFilePath(path string) { c.filePath = path }

// PauseDuration 返回连续失败达到上限后的暂停时长，默认 10 分钟。
func (c *Config) PauseDuration() time.Duration {
	if c.PauseDurationSeconds > 0 {
		return time.Duration(c.PauseDurationSeconds) * time.Second
	}
	return 10 * time.Minute
}

// Save 将配置写回其来源路径，使用缩进 JSON。
func (c *Config) Save() error {
	if c.filePath == "" {
		return fmt.Errorf("config file path is not set")
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.filePath, data, 0o644)
}

// KeyIfName 返回该实例用于标识的身份串：配置了接口则用之，否则为 Auto。
func (inst ConfigInstance) KeyIfName() string {
	if inst.Interface == "" {
		return "Auto"
	}
	return inst.Interface
}

// Validate 校验配置内容
func (c *Config) Validate() error {
	if len(c.Instance) == 0 {
		return fmt.Errorf("at least one instance configuration is required")
	}

	for i, inst := range c.Instance {
		if inst.Username == "" {
			return fmt.Errorf("instance[%d]'s username cannot be empty", i)
		}
		if inst.Password == "" {
			return fmt.Errorf("instance[%d]'s password cannot be empty", i)
		}
		if inst.KeepAlive <= 0 {
			return fmt.Errorf("instance[%d]'s keep_alive must be greater than 0", i)
		}
		if inst.RetryMax < 0 {
			return fmt.Errorf("instance[%d]'s retry_max may not be negative", i)
		}
		if inst.RetryTime <= 0 {
			return fmt.Errorf("instance[%d]'s retry_time must be greater than 0", i)
		}
	}
	return nil
}

// LoadConfig 从文件读取并解析配置
// 传入Json配置文件路径，返回总配置结构体
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 校验配置
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	cfg.filePath = path
	return &cfg, nil
}
