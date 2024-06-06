package main

import (
	"fmt"
	"github.com/hashicorp/consul/api"
	"github.com/kmlixh/consulTool"
	"net/http"
)

var config *api.Config
var serviceName = "demo"
var serviceId = "1"

func init() {
	config = api.DefaultConfig()
	config.Address = "192.168.110.249:8500"
}
func main() {
	consulTool.Debug(false)
	watch := consulTool.NewWatch(config)
	watch.WatchKv("cors_url", func(index uint64, kvPair *api.KVPair) {
		fmt.Printf("\nkey [%s] changed:%s\n", kvPair.Key, string(kvPair.Value))
	})
	//agent := consulTool.NewAgent(config)

	// 清理计时器
	http.ListenAndServe(":8081", nil)
}
