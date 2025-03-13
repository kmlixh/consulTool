package consulTool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
)

var (
	debug                 = false
	ErrServiceNotFound    = errors.New("service not found")
	ErrNoHealthyInstances = errors.New("no healthy instances available")
	ErrConsulNotAvailable = errors.New("consul service not available")
	ErrEmptyServiceName   = errors.New("empty service name")
	ErrNothingToRefresh   = errors.New("nothing to refresh")
	ErrEmptyKey           = errors.New("empty key")
	ErrKeyNotFound        = errors.New("key not found")

	defaultRetryAttempts = 3
	defaultRetryDelay    = 1 * time.Second
	defaultTimeout       = 5 * time.Second
	defaultCacheTTL      = 30 * time.Second
)

var MaxRound uint32 = 10000

// Metrics 性能指标
type Metrics struct {
	mu                    sync.RWMutex
	serviceDiscoveryCount int64
	cacheHitCount         int64
	cacheMissCount        int64
	watchCount            int64
	healthCheckFailures   int64
	kvAccessCount         int64
}

// serviceCache 缓存条目
type serviceCache struct {
	Services  []*api.ServiceEntry
	ExpiresAt time.Time
}

// IsExpired 检查缓存是否过期
func (c *serviceCache) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

type Agent struct {
	consulConfig *Config
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
	cache   *sync.Map
	metrics *Metrics
}

// Debug 设置调试模式
func Debug(_debug bool) {
	debug = _debug
}

// RetryWithTimeout 重试函数，带超时控制
func RetryWithTimeout(ctx context.Context, operation func() error) error {
	var lastErr error
	for attempt := 0; attempt < defaultRetryAttempts; attempt++ {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("%w: %v", ctx.Err(), lastErr)
			}
			return ctx.Err()
		default:
			if err := operation(); err == nil {
				return nil
			} else {
				lastErr = err
				// 如果是已知的错误类型，直接返回
				if errors.Is(err, ErrEmptyServiceName) ||
					errors.Is(err, ErrNothingToRefresh) ||
					errors.Is(err, ErrServiceNotFound) ||
					errors.Is(err, ErrEmptyKey) ||
					errors.Is(err, ErrKeyNotFound) {
					return err
				}
				if attempt < defaultRetryAttempts-1 {
					time.Sleep(defaultRetryDelay)
				}
			}
		}
	}
	return lastErr
}

// NewAgent 创建新的 Agent
func NewAgent(config *Config) *Agent {
	if config == nil {
		config = NewConfig()
	}

	client, er := api.NewClient(config.getConfig())
	if er != nil {
		panic(fmt.Errorf("failed to create consul client: %w", er))
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Agent{
		consulConfig: config,
		consulClient: client,
		consulAgent:  client.Agent(),
		serviceMap:   make(map[string][]*api.AgentService),
		watchMap:     make(map[string]*watch.Plan),
		roundMap:     make(map[string]*uint32),
		ctx:          ctx,
		cancel:       cancel,
		cache:        &sync.Map{},
		metrics:      &Metrics{},
	}
}

// HttpClient 获取 HTTP 客户端
func (a *Agent) HttpClient() *http.Client {
	if a.httpClient == nil {
		dialer := &net.Dialer{
			Timeout: defaultTimeout,
		}
		httpCLients := &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				MaxIdleConnsPerHost:   200,
				MaxConnsPerHost:       10000,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   30 * time.Second,
				ExpectContinueTimeout: 10 * time.Second,
				DisableKeepAlives:     true,
				DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
					host, port, err := net.SplitHostPort(address)
					if err != nil {
						return nil, fmt.Errorf("failed to split host port: %w", err)
					}

					ss, ok := a.pickService(host)
					if ok {
						host = ss.Address
						port = strconv.Itoa(ss.Port)
						if conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(host, port)); err == nil {
							return conn, nil
						}
					}
					return dialer.DialContext(ctx, network, address)
				},
			},
		}
		a.httpClient = httpCLients
	}
	return a.httpClient
}

func (s *Agent) pickService(service string) (*api.AgentService, bool) {
	if service == "" {
		return nil, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.initServiceLocked(service); err != nil {
		if debug {
			fmt.Printf("Failed to init service %s: %v\n", service, err)
		}
		return nil, false
	}

	sArrays, exists := s.serviceMap[service]
	if !exists || len(sArrays) == 0 {
		return nil, false
	}

	i, ok := s.roundMap[service]
	if !ok {
		d := uint32(0)
		i = &d
		s.roundMap[service] = &d
	}

	idx := atomic.LoadUint32(i) % uint32(len(sArrays))
	if atomic.LoadUint32(i) < MaxRound {
		atomic.AddUint32(i, 1)
	} else {
		atomic.StoreUint32(i, 0)
	}

	if debug {
		fmt.Printf("Service %s: picked instance %d of %d\n", service, idx, len(sArrays))
	}

	return sArrays[idx], true
}

// Watch 监控服务
func (a *Agent) Watch(serviceNames ...string) error {
	if len(serviceNames) == 0 {
		return ErrNothingToRefresh
	}

	for _, name := range serviceNames {
		if name == "" {
			return ErrEmptyServiceName
		}

		if err := a.watch(name); err != nil {
			return fmt.Errorf("failed to watch service %s: %w", name, err)
		}
	}
	return nil
}

func (s *Agent) watch(service string) (er error) {
	if service == "" {
		return ErrEmptyServiceName
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	plan, ok := s.watchMap[service]
	if ok && plan != nil {
		return nil
	}

	if err := s.initServiceLocked(service); err != nil {
		if errors.Is(err, ErrServiceNotFound) ||
			errors.Is(err, ErrEmptyServiceName) ||
			errors.Is(err, ErrNoHealthyInstances) {
			return err
		}
		return fmt.Errorf("failed to initialize service: %w", err)
	}

	params := map[string]interface{}{
		"type":    "service",
		"service": service,
	}

	plan, er = watch.Parse(params)
	if er != nil {
		return fmt.Errorf("failed to parse watch plan: %w", er)
	}

	plan.Handler = func(index uint64, data interface{}) {
		services, ok := data.([]*api.ServiceEntry)
		if !ok {
			if debug {
				fmt.Printf("Failed to parse watch data for service %s\n", service)
			}
			return
		}

		s.mu.Lock()
		defer s.mu.Unlock()

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
			// 清除本地缓存
			delete(s.serviceMap, service)
			if s.cache != nil {
				s.cache.Delete(service)
			}
			if s.metrics != nil {
				s.metrics.IncrementHealthCheckFailure()
			}
			return
		}

		s.serviceMap[service] = array
		// 更新缓存
		if s.cache != nil {
			s.cache.Store(service, &serviceCache{
				Services:  services,
				ExpiresAt: time.Now().Add(defaultCacheTTL),
			})
		}

		if s.metrics != nil {
			s.metrics.IncrementWatch()
		}

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

		if err := plan.RunWithClientAndHclog(s.consulClient, nil); err != nil && debug {
			fmt.Printf("Watch plan for service %s ended with error: %v\n", service, err)
		}
	}()

	s.watchMap[service] = plan
	return nil
}

func (s *Agent) initServiceLocked(service string) error {
	if service == "" {
		return ErrEmptyServiceName
	}

	// 从Consul获取服务信息
	entries, _, err := s.consulClient.Health().Service(service, "", true, nil)
	if err != nil {
		if _, ok := err.(*net.OpError); ok {
			return ErrConsulNotAvailable
		}
		return fmt.Errorf("failed to get service: %w", err)
	}

	if len(entries) == 0 {
		return ErrNoHealthyInstances
	}

	// 更新本地缓存
	var services []*api.AgentService
	for _, entry := range entries {
		if entry.Checks.AggregatedStatus() == api.HealthPassing {
			services = append(services, entry.Service)
		}
	}

	if len(services) == 0 {
		return ErrNoHealthyInstances
	}

	s.serviceMap[service] = services

	if s.metrics != nil {
		s.metrics.IncrementServiceDiscovery()
	}

	return nil
}

// Refresh 刷新服务
func (a *Agent) Refresh(serviceNames ...string) error {
	if len(serviceNames) == 0 {
		return ErrNothingToRefresh
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, name := range serviceNames {
		if name == "" {
			return ErrEmptyServiceName
		}

		// 停止现有的Watch
		if plan, exists := a.watchMap[name]; exists && plan != nil {
			plan.Stop()
			delete(a.watchMap, name)
		}

		// 清除本地缓存
		delete(a.serviceMap, name)
		if a.cache != nil {
			a.cache.Delete(name)
		}

		// 重新初始化服务
		if err := a.initServiceLocked(name); err != nil {
			if errors.Is(err, ErrServiceNotFound) ||
				errors.Is(err, ErrEmptyServiceName) ||
				errors.Is(err, ErrNoHealthyInstances) {
				return err
			}
			return fmt.Errorf("failed to refresh service %s: %w", name, err)
		}

		// 重新启动Watch
		if err := a.watch(name); err != nil {
			return fmt.Errorf("failed to restart watch for service %s: %w", name, err)
		}
	}
	return nil
}

// GetService 获取服务实例列表
func (a *Agent) GetService(name string) ([]*api.ServiceEntry, error) {
	if name == "" {
		return nil, ErrEmptyServiceName
	}

	// 尝试从缓存获取
	if a.cache != nil {
		if cached, ok := a.cache.Load(name); ok {
			entry := cached.(*serviceCache)
			if !entry.IsExpired() {
				if a.metrics != nil {
					a.metrics.IncrementCacheHit()
				}
				return entry.Services, nil
			}
		}
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	// 先尝试从本地缓存获取
	if services, ok := a.serviceMap[name]; ok && len(services) > 0 {
		var entries []*api.ServiceEntry
		for _, svc := range services {
			entries = append(entries, &api.ServiceEntry{
				Service: svc,
				Checks: api.HealthChecks{
					&api.HealthCheck{
						Status: api.HealthPassing,
					},
				},
			})
		}
		return entries, nil
	}

	// 如果本地缓存没有，则从Consul获取
	if a.metrics != nil {
		a.metrics.IncrementCacheMiss()
	}

	entries, _, err := a.consulClient.Health().Service(name, "", true, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	if len(entries) == 0 {
		return nil, ErrNoHealthyInstances
	}

	// 更新本地缓存
	var services []*api.AgentService
	for _, entry := range entries {
		if entry.Checks.AggregatedStatus() == api.HealthPassing {
			services = append(services, entry.Service)
		}
	}

	if len(services) > 0 {
		a.serviceMap[name] = services
	}

	// 更新缓存
	if a.cache != nil {
		a.cache.Store(name, &serviceCache{
			Services:  entries,
			ExpiresAt: time.Now().Add(defaultCacheTTL),
		})
	}

	if a.metrics != nil {
		a.metrics.IncrementServiceDiscovery()
	}

	return entries, nil
}

// Close 关闭 Agent
func (a *Agent) Close() error {
	a.cancel()
	a.mu.Lock()
	defer a.mu.Unlock()

	for service, plan := range a.watchMap {
		if plan != nil {
			if debug {
				fmt.Printf("Stopping watch for service %s\n", service)
			}
			plan.Stop()
		}
		delete(a.watchMap, service)
	}

	if a.httpClient != nil {
		if transport, ok := a.httpClient.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}

	a.serviceMap = nil
	a.roundMap = nil
	a.httpClient = nil
	return nil
}

// GetKV 获取指定键的值
func (a *Agent) GetKV(key string) (*api.KVPair, error) {
	if key == "" {
		return nil, ErrEmptyKey
	}

	pair, _, err := a.consulClient.KV().Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get KV: %w", err)
	}

	if pair == nil {
		return nil, ErrKeyNotFound
	}

	if a.metrics != nil {
		a.metrics.IncrementKVAccess()
	}

	return pair, nil
}

// GetKVs 获取指定前缀的所有键值对
func (a *Agent) GetKVs(prefix string) (api.KVPairs, error) {
	if prefix == "" {
		return nil, ErrEmptyKey
	}

	pairs, _, err := a.consulClient.KV().List(prefix, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list KVs with prefix %s: %w", prefix, err)
	}

	if len(pairs) == 0 {
		return nil, ErrKeyNotFound
	}

	if a.metrics != nil {
		a.metrics.IncrementKVAccess()
	}

	return pairs, nil
}

// PutKV 设置键值对
func (a *Agent) PutKV(key string, value []byte) error {
	if key == "" {
		return ErrEmptyKey
	}

	p := &api.KVPair{
		Key:   key,
		Value: value,
	}

	_, err := a.consulClient.KV().Put(p, nil)
	if err != nil {
		return fmt.Errorf("failed to put KV: %w", err)
	}

	if a.metrics != nil {
		a.metrics.IncrementKVAccess()
	}

	return nil
}

// DeleteKV 删除键值对
func (a *Agent) DeleteKV(key string) error {
	if key == "" {
		return ErrEmptyKey
	}

	_, err := a.consulClient.KV().Delete(key, nil)
	if err != nil {
		return fmt.Errorf("failed to delete KV: %w", err)
	}

	if a.metrics != nil {
		a.metrics.IncrementKVAccess()
	}

	return nil
}

// DeleteKVWithPrefix 删除指定前缀的所有键值对
func (a *Agent) DeleteKVWithPrefix(prefix string) error {
	if prefix == "" {
		return ErrEmptyKey
	}

	_, err := a.consulClient.KV().DeleteTree(prefix, nil)
	if err != nil {
		return fmt.Errorf("failed to delete KV tree with prefix %s: %w", prefix, err)
	}

	if a.metrics != nil {
		a.metrics.IncrementKVAccess()
	}

	return nil
}

// GetMetrics 获取性能指标
func (a *Agent) GetMetrics() map[string]interface{} {
	if a.metrics == nil {
		return map[string]interface{}{}
	}

	a.metrics.mu.RLock()
	defer a.metrics.mu.RUnlock()

	return map[string]interface{}{
		"service_discovery_count": a.metrics.serviceDiscoveryCount,
		"cache_hit_count":         a.metrics.cacheHitCount,
		"cache_miss_count":        a.metrics.cacheMissCount,
		"watch_count":             a.metrics.watchCount,
		"health_check_failures":   a.metrics.healthCheckFailures,
		"kv_access_count":         a.metrics.kvAccessCount,
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

// IncrementKVAccess 增加 KV 访问计数
func (m *Metrics) IncrementKVAccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.kvAccessCount++
}
