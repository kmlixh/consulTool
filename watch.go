package consulTool

import (
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
)

type KvWatchFunc func(index uint64, kvPair *api.KVPair)
type ServiceWatchFunc func(index uint64, services []*api.ServiceEntry)

type Watch struct {
	config   *Config
	client   *api.Client
	agent    *api.Agent
	watchMap map[string]*watch.Plan
}

func (c *Watch) WatchKv(key string, watcherFunc KvWatchFunc) error {
	plan, er := watch.Parse(map[string]interface{}{"type": "key", "key": key})
	if er != nil {
		return er
	}
	plan.Handler = func(index uint64, data interface{}) {
		if data != nil {
			watcherFunc(index, data.(*api.KVPair))
		}
	}
	c.watchMap["key_"+key] = plan
	go plan.RunWithClientAndHclog(c.client, nil)
	return nil
}
func (c *Watch) WatchService(name string, watchFunc ServiceWatchFunc) error {
	plan, er := watch.Parse(map[string]interface{}{"type": "service", "service": name})
	if er != nil {
		return er
	}
	plan.Handler = func(index uint64, data interface{}) {
		if data != nil {
			watchFunc(index, data.([]*api.ServiceEntry))
		}
	}
	c.watchMap["service_"+name] = plan
	go plan.RunWithClientAndHclog(c.client, nil)
	return nil
}
func (c *Watch) WatchAllServices(f func(index uint64, data map[string][]string)) error {
	plan, er := watch.Parse(map[string]interface{}{"type": "services"})
	if er != nil {
		return er
	}
	plan.Handler = func(index uint64, data interface{}) {
		if data != nil {
			f(index, data.(map[string][]string))
		}
	}
	c.watchMap["services_all"] = plan
	go plan.RunWithClientAndHclog(c.client, nil)
	return nil
}

func (c *Watch) StopWatchKv(key string) error {
	plan, ok := c.watchMap["key_"+key]
	if !ok {
		return nil
	}
	plan.Stop()
	return nil
}
func (c *Watch) StopWatchService(serviceName string) error {
	plan, ok := c.watchMap["service_"+serviceName]
	if !ok {
		return nil
	}
	plan.Stop()
	return nil
}
func (c *Watch) StopWatchAllServices() error {
	plan, ok := c.watchMap["services_all"]
	if !ok {
		return nil
	}
	plan.Stop()
	return nil
}
func NewWatch(config *Config) *Watch {
	client, er := api.NewClient(config.getConfig())
	if er != nil {
		panic(er)
	}
	k := Watch{config: config, client: client, watchMap: make(map[string]*watch.Plan)}
	return &k
}
