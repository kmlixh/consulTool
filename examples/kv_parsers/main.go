// Package main demonstrates how to use the consulTool library for KV watching with JSON and YAML parsers
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kmlixh/consulTool"
	"github.com/kmlixh/consulTool/logger"
)

// Config 配置结构体
type Config struct {
	Database DatabaseConfig `json:"database" yaml:"database"`
	Cache    CacheConfig    `json:"cache" yaml:"cache"`
	Server   ServerConfig   `json:"server" yaml:"server"`
}

type DatabaseConfig struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	User     string `json:"user" yaml:"user"`
	Password string `json:"password" yaml:"password"`
	Database string `json:"database" yaml:"database"`
	MaxConn  int    `json:"max_conn" yaml:"max_conn"`
}

type CacheConfig struct {
	TTL     int    `json:"ttl" yaml:"ttl"`
	MaxSize int    `json:"max_size" yaml:"max_size"`
	Path    string `json:"path" yaml:"path"`
	Enabled bool   `json:"enabled" yaml:"enabled"`
}

type ServerConfig struct {
	Port    int      `json:"port" yaml:"port"`
	Host    string   `json:"host" yaml:"host"`
	Timeout int      `json:"timeout" yaml:"timeout"`
	Tags    []string `json:"tags" yaml:"tags"`
}

func main() {
	log := logger.NewLogger(&logger.Options{
		Level:  logger.DEBUG,
		Output: os.Stdout,
	})

	// 创建Consul配置
	config := consulTool.NewConfig().
		WithAddress("192.168.111.20:8500").
		WithScheme("http")

	// 创建Agent
	agent := consulTool.NewAgent(config)
	defer agent.Close()

	// 启用调试模式
	consulTool.Debug(true)

	// 创建配置结构体实例
	jsonConfig := &Config{}
	yamlConfig := &Config{}

	// 示例1: 使用 JSON 解析器监听 KV（带回调函数）
	jsonKey := "app/config/json"
	log.Infof("Starting to watch JSON KV key: %s", jsonKey)

	// 定义配置更新回调函数
	jsonCallback := func(key string, target interface{}) {
		if config, ok := target.(*Config); ok {
			log.Infof("JSON config updated for key %s:", key)
			log.Infof("  Database: Host=%s, Port=%d, MaxConn=%d",
				config.Database.Host, config.Database.Port, config.Database.MaxConn)
			log.Infof("  Cache: TTL=%d, MaxSize=%d, Enabled=%v",
				config.Cache.TTL, config.Cache.MaxSize, config.Cache.Enabled)
			log.Infof("  Server: Port=%d, Host=%s",
				config.Server.Port, config.Server.Host)

			// 这里可以添加配置更新后的业务逻辑
			// 例如：重新初始化数据库连接、更新缓存配置等
		}
	}

	jsonWatcher := consulTool.NewJSONKvWatchFunc(jsonConfig, jsonCallback)
	if err := agent.WatchKV(jsonKey, jsonWatcher); err != nil {
		log.Errorf("Failed to watch JSON KV key %s: %v", jsonKey, err)
		return
	}

	// 示例2: 使用 YAML 解析器监听 KV（带回调函数）
	yamlKey := "app/config/yaml"
	log.Infof("Starting to watch YAML KV key: %s", yamlKey)

	// 定义配置更新回调函数
	yamlCallback := func(key string, target interface{}) {
		if config, ok := target.(*Config); ok {
			log.Infof("YAML config updated for key %s:", key)
			log.Infof("  Database: Host=%s, Port=%d, MaxConn=%d",
				config.Database.Host, config.Database.Port, config.Database.MaxConn)
			log.Infof("  Cache: TTL=%d, MaxSize=%d, Enabled=%v",
				config.Cache.TTL, config.Cache.MaxSize, config.Cache.Enabled)
			log.Infof("  Server: Port=%d, Host=%s",
				config.Server.Port, config.Server.Host)

			// 这里可以添加配置更新后的业务逻辑
			// 例如：重新初始化数据库连接、更新缓存配置等
		}
	}

	yamlWatcher := consulTool.NewYamlKvWatchFunc(yamlConfig, yamlCallback)
	if err := agent.WatchKV(yamlKey, yamlWatcher); err != nil {
		log.Errorf("Failed to watch YAML KV key %s: %v", yamlKey, err)
		return
	}

	// 设置初始 JSON 配置
	jsonData := &Config{
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "admin",
			Password: "secret",
			Database: "myapp",
			MaxConn:  100,
		},
		Cache: CacheConfig{
			TTL:     300,
			MaxSize: 1000,
			Path:    "/tmp/cache",
			Enabled: true,
		},
		Server: ServerConfig{
			Port:    8080,
			Host:    "0.0.0.0",
			Timeout: 30,
			Tags:    []string{"api", "v1"},
		},
	}

	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		log.Errorf("Failed to marshal JSON: %v", err)
		return
	}

	if err := agent.PutKV(jsonKey, jsonBytes); err != nil {
		log.Errorf("Failed to put JSON KV: %v", err)
	}

	// 设置初始 YAML 配置（支持环境变量）
	yamlData := `
database:
  host: ${DB_HOST:localhost}
  port: ${DB_PORT:5432}
  user: ${DB_USER:admin}
  password: ${DB_PASSWORD:secret}
  database: ${DB_NAME:myapp}
  max_conn: ${DB_MAX_CONN:100}

cache:
  ttl: ${CACHE_TTL:300}
  max_size: ${CACHE_MAX_SIZE:1000}
  path: ${CACHE_PATH:/tmp/cache}
  enabled: ${CACHE_ENABLED:true}

server:
  port: ${SERVER_PORT:8080}
  host: ${SERVER_HOST:0.0.0.0}
  timeout: ${SERVER_TIMEOUT:30}
  tags:
    - ${SERVER_TAG1:api}
    - ${SERVER_TAG2:v1}
`

	if err := agent.PutKV(yamlKey, []byte(yamlData)); err != nil {
		log.Errorf("Failed to put YAML KV: %v", err)
	}

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 定期更新配置以演示监听功能
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	counter := 0
	for {
		select {
		case <-ticker.C:
			counter++
			log.Infof("Updating configurations (iteration %d)...", counter)

			// 更新 JSON 配置
			jsonData.Database.MaxConn = 100 + counter
			jsonData.Cache.TTL = 300 + counter
			jsonData.Server.Port = 8080 + counter

			jsonBytes, err := json.Marshal(jsonData)
			if err != nil {
				log.Errorf("Failed to marshal JSON: %v", err)
				continue
			}

			if err := agent.PutKV(jsonKey, jsonBytes); err != nil {
				log.Errorf("Failed to update JSON KV: %v", err)
			}

			// 更新 YAML 配置
			updatedYamlData := fmt.Sprintf(`
database:
  host: ${DB_HOST:localhost}
  port: ${DB_PORT:5432}
  user: ${DB_USER:admin}
  password: ${DB_PASSWORD:secret}
  database: ${DB_NAME:myapp}
  max_conn: %d

cache:
  ttl: %d
  max_size: %d
  path: ${CACHE_PATH:/tmp/cache}
  enabled: ${CACHE_ENABLED:true}

server:
  port: %d
  host: ${SERVER_HOST:0.0.0.0}
  timeout: ${SERVER_TIMEOUT:30}
  tags:
    - ${SERVER_TAG1:api}
    - ${SERVER_TAG2:v1}
    - "iteration-%d"
`, 100+counter, 300+counter, 1000+counter, 8080+counter, counter)

			if err := agent.PutKV(yamlKey, []byte(updatedYamlData)); err != nil {
				log.Errorf("Failed to update YAML KV: %v", err)
			}

			// 打印当前配置状态
			log.Infof("Current JSON config - DB MaxConn: %d, Cache TTL: %d, Server Port: %d",
				jsonConfig.Database.MaxConn, jsonConfig.Cache.TTL, jsonConfig.Server.Port)

		case sig := <-sigChan:
			log.Infof("Received signal %v, stopping watches...", sig)

			// 停止 KV 监听
			if err := agent.StopWatchKV(jsonKey); err != nil {
				log.Errorf("Failed to stop JSON KV watch: %v", err)
			}

			if err := agent.StopWatchKV(yamlKey); err != nil {
				log.Errorf("Failed to stop YAML KV watch: %v", err)
			}

			// 清理测试数据
			log.Info("Cleaning up test data...")
			if err := agent.DeleteKVWithPrefix("app/"); err != nil {
				log.Errorf("Failed to cleanup test data: %v", err)
			}

			return
		}
	}
}
