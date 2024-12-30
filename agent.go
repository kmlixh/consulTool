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
	consulConfig *api.Config
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

		ctx, cancel := context.WithTimeout(s.ctx, defaultTimeout)
		defer cancel()

		if err := RetryWithTimeout(ctx, func() error {
			return s.watch(name)
		}); err != nil {
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

	plan, er = watch.Parse(map[string]interface{}{
		"type":    "service",
		"service": service,
		"timeout": defaultTimeout.String(),
	})
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

	watchCtx, cancel := context.WithCancel(s.ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if debug {
					fmt.Printf("Recovered from watch panic for service %s: %v\n", service, r)
				}
			}
			cancel()
		}()

		if err := plan.RunWithClientAndHclog(s.consulClient, nil); err != nil && debug {
			fmt.Printf("Watch plan for service %s ended with error: %v\n", service, err)
		}
	}()

	go func() {
		<-watchCtx.Done()
		if debug {
			fmt.Printf("Stopping watch for service %s\n", service)
		}
		plan.Stop()
	}()

	s.watchMap[service] = plan
	return nil
}

func (s *Agent) initServiceLocked(service string) error {
	if service == "" {
		return ErrEmptyServiceName
	}

	ctx, cancel := context.WithTimeout(s.ctx, defaultTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		defer close(done)

		status, services, er := s.consulAgent.AgentHealthServiceByName(service)
		if er != nil {
			if _, ok := er.(*net.OpError); ok {
				done <- ErrConsulNotAvailable
				return
			}
			done <- er
			return
		}

		if status != api.HealthPassing {
			done <- ErrNoHealthyInstances
			return
		}

		if len(services) == 0 {
			done <- ErrServiceNotFound
			return
		}

		var arrays []*api.AgentService
		for _, ss := range services {
			if ss.Service != nil {
				arrays = append(arrays, ss.Service)
			}
		}

		if len(arrays) == 0 {
			done <- ErrNoHealthyInstances
			return
		}

		s.serviceMap[service] = arrays
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

		if plan, exists := s.watchMap[name]; exists && plan != nil {
			plan.Stop()
			delete(s.watchMap, name)
		}

		delete(s.serviceMap, name)

		ctx, cancel := context.WithTimeout(s.ctx, defaultTimeout)
		defer cancel()

		if err := RetryWithTimeout(ctx, func() error {
			if err := s.initServiceLocked(name); err != nil {
				return fmt.Errorf("failed to refresh service %s: %w", name, err)
			}
			return s.watch(name)
		}); err != nil {
			return err
		}
	}
	return nil
}

func NewAgent(config *api.Config) *Agent {
	if config == nil {
		config = api.DefaultConfig()
	}

	client, er := api.NewClient(config)
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
