package agent

import (
	"sync/atomic"
	"time"
)

// AgentMetrics Agent 指标收集
type AgentMetrics struct {
	ServiceDiscoveryCount   atomic.Int64
	HealthCheckFailureCount atomic.Int64
	LoadBalancingCount      atomic.Int64
	CacheHitCount           atomic.Int64
	CacheMissCount          atomic.Int64
	LastResetTime           time.Time
}

// NewAgentMetrics 创建新的指标收集器
func NewAgentMetrics() *AgentMetrics {
	return &AgentMetrics{
		LastResetTime: time.Now(),
	}
}

// Reset 重置所有指标
func (m *AgentMetrics) Reset() {
	m.ServiceDiscoveryCount.Store(0)
	m.HealthCheckFailureCount.Store(0)
	m.LoadBalancingCount.Store(0)
	m.CacheHitCount.Store(0)
	m.CacheMissCount.Store(0)
	m.LastResetTime = time.Now()
}

// GetMetrics 获取当前指标
func (m *AgentMetrics) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"service_discovery_count":    m.ServiceDiscoveryCount.Load(),
		"health_check_failure_count": m.HealthCheckFailureCount.Load(),
		"load_balancing_count":       m.LoadBalancingCount.Load(),
		"cache_hit_count":            m.CacheHitCount.Load(),
		"cache_miss_count":           m.CacheMissCount.Load(),
		"last_reset_time":            m.LastResetTime,
		"uptime_seconds":             time.Since(m.LastResetTime).Seconds(),
	}
}

// IncrementServiceDiscovery 增加服务发现计数
func (m *AgentMetrics) IncrementServiceDiscovery() {
	m.ServiceDiscoveryCount.Add(1)
}

// IncrementHealthCheckFailure 增加健康检查失败计数
func (m *AgentMetrics) IncrementHealthCheckFailure() {
	m.HealthCheckFailureCount.Add(1)
}

// IncrementLoadBalancing 增加负载均衡计数
func (m *AgentMetrics) IncrementLoadBalancing() {
	m.LoadBalancingCount.Add(1)
}

// IncrementCacheHit 增加缓存命中计数
func (m *AgentMetrics) IncrementCacheHit() {
	m.CacheHitCount.Add(1)
}

// IncrementCacheMiss 增加缓存未命中计数
func (m *AgentMetrics) IncrementCacheMiss() {
	m.CacheMissCount.Add(1)
}

// GetCacheHitRate 获取缓存命中率
func (m *AgentMetrics) GetCacheHitRate() float64 {
	hits := m.CacheHitCount.Load()
	misses := m.CacheMissCount.Load()
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}
