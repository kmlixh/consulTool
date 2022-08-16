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

