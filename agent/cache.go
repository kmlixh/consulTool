package agent

import (
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
)

// ServiceCache 服务缓存
type ServiceCache struct {
	services map[string]*CacheEntry
	mu       sync.RWMutex
	ttl      time.Duration
}

// CacheEntry 缓存条目
type CacheEntry struct {
	Services []*api.AgentService
	Expiry   time.Time
}

// NewServiceCache 创建新的服务缓存
func NewServiceCache(ttl time.Duration) *ServiceCache {
	return &ServiceCache{
		services: make(map[string]*CacheEntry),
		ttl:      ttl,
	}
}

// Set 设置缓存
func (c *ServiceCache) Set(serviceName string, services []*api.AgentService) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.services[serviceName] = &CacheEntry{
		Services: services,
		Expiry:   time.Now().Add(c.ttl),
	}
}

// Get 获取缓存
func (c *ServiceCache) Get(serviceName string) ([]*api.AgentService, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.services[serviceName]
	if !exists {
		return nil, false
	}

	// 检查是否过期
	if time.Now().After(entry.Expiry) {
		return nil, false
	}

	return entry.Services, true
}

// Delete 删除缓存
func (c *ServiceCache) Delete(serviceName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.services, serviceName)
}

// Clear 清空缓存
func (c *ServiceCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.services = make(map[string]*CacheEntry)
}

// CleanExpired 清理过期缓存
func (c *ServiceCache) CleanExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for serviceName, entry := range c.services {
		if now.After(entry.Expiry) {
			delete(c.services, serviceName)
		}
	}
}

// Count 获取缓存条目数量
func (c *ServiceCache) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.services)
}

// GetAll 获取所有缓存的服务名称
func (c *ServiceCache) GetAll() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	services := make([]string, 0, len(c.services))
	for service := range c.services {
		services = append(services, service)
	}
	return services
}

// StartCleanupTask 启动定期清理任务
func (c *ServiceCache) StartCleanupTask(interval time.Duration) chan struct{} {
	stop := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.CleanExpired()
			case <-stop:
				return
			}
		}
	}()

	return stop
}
