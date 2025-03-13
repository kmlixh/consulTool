// Package main demonstrates how to use the consulTool library for service discovery and key-value watching
package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	// 创建Agent用于服务发现
	agent := consulTool.NewAgent(config)

	// 启动健康检查服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    ":8081",
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("HTTP server error: %v", err)
		}
	}()

	// 监控服务变化
	serviceName := "test-service"
	if err := agent.Watch(serviceName); err != nil {
		log.Errorf("Failed to watch service %s: %v", serviceName, err)
		return
	}

	// 注册服务
	serviceID := "test-service-1"
	registrant, err := consulTool.NewServiceRegistrantBuilder(config).
		WithName(serviceName).
		WithID(serviceID).
		WithPort(8081).
		WithHealthCheckPath("/health").
		Build()

	if err != nil {
		log.Errorf("Failed to build service registrant: %v", err)
		return
	}

	if err := registrant.RegisterService(); err != nil {
		log.Errorf("Failed to register service: %v", err)
		return
	}

	log.Infof("Service %s registered successfully with ID %s", serviceName, serviceID)

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 定期获取服务实例
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			services, err := agent.GetService(serviceName)
			if err != nil {
				log.Errorf("Failed to get service %s: %v", serviceName, err)
				continue
			}
			log.Infof("Found %d instances of service %s", len(services), serviceName)
			for _, service := range services {
				fmt.Printf("Service ID: %s, Address: %s, Port: %d, Health: %s\n",
					service.Service.ID, service.Service.Address, service.Service.Port,
					service.Checks[0].Status)
			}
		case sig := <-sigChan:
			log.Infof("Received signal %v, deregistering service...", sig)
			if err := registrant.DeRegisterService(); err != nil {
				log.Errorf("Failed to deregister service: %v", err)
			} else {
				log.Info("Service deregistered successfully")
			}
			if err := server.Close(); err != nil {
				log.Errorf("Failed to close health check server: %v", err)
			}
			return
		}
	}
}
