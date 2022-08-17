# consulTool
 a consul tool for service register ,service discovery ,kv watch ,service watch write by golang
 
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

