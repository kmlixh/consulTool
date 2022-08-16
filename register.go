package consulTool

import (
	"errors"
	"fmt"
	"github.com/hashicorp/consul/api"
	"net"
	"strings"
)

type ServiceRegistrant struct {
	config                         *api.Config
	client                         *api.Client
	agent                          *api.Agent
	ServiceName                    string
	ServiceId                      string
	Address                        string
	Port                           int
	HealthCheckPath                string
	HttpSchema                     string
	Notes                          string
	Tags                           []string
	Interval                       string
	Timeout                        string
	DeregisterCriticalServiceAfter string
	Weights                        api.Weights
}

func (a ServiceRegistrant) RegisterService() error {
	if a.Address == "" {
		a.Address = getOutIp(a.config.Address)
	}
	// 设置Consul对服务健康检查的参数
	check := api.AgentServiceCheck{
		HTTP:                           fmt.Sprintf("%s://%s:%d/%s", a.HttpSchema, a.Address, a.Port, a.HealthCheckPath), // 健康检查地址
		Interval:                       a.Interval,                                                                       // 健康检查频率
		Timeout:                        a.Timeout,                                                                        // 健康检查超时
		Notes:                          a.Notes,
		DeregisterCriticalServiceAfter: a.DeregisterCriticalServiceAfter, // 健康检查失败30s后 consul自动将注册服务删除
	}
	reg := &api.AgentServiceRegistration{
		Name:    a.ServiceName,
		ID:      a.ServiceId,
		Tags:    a.Tags,
		Port:    a.Port,
		Address: a.Address,
		Check:   &check,
	}

	err := a.agent.ServiceRegister(reg)
	return err
}
func (a ServiceRegistrant) DeRegisterService() error {
	fmt.Println("de register")
	return a.agent.ServiceDeregister(a.ServiceId)
}

func NewServiceRegistrant(config *api.Config, serviceName string, id string, port int, healthCheckPath string) *ServiceRegistrant {
	client, er := api.NewClient(config)
	if er != nil {
		panic(er)
	}
	if serviceName == "" {
		panic(errors.New("service name must not be empty"))
	}
	if id == "" {
		panic(errors.New("service id must not be empty"))
	}
	if port <= 0 {
		panic(errors.New("port must must be positive"))
	}
	if healthCheckPath == "" {
		panic(errors.New("HealthCheckPath must not be empty"))
	}
	a := ServiceRegistrant{config: config, client: client, agent: client.Agent(), HttpSchema: "http", ServiceId: id, ServiceName: serviceName, HealthCheckPath: healthCheckPath, Port: port, Interval: "20s", Timeout: "5s", DeregisterCriticalServiceAfter: "35s", Notes: "a consul service registration"}
	return &a
}

func getOutIp(remote string) string {
	conn, err := net.Dial("udp", remote)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	fmt.Println(localAddr.String())
	ip := strings.Split(localAddr.String(), ":")[0]
	return ip
}
