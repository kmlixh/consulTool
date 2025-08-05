package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/kmlixh/consulTool"
	"github.com/kmlixh/consulTool/logger"
)

var (
	consulAddr      = "10.0.1.5:8500"
	consulToken     = "29b48cf4-f1f3-159e-4860-d1c9a43f6e91"
	testServiceID   = "test-service-1"
	testServiceName = "test-service"
	testPort        = 8080
)

func setupTestLogger() *logger.Logger {
	return logger.NewLogger(&logger.Options{
		Level:  logger.DEBUG,
		Output: os.Stdout,
	})
}

func setupHealthCheckServer(t *testing.T) (*http.Server, string) {
	// 启动健康检查服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 获取本地IP地址
	localIP := consulTool.GetOutIp(consulAddr)
	if localIP == "" {
		t.Fatal("Failed to get local IP address")
	}

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", testPort),
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			t.Logf("HTTP server error: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(time.Second)
	return server, localIP
}

func setupTestService(t *testing.T) (*consulTool.ServiceRegistrant, func(), error) {
	// 启动健康检查服务器
	server, localIP := setupHealthCheckServer(t)

	config := consulTool.NewConfig().
		WithAddress(consulAddr).
		WithToken(consulToken).
		WithScheme("http")
	registrant, err := consulTool.NewServiceRegistrantBuilder(config).
		WithName(testServiceName).
		WithID(testServiceID).
		WithPort(testPort).
		WithAddress(localIP).
		WithHealthCheckPath("/health").
		WithInterval("5s").
		WithTimeout("3s").
		WithDeregisterCriticalServiceAfter("30s").
		Build()

	if err != nil {
		return nil, nil, fmt.Errorf("failed to build service registrant: %v", err)
	}

	if err := registrant.RegisterService(); err != nil {
		return nil, nil, fmt.Errorf("failed to register service: %v", err)
	}

	// 等待服务注册生效和健康检查通过
	time.Sleep(10 * time.Second)

	cleanup := func() {
		if err := registrant.DeRegisterService(); err != nil {
			t.Logf("Failed to deregister service: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Logf("Failed to close health check server: %v", err)
		}
	}

	return registrant, cleanup, nil
}

func TestServiceDiscovery(t *testing.T) {
	registrant, cleanup, err := setupTestService(t)
	if err != nil {
		t.Fatalf("Failed to setup test service: %v", err)
	}
	defer cleanup()

	// 创建Agent进行服务发现
	config := consulTool.NewConfig().
		WithAddress(consulAddr).
		WithToken(consulToken).
		WithScheme("http")
	agent := consulTool.NewAgent(config)

	// 测试服务发现
	t.Run("Discover Service", func(t *testing.T) {
		// 等待服务注册和健康检查完成
		time.Sleep(10 * time.Second)

		services, err := agent.GetService(testServiceName)
		if err != nil {
			t.Fatalf("Failed to discover service: %v", err)
		}

		if len(services) == 0 {
			t.Fatal("No services found")
		}

		found := false
		for _, service := range services {
			if service.Service.ID == testServiceID {
				found = true
				if service.Service.Port != testPort {
					t.Errorf("Expected service port %d, got %d", testPort, service.Service.Port)
				}
				// 验证健康状态
				if service.Checks[0].Status != "passing" {
					t.Errorf("Service health check not passing, status: %s", service.Checks[0].Status)
				}
				break
			}
		}

		if !found {
			t.Errorf("Test service not found in discovered services")
		}
	})

	// 测试服务监控
	t.Run("Watch Service", func(t *testing.T) {
		// 先确保服务是健康的
		time.Sleep(5 * time.Second)

		// 启动监控
		if err := agent.Watch(testServiceName); err != nil {
			t.Fatalf("Failed to start watching service: %v", err)
		}

		// 等待监控启动
		time.Sleep(2 * time.Second)

		// 验证服务是否被监控到
		services, err := agent.GetService(testServiceName)
		if err != nil {
			t.Fatalf("Failed to get service: %v", err)
		}

		if len(services) == 0 {
			t.Fatal("Service should be monitored")
		}

		// 注销服务
		if err := registrant.DeRegisterService(); err != nil {
			t.Fatalf("Failed to deregister service: %v", err)
		}

		// 等待服务注销生效
		time.Sleep(5 * time.Second)

		// 验证服务已被移除
		services, err = agent.GetService(testServiceName)
		if err == nil && len(services) > 0 {
			t.Error("Service should not be available after deregistration")
		}
	})

	// 测试服务缓存
	t.Run("Service Cache", func(t *testing.T) {
		// 重新注册服务
		if err := registrant.RegisterService(); err != nil {
			t.Fatalf("Failed to register service: %v", err)
		}

		// 等待服务注册生效和健康检查通过
		time.Sleep(10 * time.Second)

		// 第一次调用应该会缓存
		start := time.Now()
		services, err := agent.GetService(testServiceName)
		if err != nil {
			t.Fatalf("Failed to get service: %v", err)
		}
		firstCallDuration := time.Since(start)

		// 验证服务健康状态
		if len(services) > 0 && services[0].Checks[0].Status != "passing" {
			t.Errorf("Service health check not passing, status: %s", services[0].Checks[0].Status)
		}

		// 第二次调用应该使用缓存
		start = time.Now()
		_, err = agent.GetService(testServiceName)
		if err != nil {
			t.Fatalf("Failed to get service from cache: %v", err)
		}
		secondCallDuration := time.Since(start)

		// 第二次调用应该明显快于第一次
		if secondCallDuration > firstCallDuration/2 {
			t.Errorf("Cache doesn't seem to be working: first call took %v, second call took %v",
				firstCallDuration, secondCallDuration)
		}
	})

	// 测试指标收集
	t.Run("Metrics Collection", func(t *testing.T) {
		metrics := agent.GetMetrics()
		if metrics["service_discovery_count"].(int64) == 0 {
			t.Error("Service discovery count should be greater than 0")
		}
		if metrics["cache_hit_count"].(int64) == 0 {
			t.Error("Cache hit count should be greater than 0")
		}
	})
}

func TestServiceRegistration(t *testing.T) {
	// 测试无配置的情况
	t.Run("Register Service Without Config", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when config is nil")
			}
		}()
		_, _ = consulTool.NewServiceRegistrantBuilder(nil).Build()
	})

	config := consulTool.NewConfig().
		WithAddress(consulAddr).
		WithToken(consulToken).
		WithScheme("http")

	// 测试服务注册
	t.Run("Register Service", func(t *testing.T) {
		// 启动健康检查服务器
		server, localIP := setupHealthCheckServer(t)
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(ctx); err != nil {
				t.Logf("Failed to close health check server: %v", err)
			}
		}()

		registrant, err := consulTool.NewServiceRegistrantBuilder(config).
			WithName(testServiceName).
			WithID(testServiceID).
			WithPort(testPort).
			WithAddress(localIP).
			WithHealthCheckPath("/health").
			WithInterval("5s").
			WithTimeout("3s").
			WithDeregisterCriticalServiceAfter("30s").
			Build()

		if err != nil {
			t.Fatalf("Failed to build service registrant: %v", err)
		}

		if err := registrant.RegisterService(); err != nil {
			t.Fatalf("Failed to register service: %v", err)
		}

		// 清理
		defer func() {
			if err := registrant.DeRegisterService(); err != nil {
				t.Logf("Failed to deregister service: %v", err)
			}
		}()

		// 等待服务注册生效和健康检查通过
		time.Sleep(10 * time.Second)

		// 验证服务已注册
		testAgent := consulTool.NewAgent(config)
		services, err := testAgent.GetService(testServiceName)
		if err != nil {
			t.Fatalf("Failed to get service: %v", err)
		}

		if len(services) == 0 {
			t.Fatal("Service was not registered")
		}

		// 验证服务属性
		found := false
		for _, service := range services {
			if service.Service.ID == testServiceID {
				found = true
				if service.Service.Port != testPort {
					t.Errorf("Expected service port %d, got %d", testPort, service.Service.Port)
				}
				if service.Service.Address != localIP {
					t.Errorf("Expected service address %s, got %s", localIP, service.Service.Address)
				}
				// 验证健康状态
				if service.Checks[0].Status != "passing" {
					t.Errorf("Service health check not passing, status: %s", service.Checks[0].Status)
				}
				break
			}
		}

		if !found {
			t.Error("Test service not found in discovered services")
		}
	})

	// 测试服务注册参数验证
	t.Run("Validate Registration Parameters", func(t *testing.T) {
		// 测试空服务名
		_, err := consulTool.NewServiceRegistrantBuilder(config).
			WithName("").
			WithID(testServiceID).
			WithPort(testPort).
			Build()
		if err == nil {
			t.Error("Expected error for empty service name")
		}

		// 测试空服务ID
		_, err = consulTool.NewServiceRegistrantBuilder(config).
			WithName(testServiceName).
			WithID("").
			WithPort(testPort).
			Build()
		if err == nil {
			t.Error("Expected error for empty service ID")
		}

		// 测试无效端口
		_, err = consulTool.NewServiceRegistrantBuilder(config).
			WithName(testServiceName).
			WithID(testServiceID).
			WithPort(0).
			Build()
		if err == nil {
			t.Error("Expected error for invalid port")
		}

		// 测试空Consul地址
		emptyConfig := consulTool.NewConfig()
		_, err = consulTool.NewServiceRegistrantBuilder(emptyConfig).
			WithName(testServiceName).
			WithID(testServiceID).
			WithPort(testPort).
			Build()
		if err == nil || err.Error() != "consul address is required" {
			t.Errorf("Expected error 'consul address is required', got: %v", err)
		}
	})
}
