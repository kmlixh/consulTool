# consulTool

一个强大的 Golang Consul 服务发现和注册工具包，提供简单易用的 API 接口。

## 特性

- 服务注册与注销
- 服务发现与监控
- 健康检查管理
- 本地缓存支持
- 负载均衡（轮询）
- 性能指标收集
- 优雅的错误处理
- HTTP 客户端集成

## 安装

```bash
go get github.com/kmlixh/consulTool@v4.6.1
```

## 快速开始

### 服务注册

```go
package main

import (
    "github.com/kmlixh/consulTool"
)

func main() {
    // 创建配置
    config := consulTool.NewConfig(consulTool.WithAddress("http://localhost:8500"))
    
    // 创建服务注册器
    registrant, err := consulTool.NewServiceRegistrantBuilder(config).
        WithName("my-service").
        WithID("my-service-1").
        WithPort(8080).
        WithHealthCheckPath("/health").
        WithInterval("5s").
        Build()
    
    if err != nil {
        panic(err)
    }
    
    // 注册服务
    if err := registrant.RegisterService(); err != nil {
        panic(err)
    }
    
    // 在程序退出时注销服务
    defer registrant.DeRegisterService()
}
```

### 服务发现

```go
package main

import (
    "github.com/kmlixh/consulTool"
    "github.com/kmlixh/consulTool/agent"
)

func main() {
    // 创建配置
    config := consulTool.NewConfig(consulTool.WithAddress("http://localhost:8500"))
    
    // 创建 Agent
    agent := agent.NewAgent(config)
    defer agent.Close()
    
    // 获取服务实例
    services, err := agent.GetService("my-service")
    if err != nil {
        panic(err)
    }
    
    // 监控服务变化
    if err := agent.Watch("my-service"); err != nil {
        panic(err)
    }
}
```

## 配置选项

### Config 配置

```go
config := consulTool.NewConfig(
    consulTool.WithAddress("http://localhost:8500"),  // Consul 地址
    consulTool.WithScheme("http"),                    // 协议（http/https）
    consulTool.WithToken("your-token"),               // 访问令牌
    consulTool.WithDatacenter("dc1"),                 // 数据中心
    consulTool.WithTimeout(5 * time.Second),          // 超时时间
)
```

### 服务注册选项

```go
registrant := consulTool.NewServiceRegistrantBuilder(config).
    WithName("service-name")                          // 服务名称
    WithID("service-id")                              // 服务 ID
    WithPort(8080)                                    // 服务端口
    WithAddress("192.168.1.100")                      // 服务地址
    WithTags([]string{"v1", "prod"})                 // 服务标签
    WithMeta(map[string]string{"version": "1.0"})    // 服务元数据
    WithHealthCheckPath("/health")                    // 健康检查路径
    WithInterval("5s")                                // 检查间隔
    WithTimeout("3s")                                 // 检查超时
    WithDeregisterCriticalServiceAfter("30s")        // 不健康注销时间
```

## HTTP 客户端集成

consulTool 提供了与服务发现集成的 HTTP 客户端：

```go
agent := agent.NewAgent(config)
client := agent.HttpClient()

// 使用服务名称替代具体地址
resp, err := client.Get("http://my-service/api/endpoint")
```

## 性能指标

可以通过 Agent 获取性能指标：

```go
metrics := agent.GetMetrics()
fmt.Printf("Service Discovery Count: %d\n", metrics["service_discovery_count"])
fmt.Printf("Cache Hit Count: %d\n", metrics["cache_hit_count"])
fmt.Printf("Watch Count: %d\n", metrics["watch_count"])
```

## 错误处理

consulTool 定义了一系列标准错误类型：

- `ErrServiceNotFound`: 服务未找到
- `ErrNoHealthyInstances`: 没有健康的服务实例
- `ErrConsulNotAvailable`: Consul 服务不可用
- `ErrEmptyServiceName`: 服务名称为空
- `ErrNothingToRefresh`: 没有需要刷新的服务

## 调试模式

可以启用调试模式查看详细日志：

```go
consulTool.Debug(true)
```

## 最佳实践

1. 始终使用 defer 注销服务
2. 合理设置健康检查参数
3. 使用服务缓存提高性能
4. 监控服务变化及时更新
5. 正确处理错误情况

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

MIT License

## 作者

kmlixh

## 版本历史

- v4.6.1
  - 优化服务发现性能
  - 改进错误处理
  - 增加性能指标收集
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

