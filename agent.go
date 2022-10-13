package consulTool

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

var debug = false

var MaxRound uint32 = 10000

type Agent struct {
	consulConfig *api.Config
	consulClient *api.Client
	consulAgent  *api.Agent
	serviceMap   map[string][]*api.AgentService
	watchMap     map[string]*watch.Plan
	roundMap     map[string]*uint32
	httpClient   *http.Client
}

func Debug(_debug bool) {
	debug = _debug
}

func (a *Agent) HttpClient() *http.Client {
	if a.httpClient == nil {
		dialer := &net.Dialer{
			Timeout: 39 * time.Second,
		}
		httpCLients := &http.Client{
			Timeout: time.Duration(5) * time.Second, //超时时间
			Transport: &http.Transport{
				MaxIdleConnsPerHost:   200,   //单个路由最大空闲连接数
				MaxConnsPerHost:       10000, //单个路由最大连接数
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   30 * time.Second,
				ExpectContinueTimeout: 10 * time.Second,
				DisableKeepAlives:     true,
				DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
					host, port, err := net.SplitHostPort(address)
					if err != nil {
						return nil, err
					}
					//通过自定义nameserver获取域名解析的IP
					ss, ok := a.pickService(host)
					// 创建链接
					if ok {
						host = ss.Address
						port = strconv.Itoa(ss.Port)
						conn, err := dialer.DialContext(ctx, network, host+":"+port)
						if err == nil {
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
	if er := s.initService(service); er != nil {
		return nil, false
	}
	sArrays, _ := s.serviceMap[service]
	if sArrays == nil || len(sArrays) == 0 {
		return nil, false
	}
	//Round Robin
	i, ok := s.roundMap[service]
	if !ok {
		d := uint32(0)
		i = &d
		s.roundMap[service] = &d
	}
	idx := (*i) % uint32(len(sArrays))
	if *i < MaxRound {
		atomic.AddUint32(i, 1)
	} else {
		atomic.StoreUint32(i, 0)
	}
	if debug {
		fmt.Println("Round Robin index was:", *i)
	}
	return sArrays[idx], true
}
func (s *Agent) Watch(serviceNames ...string) error {
	if len(serviceNames) == 0 {
		panic(errors.New("watch noting"))
	}
	if len(serviceNames) > 0 {
		for _, v := range serviceNames {
			s.watch(v)
		}
	}
	return nil
}
func (s *Agent) watch(service string) (er error) {
	plan, ok := s.watchMap[service]
	if ok {
		return nil
	}
	plan, er = watch.Parse(map[string]interface{}{"type": "service", "service": service})
	if er != nil {
		return
	}
	plan.Handler = func(index uint64, data interface{}) {
		services, ok := data.([]*api.ServiceEntry)
		if !ok {
			er = errors.New("parse data to ServiceEntry failed")
			return
		}
		var array []*api.AgentService
		for _, v := range services {
			if v.Checks.AggregatedStatus() == "passing" {
				array = append(array, v.Service)
			}
		}
		s.serviceMap[service] = array
	}
	go plan.RunWithClientAndHclog(s.consulClient, nil)
	s.watchMap[service] = plan
	return nil
}
func (s *Agent) initService(service string) error {
	arrays, ok := s.serviceMap[service]
	if ok {
		return nil
	} else {
		arrays = []*api.AgentService{}
	}
	status, services, er := s.consulAgent.AgentHealthServiceByName(service)
	if er != nil {
		return er
	}
	if status != "passing" {
		return errors.New(fmt.Sprintf("there was no passing service named %s", service))
	}
	for _, ss := range services {
		arrays = append(arrays, ss.Service)
	}
	s.serviceMap[service] = arrays
	s.watch(service)
	return nil
}
func (s *Agent) Refresh(serviceNames ...string) error {
	if len(serviceNames) == 0 {
		return errors.New("nothing to refresh")
	}
	for _, name := range serviceNames {
		delete(s.serviceMap, name)
		s.initService(name)
	}
	return nil
}

func NewAgent(config *api.Config) *Agent {
	client, er := api.NewClient(config)
	if er != nil {
		panic(er)
	}
	return &Agent{consulConfig: config, consulClient: client, consulAgent: client.Agent(), serviceMap: map[string][]*api.AgentService{}, watchMap: map[string]*watch.Plan{}, roundMap: map[string]*uint32{}}
}
