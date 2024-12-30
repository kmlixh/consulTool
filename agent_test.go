package consulTool

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
)

const (
	testConsulAddress = "192.168.110.249:8500"
	testTimeout       = 10 * time.Second
)

func setupTestAgent(t *testing.T) *Agent {
	config := api.DefaultConfig()
	config.Address = testConsulAddress

	agent := NewAgent(config)
	t.Cleanup(func() {
		agent.Close()
	})
	return agent
}

func TestNewAgent(t *testing.T) {
	agent := setupTestAgent(t)

	if agent.consulClient == nil {
		t.Error("consulClient should not be nil")
	}
	if agent.consulAgent == nil {
		t.Error("consulAgent should not be nil")
	}
}

func TestAgent_Watch(t *testing.T) {
	agent := setupTestAgent(t)

	testCases := []struct {
		name        string
		serviceName string
		expectError bool
		checkError  func(error) bool
	}{
		{
			name:        "non-existent-service",
			serviceName: "non-existent-service",
			expectError: true,
			checkError: func(err error) bool {
				return err != nil && (strings.Contains(err.Error(), "context deadline exceeded") ||
					errors.Is(err, ErrServiceNotFound))
			},
		},
		{
			name:        "empty-service",
			serviceName: "",
			expectError: true,
			checkError: func(err error) bool {
				return errors.Is(err, ErrEmptyServiceName)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := agent.Watch(tc.serviceName)

			if tc.expectError && err == nil {
				t.Errorf("Expected error for service %s, got nil", tc.serviceName)
			}

			if tc.expectError && !tc.checkError(err) {
				t.Errorf("Unexpected error type for service %s: %v", tc.serviceName, err)
			}

			// 等待watch生效或超时
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			done := make(chan struct{})
			go func() {
				defer close(done)

				time.Sleep(time.Second)
				agent.mu.RLock()
				defer agent.mu.RUnlock()

				services, exists := agent.serviceMap[tc.serviceName]
				if exists {
					t.Logf("Service %s has %d instances", tc.serviceName, len(services))
				}
			}()

			select {
			case <-ctx.Done():
				t.Log("Watch test timed out")
			case <-done:
				t.Log("Watch test completed")
			}
		})
	}
}

func TestAgent_HttpClient(t *testing.T) {
	agent := setupTestAgent(t)

	client := agent.HttpClient()
	if client == nil {
		t.Error("HttpClient should not be nil")
	}

	// 测试transport配置
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Error("Expected *http.Transport")
		return
	}

	if transport.MaxIdleConnsPerHost != 200 {
		t.Errorf("Expected MaxIdleConnsPerHost to be 200, got %d", transport.MaxIdleConnsPerHost)
	}

	if transport.MaxConnsPerHost != 10000 {
		t.Errorf("Expected MaxConnsPerHost to be 10000, got %d", transport.MaxConnsPerHost)
	}

	// 测试HTTP客户端超时设置
	if client.Timeout != 5*time.Second {
		t.Errorf("Expected timeout to be 5s, got %v", client.Timeout)
	}
}

func TestAgent_Refresh(t *testing.T) {
	agent := setupTestAgent(t)

	testCases := []struct {
		name        string
		services    []string
		expectError bool
		checkError  func(error) bool
	}{
		{
			name:        "empty_list",
			services:    []string{},
			expectError: true,
			checkError: func(err error) bool {
				return errors.Is(err, ErrNothingToRefresh)
			},
		},
		{
			name:        "empty_service",
			services:    []string{""},
			expectError: true,
			checkError: func(err error) bool {
				return errors.Is(err, ErrEmptyServiceName)
			},
		},
		{
			name:        "non_existent",
			services:    []string{"non-existent-service"},
			expectError: true,
			checkError: func(err error) bool {
				return err != nil && (strings.Contains(err.Error(), "context deadline exceeded") ||
					errors.Is(err, ErrServiceNotFound))
			},
		},
		{
			name:        "multiple_non_existent",
			services:    []string{"service1", "service2"},
			expectError: true,
			checkError: func(err error) bool {
				return err != nil && (strings.Contains(err.Error(), "context deadline exceeded") ||
					errors.Is(err, ErrServiceNotFound))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			errCh := make(chan error, 1)
			go func() {
				errCh <- agent.Refresh(tc.services...)
			}()

			var err error
			select {
			case err = <-errCh:
			case <-ctx.Done():
				t.Log("Refresh operation timed out")
				err = ctx.Err()
			}

			if tc.expectError && err == nil {
				t.Error("Expected error but got nil")
			}

			if tc.expectError && !tc.checkError(err) {
				t.Errorf("Unexpected error type: %v", err)
			}
		})
	}
}

func TestAgent_PickService(t *testing.T) {
	agent := setupTestAgent(t)

	testCases := []struct {
		name        string
		serviceName string
		expectOk    bool
	}{
		{"non_existent", "non-existent-service", false},
		{"empty_name", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			resultCh := make(chan struct {
				service *api.AgentService
				ok      bool
			}, 1)

			go func() {
				service, ok := agent.pickService(tc.serviceName)
				resultCh <- struct {
					service *api.AgentService
					ok      bool
				}{service, ok}
			}()

			var service *api.AgentService
			var ok bool

			select {
			case result := <-resultCh:
				service = result.service
				ok = result.ok
			case <-ctx.Done():
				t.Log("PickService operation timed out")
				return
			}

			if ok != tc.expectOk {
				t.Errorf("Expected ok=%v, got %v", tc.expectOk, ok)
			}

			if tc.expectOk && service == nil {
				t.Error("Expected non-nil service when ok is true")
			}

			if !tc.expectOk && service != nil {
				t.Error("Expected nil service when ok is false")
			}
		})
	}

	// 测试轮询机制
	t.Run("round_robin", func(t *testing.T) {
		Debug(true)
		defer Debug(false)

		seen := make(map[string]bool)
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		for i := 0; i < 3; i++ {
			resultCh := make(chan struct {
				service *api.AgentService
				ok      bool
			}, 1)

			go func(iteration int) {
				service, ok := agent.pickService("test-service")
				resultCh <- struct {
					service *api.AgentService
					ok      bool
				}{service, ok}
			}(i)

			select {
			case result := <-resultCh:
				if result.ok && result.service != nil {
					seen[result.service.ID] = true
					t.Logf("Round %d picked service: ID=%s, Address=%s, Port=%d",
						i, result.service.ID, result.service.Address, result.service.Port)
				}
			case <-ctx.Done():
				t.Log("Round robin test timed out")
				return
			}

			time.Sleep(time.Millisecond * 100)
		}

		t.Logf("Unique services seen: %d", len(seen))
	})
}
