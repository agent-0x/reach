package client

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ServerConfig 单个服务器的连接配置
type ServerConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Token       string `yaml:"token"`
	Fingerprint string `yaml:"fingerprint"`
}

// Config 客户端配置文件
type Config struct {
	Servers map[string]*ServerConfig `yaml:"servers"`
}

// ConfigPath 返回客户端配置文件路径 (~/.reach/config.yaml)
func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".reach", "config.yaml")
}

// LoadClientConfig 从磁盘读取配置；文件不存在时返回空配置而不是报错
func LoadClientConfig() (*Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{Servers: make(map[string]*ServerConfig)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Servers == nil {
		cfg.Servers = make(map[string]*ServerConfig)
	}
	return &cfg, nil
}

// Save 将配置写入磁盘，目录 0700，文件 0600
func (c *Config) Save() error {
	path := ConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
