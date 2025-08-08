package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/kmlixh/consulTool"
	"github.com/kmlixh/consulTool/logger"
)

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

	// 示例1: 监听单个 KV 键
	kvKey := "test/config"
	log.Infof("Starting to watch KV key: %s", kvKey)

	if err := agent.WatchKV(kvKey, func(index uint64, kvPair *api.KVPair) {
		log.Infof("KV changed - Index: %d, Key: %s, Value: %s",
			index, kvPair.Key, string(kvPair.Value))
	}); err != nil {
		log.Errorf("Failed to watch KV key %s: %v", kvKey, err)
		return
	}

	// 示例2: 监听 KV 前缀
	kvPrefix := "test/"
	log.Infof("Starting to watch KV prefix: %s", kvPrefix)

	if err := agent.WatchKVPrefix(kvPrefix, func(index uint64, pairs api.KVPairs) {
		log.Infof("KV prefix changed - Index: %d, Found %d pairs", index, len(pairs))
		for _, pair := range pairs {
			log.Infof("  Key: %s, Value: %s", pair.Key, string(pair.Value))
		}
	}); err != nil {
		log.Errorf("Failed to watch KV prefix %s: %v", kvPrefix, err)
		return
	}

	// 设置一些初始 KV 值
	log.Info("Setting initial KV values...")
	if err := agent.PutKV("test/config", []byte("initial-value")); err != nil {
		log.Errorf("Failed to put KV: %v", err)
	}

	if err := agent.PutKV("test/app1/config", []byte("app1-config")); err != nil {
		log.Errorf("Failed to put KV: %v", err)
	}

	if err := agent.PutKV("test/app2/config", []byte("app2-config")); err != nil {
		log.Errorf("Failed to put KV: %v", err)
	}

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 定期更新 KV 值以演示监听功能
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	counter := 0
	for {
		select {
		case <-ticker.C:
			counter++
			newValue := fmt.Sprintf("updated-value-%d", counter)
			log.Infof("Updating KV value to: %s", newValue)

			if err := agent.PutKV("test/config", []byte(newValue)); err != nil {
				log.Errorf("Failed to update KV: %v", err)
			}

			// 添加新的 KV 对
			newKey := fmt.Sprintf("test/dynamic-%d", counter)
			newValue2 := fmt.Sprintf("dynamic-value-%d", counter)
			if err := agent.PutKV(newKey, []byte(newValue2)); err != nil {
				log.Errorf("Failed to put new KV: %v", err)
			}

		case sig := <-sigChan:
			log.Infof("Received signal %v, stopping watches...", sig)

			// 停止 KV 监听
			if err := agent.StopWatchKV(kvKey); err != nil {
				log.Errorf("Failed to stop KV watch: %v", err)
			}

			if err := agent.StopWatchKVPrefix(kvPrefix); err != nil {
				log.Errorf("Failed to stop KV prefix watch: %v", err)
			}

			// 清理测试数据
			log.Info("Cleaning up test data...")
			if err := agent.DeleteKVWithPrefix("test/"); err != nil {
				log.Errorf("Failed to cleanup test data: %v", err)
			}

			return
		}
	}
}
