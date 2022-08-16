package main

import (
	"consulTool"
	"fmt"
	"github.com/hashicorp/consul/api"
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
	consulTool.Debug(true)
	watch := consulTool.NewKvWatch(config)
	watch.WatchKey("janyee/auth/prod", func(index uint64, kvPair *api.KVPair) {
		fmt.Printf("\nkey [%s] changed:%s\n", kvPair.Key, string(kvPair.Value))
	})
	agent := consulTool.NewServiceAgent(config)
	agent.Watch("mongo")
	go func() {
		for {
			timeAfterTrigger := time.After(time.Millisecond * 50)
			// will be suspend but we have `timer` so will be not deadlocked
			curTime, _ := <-timeAfterTrigger
			// print current time
			fmt.Println(curTime.Format("2006-01-02 15:04:05"))
			resp, er := agent.Service("mongo").Get("/hello", nil)
			if er != nil {
				fmt.Println(er)
				continue
			}
			bytes, err := io.ReadAll(resp.Body)

			fmt.Println(err, string(bytes))
		}
	}()
	// 清理计时器
	http.ListenAndServe(":8080", nil)
}
