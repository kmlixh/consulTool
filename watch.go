package consulTool

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	"github.com/kmlixh/dollarYaml"
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

// ConfigCallbackFunc 配置更新回调函数类型
type ConfigCallbackFunc func(key string, target interface{})

// NewJSONKvWatchFunc 创建一个默认的 JSON 反序列化 KvWatchFunc
// 传入一个结构体指针，当 KV 值变化时，会自动将 JSON 值反序列化到该结构体
// 可选传入回调函数，在配置更新后会被调用
func NewJSONKvWatchFunc(target interface{}, callback ...ConfigCallbackFunc) KvWatchFunc {
	return func(index uint64, kvPair *api.KVPair) {
		if kvPair == nil || len(kvPair.Value) == 0 {
			return
		}

		if err := json.Unmarshal(kvPair.Value, target); err != nil {
			fmt.Printf("Failed to unmarshal JSON for key %s: %v\n", kvPair.Key, err)
			return
		}

		fmt.Printf("JSON unmarshaled successfully for key %s at index %d\n", kvPair.Key, index)

		// 调用回调函数通知配置更新
		if len(callback) > 0 && callback[0] != nil {
			callback[0](kvPair.Key, target)
		}
	}
}

// NewYamlKvWatchFunc 创建一个默认的 YAML 反序列化 KvWatchFunc
// 传入一个结构体指针，当 KV 值变化时，会自动将 YAML 值反序列化到该结构体
// 支持环境变量解析和类型自动转换
// 可选传入回调函数，在配置更新后会被调用
func NewYamlKvWatchFunc(target interface{}, callback ...ConfigCallbackFunc) KvWatchFunc {
	return func(index uint64, kvPair *api.KVPair) {
		if kvPair == nil || len(kvPair.Value) == 0 {
			return
		}

		// 创建 dollarYaml 实例
		p := dollarYaml.New()

		// 从字节数组读取 YAML
		if err := p.Read(kvPair.Value); err != nil {
			fmt.Printf("Failed to read YAML for key %s: %v\n", kvPair.Key, err)
			return
		}

		// 反序列化到目标结构体
		if err := p.UnmarshalTo(target); err != nil {
			fmt.Printf("Failed to unmarshal YAML for key %s: %v\n", kvPair.Key, err)
			return
		}

		fmt.Printf("YAML unmarshaled successfully for key %s at index %d\n", kvPair.Key, index)

		// 调用回调函数通知配置更新
		if len(callback) > 0 && callback[0] != nil {
			callback[0](kvPair.Key, target)
		}
	}
}
