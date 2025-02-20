package main

import (
	"fmt"
	"net/http"

	"github.com/hashicorp/consul/api"
	"github.com/kmlixh/consulTool"
)

var config *consulTool.Config
var serviceName = "demo"
var serviceId = "1"

func init() {
	config = consulTool.NewConfig()
	config.SetAddress("192.168.110.249:8500")
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
