package main

import (
	"fmt"
	"github.com/hashicorp/consul/api"
	"github.com/kmlixh/consulTool"
	"io"
	"net/http"
	"time"
)

var config *api.Config
var serviceName = "demo"
var serviceId = "1"

func init() {
	config = api.DefaultConfig()
	config.Address = "10.0.1.5:8500"
}
func main() {
	consulTool.Debug(false)
	watch := consulTool.NewWatch(config)
	watch.WatchKv("janyee/auth/prod", func(index uint64, kvPair *api.KVPair) {
		fmt.Printf("\nkey [%s] changed:%s\n", kvPair.Key, string(kvPair.Value))
	})
	agent := consulTool.NewAgent(config)

	agent.Watch("mongo")
	go func() {
		for {
			timeAfterTrigger := time.After(time.Millisecond * 699)
			// will be suspend but we have `timer` so will be not deadlocked
			<-timeAfterTrigger
			// print current time
			resp, er := agent.HttpClient().Get("http://mongo/hello")
			//resp, er := agent.HttpClient().Get("https://fochan.org/admin/xxx")
			if er != nil {
				fmt.Println(er)
				continue
			}
			bytes, err := io.ReadAll(resp.Body)

			fmt.Println(err, string(bytes))
		}
	}()
	// 清理计时器
	http.ListenAndServe(":8081", nil)
}
