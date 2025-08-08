# KV 监听功能实现总结

## 概述

本次实现为 consulTool 库添加了完整的 KV 监听功能，包括：

1. **基础 KV 监听功能**
2. **JSON 自动反序列化监听**
3. **YAML 自动反序列化监听（支持环境变量解析）**
4. **示例代码和文档**

## 实现的功能

### 1. Agent 中的 KV 监听方法

在 `agent.go` 中添加了以下方法：

- `WatchKV(key string, watcherFunc func(index uint64, kvPair *api.KVPair)) error`
  - 监听单个 KV 键的变化
  - 支持自定义回调函数处理变化

- `StopWatchKV(key string) error`
  - 停止监听指定 KV 键

- `WatchKVPrefix(prefix string, watcherFunc func(index uint64, pairs api.KVPairs)) error`
  - 监听指定前缀的所有 KV 变化
  - 支持批量处理多个 KV 对

- `StopWatchKVPrefix(prefix string) error`
  - 停止监听指定前缀的 KV

### 2. 默认解析器函数

在 `watch.go` 中添加了两个默认的 KvWatchFunc 返回方法和回调函数类型：

#### `ConfigCallbackFunc` 回调函数类型
- 定义：`type ConfigCallbackFunc func(key string, target interface{})`
- 用于在配置更新后通知系统
- 参数：`key` - KV 键名，`target` - 更新后的配置结构体

#### `NewJSONKvWatchFunc(target interface{}, callback ...ConfigCallbackFunc) KvWatchFunc`
- 创建一个默认的 JSON 反序列化 KvWatchFunc
- 传入一个结构体指针，当 KV 值变化时，会自动将 JSON 值反序列化到该结构体
- 支持标准的 JSON 格式
- 可选传入回调函数，在配置更新后会被调用，用于通知系统配置已更新

#### `NewYamlKvWatchFunc(target interface{}, callback ...ConfigCallbackFunc) KvWatchFunc`
- 创建一个默认的 YAML 反序列化 KvWatchFunc
- 传入一个结构体指针，当 KV 值变化时，会自动将 YAML 值反序列化到该结构体
- 支持环境变量解析：`${ENV_NAME:default_value}`
- 支持类型自动转换（字符串、数字、布尔值）
- 可选传入回调函数，在配置更新后会被调用，用于通知系统配置已更新

### 3. 依赖集成

- 集成了 [dollarYaml](https://github.com/kmlixh/dollarYaml) 依赖
- 支持 YAML 环境变量解析和类型自动转换
- 更新了 `go.mod` 文件

### 4. 错误修复

- 修复了 `WatchGroup` 中的错误停止方法调用
- 更新了 `Close()` 方法以支持停止 KV 监听
- 添加了必要的导入包

## 使用示例

### 基础 KV 监听

```go
// 监听单个 KV 键
if err := agent.WatchKV("app/config", func(index uint64, kvPair *api.KVPair) {
    fmt.Printf("KV changed: %s = %s\n", kvPair.Key, string(kvPair.Value))
}); err != nil {
    return err
}

// 监听 KV 前缀
if err := agent.WatchKVPrefix("app/", func(index uint64, pairs api.KVPairs) {
    fmt.Printf("Found %d KV pairs\n", len(pairs))
    for _, pair := range pairs {
        fmt.Printf("  %s = %s\n", pair.Key, string(pair.Value))
    }
}); err != nil {
    return err
}
```

### 使用 JSON 解析器（带回调函数）

```go
type Config struct {
    Database DatabaseConfig `json:"database"`
    Cache    CacheConfig    `json:"cache"`
}

config := &Config{}

// 定义配置更新回调函数
configCallback := func(key string, target interface{}) {
    if config, ok := target.(*Config); ok {
        fmt.Printf("Config updated for key %s:\n", key)
        fmt.Printf("  Database: %s:%d\n", config.Database.Host, config.Database.Port)
        fmt.Printf("  Cache TTL: %d, Enabled: %v\n", config.Cache.TTL, config.Cache.Enabled)
        
        // 这里可以添加配置更新后的业务逻辑
        // 例如：重新初始化数据库连接、更新缓存配置等
    }
}

jsonWatcher := consulTool.NewJSONKvWatchFunc(config, configCallback)

if err := agent.WatchKV("app/config/json", jsonWatcher); err != nil {
    return err
}

// 当 KV 值变化时，config 结构体会自动更新，并调用回调函数
```

### 使用 YAML 解析器（支持环境变量，带回调函数）

```go
type Config struct {
    Database DatabaseConfig `yaml:"database"`
    Cache    CacheConfig    `yaml:"cache"`
}

config := &Config{}

// 定义配置更新回调函数
configCallback := func(key string, target interface{}) {
    if config, ok := target.(*Config); ok {
        fmt.Printf("Config updated for key %s:\n", key)
        fmt.Printf("  Database: %s:%d\n", config.Database.Host, config.Database.Port)
        fmt.Printf("  Cache TTL: %d, Enabled: %v\n", config.Cache.TTL, config.Cache.Enabled)
        
        // 这里可以添加配置更新后的业务逻辑
        // 例如：重新初始化数据库连接、更新缓存配置等
    }
}

yamlWatcher := consulTool.NewYamlKvWatchFunc(config, configCallback)

if err := agent.WatchKV("app/config/yaml", yamlWatcher); err != nil {
    return err
}

// YAML 支持环境变量解析：
// database:
//   host: ${DB_HOST:localhost}
//   port: ${DB_PORT:5432}
// 当 KV 值变化时，config 结构体会自动更新，并调用回调函数
```

## 文件结构

```
examples/
├── watch_demo.go                    # 服务监听示例
├── kv_watch/
│   └── main.go                      # 基础 KV 监听示例
└── kv_parsers/
    └── main.go                      # JSON/YAML 解析器示例
```

## 版本更新

- 更新了 `README.md` 文档
- 添加了详细的使用示例
- 更新了版本历史到 v1.2.0-ai
- 添加了功能特性说明

## 测试

所有代码已通过编译测试，示例代码可以正常运行。

## 依赖

- `github.com/hashicorp/consul/api` - Consul API 客户端
- `github.com/kmlixh/dollarYaml` - YAML 解析和环境变量支持

## 注意事项

1. YAML 解析器支持环境变量解析，格式为 `${ENV_NAME:default_value}`
2. 解析器会自动处理类型转换（字符串、数字、布尔值）
3. 监听器会在 goroutine 中运行，支持并发处理
4. 支持优雅停止监听和资源清理
5. 回调函数是可选的，如果不传入回调函数，配置仍会自动更新到结构体中
6. 回调函数在配置更新成功后才会被调用，如果解析失败则不会调用回调函数
