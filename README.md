# ConsulTool

ConsulTool 是一个用于与 Consul 服务发现和配置系统交互的 Go 库。

## 功能特性

- 服务发现与健康检查
- 服务监控（Watch）
- 负载均衡
- KV 存储操作
- 缓存机制
- 指标收集

## 使用方法

```go
package main

import (
	"fmt"
	"github.com/kmlixh/consulTool"
)

func main() {
	// 创建配置
	config := consulTool.NewConfig().
		WithAddress("localhost:8500").
		WithScheme("http")

	// 创建 Agent
	agent := consulTool.NewAgent(config)
	defer agent.Close()

	// 监控服务
	if err := agent.Watch("my-service"); err != nil {
		fmt.Printf("Failed to watch service: %v\n", err)
		return
	}

	// 获取服务实例
	services, err := agent.GetService("my-service")
	if err != nil {
		fmt.Printf("Failed to get service: %v\n", err)
		return
	}

	for _, service := range services {
		fmt.Printf("Service: %s, Address: %s, Port: %d\n",
			service.Service.Service,
			service.Service.Address,
			service.Service.Port)
	}

	// KV 操作
	if err := agent.PutKV("my-key", []byte("my-value")); err != nil {
		fmt.Printf("Failed to put KV: %v\n", err)
		return
	}

	pair, err := agent.GetKV("my-key")
	if err != nil {
		fmt.Printf("Failed to get KV: %v\n", err)
		return
	}

	fmt.Printf("Key: %s, Value: %s\n", pair.Key, string(pair.Value))
}
```

## 代码结构说明

为了避免导入循环和简化代码结构，所有核心功能已经迁移到根目录下的 agent.go 文件中。

### 迁移计划

- [x] 将 agent 包下的功能迁移到根目录
- [x] 解决导入循环问题
- [x] 统一错误处理
- [x] 优化缓存机制
- [x] 完善指标收集

## 许可证

MIT

## 作者

kmlixh

## 版本历史

- v1.1.0-ai
  - 添加 KV 存储操作支持
  - 优化服务发现性能
  - 改进错误处理机制
  - 增加本地缓存支持
  - 增加性能指标收集
  - 优化 Watch 机制
  - 完善单元测试
  - 增加集成测试
  - 完善文档

components:
- Register
- Watch
- Agent

### Register

Create a new ServiceRegistrant

`consulTool.NewServiceRegistrant(....) `

api.config was the 'github.com/hashicorp/consul/api' api.Config,use to config the consul server data

ServiceRegistrant has two functions:

`
RegisterService() //register service to consul
DeRegisterService() //deregister service
`
### Watch

`
NewWatch(config *api.Config) *Watch //to create a tool for watching

WatchKv  //watch the key/value changed
WatchService //watch one service status
WatchAllServices // watch all service status,only name changed

`

### Agent

*Agent use to choose a service and 'proxy' it

`
NewAgent(config *api.Config) *Agent 


`

## 错误处理

consulTool 定义了一系列标准错误类型：

- `ErrServiceNotFound`: 服务未找到
- `ErrNoHealthyInstances`: 没有健康的服务实例
- `ErrConsulNotAvailable`: Consul 服务不可用
- `ErrEmptyServiceName`: 服务名称为空
- `ErrNothingToRefresh`: 没有需要刷新的服务
- `ErrEmptyKey`: 键名为空
- `ErrKeyNotFound`: 键不存在

## 调试模式

可以启用调试模式查看详细日志：

```go
consulTool.Debug(true)
```

