package consulTool

import (
	"errors"
	"fmt"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
)

type KvWatchFunc func(index uint64, kvPair *api.KVPair)

type Watch struct {
	config  *api.Config
	client  *api.Client
	agent   *api.Agent
	planMap map[string]*watch.Plan
}

func (c *Watch) WatchKey(key string, watcherFunc KvWatchFunc) error {
	plan, er := watch.Parse(map[string]interface{}{"type": "key", "key": key})
	if er != nil {
		return er
	}
	plan.Handler = func(index uint64, data interface{}) {
		if data != nil {
			watcherFunc(index, data.(*api.KVPair))
		}
	}
	c.planMap[key] = plan
	go plan.RunWithClientAndHclog(c.client, nil)
	return nil
}
func (c *Watch) StopWatch(key string) error {
	plan, ok := c.planMap[key]
	if !ok {
		return errors.New(fmt.Sprintf("key [%s] not found", key))
	}
	plan.Stop()
	return nil
}

func NewKvWatch(config *api.Config) *Watch {
	client, er := api.NewClient(config)
	if er != nil {
		panic(er)
	}
	k := Watch{config: config, client: client, planMap: make(map[string]*watch.Plan)}
	return &k
}
