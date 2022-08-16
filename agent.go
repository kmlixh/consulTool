package consulTool

import (
	"errors"
	"fmt"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	"net/http"
	"sync/atomic"
)

var debug = false

var MaxRound uint32 = 10000

type Agent struct {
	config     *api.Config
	client     *api.Client
	agent      *api.Agent
	serviceMap map[string][]*api.AgentService
	watchMap   map[string]*watch.Plan
	roundMap   map[string]*uint32
}

func Debug(_debug bool) {
	debug = _debug
}

func (s *Agent) Service(service string) *Service {
	if er := s.initService(service); er != nil {
		panic(er)
	}
	sArrays, _ := s.serviceMap[service]
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
	return &Service{host: sArrays[idx].Address, port: sArrays[idx].Port, HttpClient: http.DefaultClient}
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
	go plan.RunWithClientAndHclog(s.client, nil)
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
	status, services, er := s.agent.AgentHealthServiceByName(service)
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
func NewServiceAgent(config *api.Config) *Agent {
	client, er := api.NewClient(config)
	if er != nil {
		panic(er)
	}
	return &Agent{config: config, client: client, agent: client.Agent(), serviceMap: map[string][]*api.AgentService{}, watchMap: map[string]*watch.Plan{}, roundMap: map[string]*uint32{}}
}
