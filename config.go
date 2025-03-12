package consulTool

import (
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/kmlixh/consulTool/errors"
)

// Config 封装了 api.Config，提供安全的访问方式
type Config struct {
	config *api.Config
}

// ConfigOption 定义配置选项函数类型
type ConfigOption func(*Config)

// NewConfig 创建一个新的 Config 实例
func NewConfig(opts ...ConfigOption) *Config {
	cfg := &Config{
		config: api.DefaultConfig(),
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// NewConfigWithAddress 使用指定地址创建一个新的 Config 实例
func NewConfigWithAddress(address string) *Config {
	return NewConfig(WithAddress(address))
}

// WithAddress 设置地址的配置选项
func WithAddress(address string) ConfigOption {
	return func(c *Config) {
		c.SetAddress(address)
	}
}

// WithToken 设置令牌的配置选项
func WithToken(token string) ConfigOption {
	return func(c *Config) {
		c.SetToken(token)
	}
}

// WithScheme 设置协议的配置选项
func WithScheme(scheme string) ConfigOption {
	return func(c *Config) {
		c.SetScheme(scheme)
	}
}

// WithDatacenter 设置数据中心的配置选项
func WithDatacenter(datacenter string) ConfigOption {
	return func(c *Config) {
		c.SetDatacenter(datacenter)
	}
}

// WithTimeout 设置超时时间的配置选项
func WithTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) {
		c.config.WaitTime = timeout
	}
}

// SetAddress 设置 Consul 服务器地址
func (c *Config) SetAddress(address string) {
	c.config.Address = address
}

// GetAddress 获取 Consul 服务器地址
func (c *Config) GetAddress() string {
	return c.config.Address
}

// SetToken 设置访问令牌
func (c *Config) SetToken(token string) {
	c.config.Token = token
}

// GetToken 获取访问令牌
func (c *Config) GetToken() string {
	return c.config.Token
}

// SetScheme 设置协议方案（http/https）
func (c *Config) SetScheme(scheme string) {
	c.config.Scheme = scheme
}

// GetScheme 获取协议方案
func (c *Config) GetScheme() string {
	return c.config.Scheme
}

// SetDatacenter 设置数据中心
func (c *Config) SetDatacenter(datacenter string) {
	c.config.Datacenter = datacenter
}

// GetDatacenter 获取数据中心
func (c *Config) GetDatacenter() string {
	return c.config.Datacenter
}

// GetTimeout 获取超时时间
func (c *Config) GetTimeout() time.Duration {
	return c.config.WaitTime
}

// SetTimeout 设置超时时间
func (c *Config) SetTimeout(timeout time.Duration) {
	c.config.WaitTime = timeout
}

// Validate 验证配置是否有效
func (c *Config) Validate() error {
	if c.GetAddress() == "" {
		return errors.NewError(errors.ErrCodeConfigInvalid, "address is required", nil)
	}
	if c.GetScheme() != "" && c.GetScheme() != "http" && c.GetScheme() != "https" {
		return errors.NewError(errors.ErrCodeConfigInvalid, "scheme must be either http or https", nil)
	}
	return nil
}

// Clone 克隆配置
func (c *Config) Clone() *Config {
	newConfig := api.DefaultConfig()
	newConfig.Address = c.GetAddress()
	newConfig.Scheme = c.GetScheme()
	newConfig.Datacenter = c.GetDatacenter()
	newConfig.Token = c.GetToken()
	newConfig.WaitTime = c.GetTimeout()

	return &Config{
		config: newConfig,
	}
}

// getConfig 获取内部的 api.Config（仅供内部使用）
func (c *Config) getConfig() *api.Config {
	return c.config
}

// String 返回配置的字符串表示
func (c *Config) String() string {
	hasToken := "false"
	if c.GetToken() != "" {
		hasToken = "true"
	}
	return "Config{" +
		"Address: " + c.GetAddress() +
		", Scheme: " + c.GetScheme() +
		", Datacenter: " + c.GetDatacenter() +
		", HasToken: " + hasToken +
		", Timeout: " + c.GetTimeout().String() +
		"}"
}
