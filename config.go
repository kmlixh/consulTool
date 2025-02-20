package consulTool

import "github.com/hashicorp/consul/api"

// Config 封装了 api.Config，提供安全的访问方式
type Config struct {
	config *api.Config
}

// NewConfig 创建一个新的 Config 实例
func NewConfig() *Config {
	return &Config{
		config: api.DefaultConfig(),
	}
}

// NewConfigWithAddress 使用指定地址创建一个新的 Config 实例
func NewConfigWithAddress(address string) *Config {
	cfg := api.DefaultConfig()
	cfg.Address = address
	return &Config{
		config: cfg,
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

// GetConfig 获取内部的 api.Config（仅供内部使用）
func (c *Config) getConfig() *api.Config {
	return c.config
}
