// build ignore
// 实现在lua脚本中打印log到文件，方便调试.
// 未完成
package base

import (
	"log"
	"strings"

	"github.com/yuin/gopher-lua"
)

type Logger interface {
	Debug(fmt string, v ...interface{})
	Trace(fmt string, v ...interface{})
	Info(fmt string, v ...interface{})
	Warn(fmt string, v ...interface{})
	Error(fmt string, v ...interface{})
}

var logger Logger

func init() {
	logger = newL()
}

func SetLogger(l Logger) {
	logger = l
}

func LoadLogger(L0 *lua.LState) {

	fucs := make(map[string]lua.LGFunction)
	ParseStruct(logger, fucs)
	L0.PreloadModule("log", func(L *lua.LState) int {
		t := L.NewTable()
		L.SetFuncs(t, fucs)
		mt := L.NewTable()
		//Ignore the case of the function's name
		L.SetField(mt, "__index", L.NewFunction(func(L2 *lua.LState) int {
			t := L2.CheckTable(1)
			key := L2.CheckString(2)
			if f := t.RawGetString(strings.ToLower(key)); f.Type() == lua.LTFunction {
				L2.Push(f)
				return 1
			} else {
				L2.ArgError(2, "unknown func "+key)
				return 0
			}
		}))
		//Forbidden to add anything
		L.SetField(mt, "__newindex", L.NewFunction(func(L2 *lua.LState) int {
			L2.ArgError(2, "You are not allowed to add field.")
			return 0
		}))
		L.SetMetatable(t, mt)
		L.Push(t)
		return 1
	})
}

//===========
type L struct{}

func newL() *L {
	log.SetFlags(log.Ldate | log.Ltime)
	return &L{}
}
func (l *L) Debug(fmt string, v ...interface{}) { log.Printf(fmt, v...) }
func (l *L) Trace(fmt string, v ...interface{}) { log.Printf(fmt, v...) }
func (l *L) Info(fmt string, v ...interface{})  { log.Printf(fmt, v...) }
func (l *L) Warn(fmt string, v ...interface{})  { log.Printf(fmt, v...) }
func (l *L) Error(fmt string, v ...interface{}) { log.Printf(fmt, v...) }
