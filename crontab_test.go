//Lua.go

package luaApi

import (
	"fmt"
	"testing"
	"time"

	"github.com/yuin/gopher-lua"
)

// Time definitions refer to the crontab syntax
//
// -- lua/crontab.lua
// local schedule = {}
//
// table.insert(schedule, {sec='*/10', min='*', hour='*', lvm=1, doFile='lua/test.lua'}) -- Execute every 10s
// --table.insert(schedule, {sec='0/15', min='61', hour='5-7/1', lvm=0, doFile='lua/push4qkq2.lua'}) -- Every day at 5:00,6:00 and 7:00
//
// setJobs(schedule)
//
func TestCrontab(t *testing.T) {
	ct := Crontab{}
	tkr := time.NewTicker(time.Second * 10)
	stop := make(chan struct{})
	go func() {
		defer close(stop)
		time.Sleep(time.Minute + time.Second)
	}()
	for {
		select {
		case <-stop:
			return
		case <-tkr.C:
			if err := ct.Load("lua/crontab.lua", func(L *lua.LState) error {
				//preload the api
				L.Register("sayHello", func(L2 *lua.LState) int {
					p := L2.CheckString(1)
					L2.Push(lua.LString("hello " + p))
					return 1
				})
				return nil
			}); err != nil {
				fmt.Println("locad err:", err)
			}
		}
	}

}
