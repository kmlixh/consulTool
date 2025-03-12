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

	defaultRetryAttempts = 3
	defaultRetryDelay    = 1 * time.Second
	defaultTimeout       = 5 * time.Second
)

var MaxRound uint32 = 10000

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
					errors.Is(err, ErrServiceNotFound) {
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

func Debug(_debug bool) {
	debug = _debug
}

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

func (s *Agent) Watch(serviceNames ...string) error {
	if len(serviceNames) == 0 {
		return ErrNothingToRefresh
	}

	for _, name := range serviceNames {
		if name == "" {
			return ErrEmptyServiceName
		}

		if err := s.watch(name); err != nil {
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
			return
		}

		s.serviceMap[service] = array
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
	return nil
}

func (s *Agent) Refresh(serviceNames ...string) error {
	if len(serviceNames) == 0 {
		return ErrNothingToRefresh
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, name := range serviceNames {
		if name == "" {
			return ErrEmptyServiceName
		}

		// 停止现有的Watch
		if plan, exists := s.watchMap[name]; exists && plan != nil {
			plan.Stop()
			delete(s.watchMap, name)
		}

		// 清除本地缓存
		delete(s.serviceMap, name)

		// 重新初始化服务
		if err := s.initServiceLocked(name); err != nil {
			if errors.Is(err, ErrServiceNotFound) ||
				errors.Is(err, ErrEmptyServiceName) ||
				errors.Is(err, ErrNoHealthyInstances) {
				return err
			}
			return fmt.Errorf("failed to refresh service %s: %w", name, err)
		}

		// 重新启动Watch
		if err := s.watch(name); err != nil {
			return fmt.Errorf("failed to restart watch for service %s: %w", name, err)
		}
	}
	return nil
}

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
	}
}

func (s *Agent) Close() error {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()

	for service, plan := range s.watchMap {
		if plan != nil {
			if debug {
				fmt.Printf("Stopping watch for service %s\n", service)
			}
			plan.Stop()
		}
		delete(s.watchMap, service)
	}

	if s.httpClient != nil {
		if transport, ok := s.httpClient.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}

	s.serviceMap = nil
	s.roundMap = nil
	s.httpClient = nil
	return nil
}

func (s *Agent) GetService(service string) ([]*api.ServiceEntry, error) {
	if service == "" {
		return nil, ErrEmptyServiceName
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 先尝试从本地缓存获取
	if services, ok := s.serviceMap[service]; ok && len(services) > 0 {
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
	entries, _, err := s.consulClient.Health().Service(service, "", true, nil)
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
		s.serviceMap[service] = services
	}

	return entries, nil
}
