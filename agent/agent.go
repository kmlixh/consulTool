package agent

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	"github.com/kmlixh/consulTool"
	"github.com/kmlixh/consulTool/errors"
	"github.com/kmlixh/consulTool/logger"
)

const (
	defaultCacheTTL           = 30 * time.Second
	defaultCacheCleanInterval = 1 * time.Minute
	defaultRetryAttempts      = 3
	defaultRetryDelay         = 5 * time.Second
	defaultTimeout            = 5 * time.Second
	defaultWatchTimeout       = 5 * time.Minute
)

var (
	debug    = false
	MaxRound = uint32(10000)
)

// Agent Consul 代理
type Agent struct {
	consulConfig *consulTool.Config
	consulClient *api.Client
	consulAgent  *api.Agent
	serviceMap   map[string][]*api.AgentService
	watchMap     map[string]*watch.Plan
	roundMap     map[string]*uint32
	httpClient   *http.Client
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc

	// 新增字段
	cache     *sync.Map
	metrics   *Metrics
	cacheStop chan struct{}
	log       *logger.Logger
}

// Metrics 性能指标
type Metrics struct {
	mu                    sync.RWMutex
	serviceDiscoveryCount int64
	cacheHitCount         int64
	cacheMissCount        int64
	watchCount            int64
	healthCheckFailures   int64
}

// serviceCache 缓存条目
type serviceCache struct {
	Services  []*api.ServiceEntry
	ExpiresAt time.Time
}

// NewAgent 创建新的Agent
func NewAgent(config *consulTool.Config) *Agent {
	apiConfig := &api.Config{
		Address: config.GetAddress(),
		Scheme:  config.GetScheme(),
		Token:   config.GetToken(),
	}

	client, err := api.NewClient(apiConfig)
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Agent{
		consulClient: client,
		consulAgent:  client.Agent(),
		consulConfig: config,
		serviceMap:   make(map[string][]*api.AgentService),
		watchMap:     make(map[string]*watch.Plan),
		roundMap:     make(map[string]*uint32),
		httpClient:   &http.Client{},
		cache:        &sync.Map{},
		metrics:      &Metrics{},
		log:          logger.DefaultLogger(),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// initServiceLocked 初始化服务（需要在调用前加锁）
func (a *Agent) initServiceLocked(service string) error {
	if service == "" {
		return errors.ErrEmptyServiceName
	}

	ctx, cancel := context.WithTimeout(a.ctx, defaultTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		defer close(done)

		status, services, err := a.consulAgent.AgentHealthServiceByName(service)
		if err != nil {
			if _, ok := err.(*net.OpError); ok {
				done <- errors.ErrConsulNotAvailable
				return
			}
			done <- err
			return
		}

		if status != api.HealthPassing {
			a.metrics.IncrementHealthCheckFailure()
			done <- errors.ErrNoHealthyInstances
			return
		}

		if len(services) == 0 {
			done <- errors.ErrServiceNotFound
			return
		}

		var arrays []*api.AgentService
		for _, ss := range services {
			if ss.Service != nil {
				arrays = append(arrays, ss.Service)
			}
		}

		if len(arrays) == 0 {
			done <- errors.ErrNoHealthyInstances
			return
		}

		a.serviceMap[service] = arrays
		a.metrics.IncrementServiceDiscovery()
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to init service %s: %w", service, err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("init service %s timeout: %w", service, ctx.Err())
	}
}

// Watch 监控服务
func (a *Agent) Watch(serviceNames ...string) error {
	if len(serviceNames) == 0 {
		return errors.ErrNothingToRefresh
	}

	for _, name := range serviceNames {
		if name == "" {
			return errors.ErrEmptyServiceName
		}

		ctx, cancel := context.WithTimeout(a.ctx, defaultTimeout)
		defer cancel()

		if err := RetryWithTimeout(ctx, func() error {
			return a.watch(name)
		}); err != nil {
			return fmt.Errorf("failed to watch service %s: %w", name, err)
		}
	}
	return nil
}

// watch 监控单个服务
func (a *Agent) watch(service string) error {
	if service == "" {
		return errors.ErrEmptyServiceName
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	plan, ok := a.watchMap[service]
	if ok && plan != nil {
		return nil
	}

	if err := a.initServiceLocked(service); err != nil {
		if errors.IsConsulError(err) {
			return err
		}
		return fmt.Errorf("failed to initialize service: %w", err)
	}

	plan, err := watch.Parse(map[string]interface{}{
		"type":    "service",
		"service": service,
	})
	if err != nil {
		return fmt.Errorf("failed to parse watch plan: %w", err)
	}

	plan.Handler = func(index uint64, data interface{}) {
		services, ok := data.([]*api.ServiceEntry)
		if !ok {
			if debug {
				fmt.Printf("Failed to parse watch data for service %s\n", service)
			}
			return
		}

		a.mu.Lock()
		defer a.mu.Unlock()

		var array []*api.AgentService
		for _, v := range services {
			if v.Checks.AggregatedStatus() == api.HealthPassing {
				array = append(array, v.Service)
			}
		}

		if len(array) == 0 {
			if debug {
				fmt.Printf("No healthy instances found for service: %s\n", service)
			}
			a.metrics.IncrementHealthCheckFailure()
			// 清除本地缓存
			delete(a.serviceMap, service)
			a.cache.Delete(service)
			return
		}

		a.serviceMap[service] = array
		// 更新缓存
		a.cache.Store(service, &serviceCache{
			Services:  services,
			ExpiresAt: time.Now().Add(defaultCacheTTL),
		})

		if debug {
			fmt.Printf("Updated service %s with %d healthy instances\n", service, len(array))
		}
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				if debug {
					fmt.Printf("Recovered from watch panic for service %s: %v\n", service, r)
				}
			}
		}()

		if err := plan.RunWithClientAndHclog(a.consulClient, nil); err != nil && debug {
			fmt.Printf("Watch plan for service %s ended with error: %v\n", service, err)
		}
	}()

	a.watchMap[service] = plan
	return nil
}

// Refresh 刷新服务
func (a *Agent) Refresh(serviceNames ...string) error {
	if len(serviceNames) == 0 {
		return errors.ErrNothingToRefresh
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, name := range serviceNames {
		if name == "" {
			return errors.ErrEmptyServiceName
		}

		if plan, exists := a.watchMap[name]; exists && plan != nil {
			plan.Stop()
			delete(a.watchMap, name)
		}

		delete(a.serviceMap, name)
		a.cache.Delete(name)

		ctx, cancel := context.WithTimeout(a.ctx, defaultTimeout)
		defer cancel()

		if err := RetryWithTimeout(ctx, func() error {
			if err := a.initServiceLocked(name); err != nil {
				return fmt.Errorf("failed to refresh service %s: %w", name, err)
			}
			return a.watch(name)
		}); err != nil {
			return err
		}
	}
	return nil
}

// GetService 获取服务实例列表
func (a *Agent) GetService(name string) ([]*api.ServiceEntry, error) {
	// 尝试从缓存获取
	if cached, ok := a.cache.Load(name); ok {
		entry := cached.(*serviceCache)
		if !entry.IsExpired() {
			a.metrics.IncrementCacheHit()
			return entry.Services, nil
		}
	}

	a.metrics.IncrementCacheMiss()
	services, _, err := a.consulClient.Health().Service(name, "", true, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %v", err)
	}

	// 更新缓存
	a.cache.Store(name, &serviceCache{
		Services:  services,
		ExpiresAt: time.Now().Add(defaultCacheTTL),
	})

	a.metrics.IncrementServiceDiscovery()
	return services, nil
}

// GetMetrics 获取性能指标
func (a *Agent) GetMetrics() map[string]interface{} {
	a.metrics.mu.RLock()
	defer a.metrics.mu.RUnlock()

	return map[string]interface{}{
		"service_discovery_count": a.metrics.serviceDiscoveryCount,
		"cache_hit_count":         a.metrics.cacheHitCount,
		"cache_miss_count":        a.metrics.cacheMissCount,
		"watch_count":             a.metrics.watchCount,
		"health_check_failures":   a.metrics.healthCheckFailures,
	}
}

// IncrementServiceDiscovery 增加服务发现计数
func (m *Metrics) IncrementServiceDiscovery() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serviceDiscoveryCount++
}

// IncrementCacheHit 增加缓存命中计数
func (m *Metrics) IncrementCacheHit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cacheHitCount++
}

// IncrementCacheMiss 增加缓存未命中计数
func (m *Metrics) IncrementCacheMiss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cacheMissCount++
}

// IncrementWatch 增加监控计数
func (m *Metrics) IncrementWatch() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watchCount++
}

// IncrementHealthCheckFailure 增加健康检查失败计数
func (m *Metrics) IncrementHealthCheckFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthCheckFailures++
}

// IsExpired 检查缓存是否过期
func (c serviceCache) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// Close 关闭Agent
func (a *Agent) Close() error {
	a.cancel()
	return nil
}

// GetKV 获取指定键的值
func (a *Agent) GetKV(key string) (*api.KVPair, error) {
	if key == "" {
		return nil, errors.ErrEmptyKey
	}

	pair, _, err := a.consulClient.KV().Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get KV: %w", err)
	}

	if pair == nil {
		return nil, errors.ErrKeyNotFound
	}

	return pair, nil
}

// GetKVs 获取指定前缀的所有键值对
func (a *Agent) GetKVs(prefix string) (api.KVPairs, error) {
	if prefix == "" {
		return nil, errors.ErrEmptyKey
	}

	pairs, _, err := a.consulClient.KV().List(prefix, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list KVs with prefix %s: %w", prefix, err)
	}

	if len(pairs) == 0 {
		return nil, errors.ErrKeyNotFound
	}

	return pairs, nil
}

// PutKV 设置键值对
func (a *Agent) PutKV(key string, value []byte) error {
	if key == "" {
		return errors.ErrEmptyKey
	}

	p := &api.KVPair{
		Key:   key,
		Value: value,
	}

	_, err := a.consulClient.KV().Put(p, nil)
	if err != nil {
		return fmt.Errorf("failed to put KV: %w", err)
	}

	return nil
}

// DeleteKV 删除键值对
func (a *Agent) DeleteKV(key string) error {
	if key == "" {
		return errors.ErrEmptyKey
	}

	_, err := a.consulClient.KV().Delete(key, nil)
	if err != nil {
		return fmt.Errorf("failed to delete KV: %w", err)
	}

	return nil
}

// DeleteKVWithPrefix 删除指定前缀的所有键值对
func (a *Agent) DeleteKVWithPrefix(prefix string) error {
	if prefix == "" {
		return errors.ErrEmptyKey
	}

	_, err := a.consulClient.KV().DeleteTree(prefix, nil)
	if err != nil {
		return fmt.Errorf("failed to delete KV tree with prefix %s: %w", prefix, err)
	}

	return nil
}
