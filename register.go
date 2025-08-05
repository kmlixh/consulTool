package consulTool

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"errors"

	"github.com/hashicorp/consul/api"
)

// ServiceRegistrant 服务注册器
type ServiceRegistrant struct {
	client  *api.Client
	service *api.AgentServiceRegistration
}

// ServiceRegistrantBuilder 服务注册器构建器
type ServiceRegistrantBuilder struct {
	config                         *Config
	name                           string
	id                             string
	port                           int
	tags                           []string
	meta                           map[string]string
	address                        string
	healthCheckPath                string
	httpSchema                     string
	interval                       string
	timeout                        string
	notes                          string
	deregisterCriticalServiceAfter string
}

// GetOutIp 获取本地IP地址
func GetOutIp(remote string) string {
	// 处理URL格式的地址
	if strings.HasPrefix(remote, "http://") || strings.HasPrefix(remote, "https://") {
		u, err := url.Parse(remote)
		if err != nil {
			fmt.Printf("Failed to parse URL %s: %v\n", remote, err)
			return ""
		}
		remote = u.Host
	}

	// 如果地址中包含端口，直接使用
	if strings.Contains(remote, ":") {
		conn, err := net.Dial("udp", remote)
		if err != nil {
			fmt.Printf("Failed to dial UDP %s: %v\n", remote, err)
			return ""
		}
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		return localAddr.IP.String()
	}

	// 如果地址不包含端口，添加一个临时端口
	conn, err := net.Dial("udp", remote+":53")
	if err != nil {
		fmt.Printf("Failed to dial UDP %s:53: %v\n", remote, err)
		return ""
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// NewServiceRegistrantBuilder 创建服务注册器构建器
func NewServiceRegistrantBuilder(config *Config) *ServiceRegistrantBuilder {
	if config == nil {
		panic("config is required")
	}
	return &ServiceRegistrantBuilder{
		config:                         config,
		httpSchema:                     "http",
		interval:                       "10s",
		timeout:                        "5s",
		deregisterCriticalServiceAfter: "30s",
	}
}

// WithName 设置服务名称
func (b *ServiceRegistrantBuilder) WithName(name string) *ServiceRegistrantBuilder {
	b.name = name
	return b
}

// WithID 设置服务ID
func (b *ServiceRegistrantBuilder) WithID(id string) *ServiceRegistrantBuilder {
	b.id = id
	return b
}

// WithPort 设置服务端口
func (b *ServiceRegistrantBuilder) WithPort(port int) *ServiceRegistrantBuilder {
	b.port = port
	return b
}

// WithTags 设置服务标签
func (b *ServiceRegistrantBuilder) WithTags(tags []string) *ServiceRegistrantBuilder {
	b.tags = tags
	return b
}

// WithMeta 设置服务元数据
func (b *ServiceRegistrantBuilder) WithMeta(meta map[string]string) *ServiceRegistrantBuilder {
	b.meta = meta
	return b
}

// WithAddress 设置服务地址
func (b *ServiceRegistrantBuilder) WithAddress(address string) *ServiceRegistrantBuilder {
	b.address = address
	return b
}

// WithHealthCheckPath 设置健康检查路径
func (b *ServiceRegistrantBuilder) WithHealthCheckPath(path string) *ServiceRegistrantBuilder {
	b.healthCheckPath = path
	return b
}

// WithHttpSchema 设置HTTP协议（http/https）
func (b *ServiceRegistrantBuilder) WithHttpSchema(schema string) *ServiceRegistrantBuilder {
	b.httpSchema = schema
	return b
}

// WithInterval 设置健康检查间隔
func (b *ServiceRegistrantBuilder) WithInterval(interval string) *ServiceRegistrantBuilder {
	b.interval = interval
	return b
}

// WithTimeout 设置健康检查超时
func (b *ServiceRegistrantBuilder) WithTimeout(timeout string) *ServiceRegistrantBuilder {
	b.timeout = timeout
	return b
}

// WithNotes 设置健康检查备注
func (b *ServiceRegistrantBuilder) WithNotes(notes string) *ServiceRegistrantBuilder {
	b.notes = notes
	return b
}

// WithDeregisterCriticalServiceAfter 设置不健康注销时间
func (b *ServiceRegistrantBuilder) WithDeregisterCriticalServiceAfter(duration string) *ServiceRegistrantBuilder {
	b.deregisterCriticalServiceAfter = duration
	return b
}

// Build 构建服务注册器
func (b *ServiceRegistrantBuilder) Build() (*ServiceRegistrant, error) {
	if b.config == nil {
		return nil, errors.New("config is required")
	}

	address := b.config.GetAddress()
	if address == "" {
		return nil, errors.New("consul address is required")
	}

	if b.name == "" {
		return nil, errors.New("service name is required")
	}
	if b.id == "" {
		return nil, errors.New("service id is required")
	}
	if b.port <= 0 {
		return nil, errors.New("service port must be greater than 0")
	}

	// 创建Consul客户端
	client, err := api.NewClient(b.config.getConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %v", err)
	}

	// 如果没有设置地址，使用GetOutIp获取本地IP
	if b.address == "" {
		b.address = GetOutIp(address)
		if b.address == "" {
			return nil, errors.New("failed to get local IP address")
		}
	}

	registration := &api.AgentServiceRegistration{
		ID:      b.id,
		Name:    b.name,
		Port:    b.port,
		Address: b.address,
		Tags:    b.tags,
		Meta:    b.meta,
	}

	// 如果设置了健康检查路径，添加健康检查配置
	if b.healthCheckPath != "" {
		schema := b.httpSchema
		if schema == "" {
			schema = "http"
		}

		interval := b.interval
		if interval == "" {
			interval = "10s"
		}

		timeout := b.timeout
		if timeout == "" {
			timeout = "5s"
		}

		deregister := b.deregisterCriticalServiceAfter
		if deregister == "" {
			deregister = "30s"
		}

		registration.Check = &api.AgentServiceCheck{
			HTTP:                           fmt.Sprintf("%s://%s:%d%s", schema, b.address, b.port, b.healthCheckPath),
			Interval:                       interval,
			Timeout:                        timeout,
			DeregisterCriticalServiceAfter: deregister,
			TLSSkipVerify:                  true,
			Notes:                          b.notes,
		}
	}

	return &ServiceRegistrant{
		client:  client,
		service: registration,
	}, nil
}

// RegisterService 注册服务
func (r *ServiceRegistrant) RegisterService() error {
	if err := r.client.Agent().ServiceRegister(r.service); err != nil {
		return fmt.Errorf("failed to register service %s: %v", r.service.Name, err)
	}
	fmt.Printf("Service %s registered successfully with ID %s on port %d at address %s\n",
		r.service.Name, r.service.ID, r.service.Port, r.service.Address)
	return nil
}

// DeRegisterService 注销服务
func (r *ServiceRegistrant) DeRegisterService() error {
	if err := r.client.Agent().ServiceDeregister(r.service.ID); err != nil {
		return fmt.Errorf("failed to deregister service %s: %v", r.service.ID, err)
	}
	fmt.Printf("Service %s deregistered successfully\n", r.service.ID)
	return nil
}
