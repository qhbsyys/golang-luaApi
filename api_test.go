//

package luaApi

import (
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"
)

func TestRegisterUserData(t *testing.T) {
	type ScorePair struct {
		Score  int64
		Member string
	}

	L := lua.NewState(lua.Options{IncludeGoStackTrace: true})
	defer L.Close()

	RegisterUserData(L, ScorePair{})

	if err := L.DoString(`
    s = scorepair.new(100, "mem")
    assert(type(s)=='userdata')

    assert(s:getScore()==100)
    assert(s:getMember()=='mem')

    s:setScore(120)
    assert(s:getScore()==120)

    --print(s) --will print the str defined by 'json.Marsh(&ScorePair{120, "mem"})'
    `); err != nil {
		t.Fatal(err)
	}
}

type xt struct {
	Double func(int) int
	StrCat func(s ...string) string
}

func (x xt) Globals() map[string]string {
	return map[string]string{
		"flag": "globelsFlag",
	}
}
func (x xt) Funcs() map[string]lua.LGFunction {
	fucs := make(map[string]lua.LGFunction)
	ParseStruct(x, fucs)

	return fucs
}

func TestFuncModule(t *testing.T) {
	L := lua.NewState(lua.Options{IncludeGoStackTrace: true})
	defer L.Close()

	x := &xt{
		Double: func(n int) int {
			return n * n
		},
		StrCat: func(s ...string) string {
			return strings.Join(s, "")
		},
	}
	L.PreloadModule("xt", FuncModule(x))

	if err := L.DoString(`
    xt = require("xt")
    assert(type(xt)=='table')
    assert(type(xt.double)=='function')
    assert(type(xt.strcat)=='function')

    assert(type(xt.flag)=='string')
    assert(xt.flag=='globelsFlag')

    assert(xt.double(4)==16)

    --test variable parameters
    assert(xt.strcat('abc', '123')=='abc123')

    ss = {}
    table.insert(ss, 'abc')
    table.insert(ss, '123')
    assert(xt.strcat(ss)=='abc123')

    `); err != nil {
		t.Fatal(err)
	}
}
