//Lua.go

//Defines how the lua's virtual machine works
package base

import (
	"fmt"
	"sync"

	"github.com/yuin/gopher-lua"
)

const (
	MAX_LVM_NUM = 16
)

var (
	defaultLVMs = NewVMManager()
)

type PreloadFunc func(L *lua.LState) error

func exec(L *lua.LState, script string, isFile bool, preload PreloadFunc) error {
	if preload != nil {
		if err := preload(L); err != nil {
			return err
		}
	}
	if isFile {
		return L.DoFile(script)
	} else {
		return L.DoString(script)
	}
}

func DoScriptOnce(script string, preload PreloadFunc) error {
	L := lua.NewState(lua.Options{IncludeGoStackTrace: true})
	defer L.Close()
	return exec(L, script, false, preload)
}
func DoFileOnce(filepath string, preload PreloadFunc) error {
	L := lua.NewState(lua.Options{IncludeGoStackTrace: true})
	defer L.Close()
	return exec(L, filepath, true, preload)
}

//preload can be nil
func DoScriptInLuaVM(id int, script string, preload PreloadFunc) error {
	return defaultLVMs.DoScriptInLuaVM(id, script, preload)
}

func DoFileInLuaVM(id int, filepath string, preload PreloadFunc) error {
	return defaultLVMs.DoFileInLuaVM(id, filepath, preload)
}

//--------------------------------------------
type VMManage struct {
	vms map[int]*lua.LState
	wlk sync.Mutex
}

func NewVMManager() *VMManage {
	return &VMManage{vms: make(map[int]*lua.LState)}
}

func (m *VMManage) getLVM(id int) (*lua.LState, error) {
	m.wlk.Lock()
	defer m.wlk.Unlock()

	vm, ok := m.vms[id]
	if !ok {
		if len(m.vms) > MAX_LVM_NUM {
			return nil, fmt.Errorf("Too many virtual machines. limited=%d", MAX_LVM_NUM)
		}
		vm = lua.NewState(lua.Options{IncludeGoStackTrace: true})
		m.vms[id] = vm
	}
	return vm, nil
}

func (m *VMManage) RemoveLVM(id int) {
	m.wlk.Lock()
	defer m.wlk.Unlock()

	if L, ok := m.vms[id]; !ok {
		L.Close()
		delete(m.vms, id)
	}
}

func (m *VMManage) DoScriptInLuaVM(id int, script string, preload PreloadFunc) error {
	L, err := m.getLVM(id)
	if err != nil {
		return err
	}

	return exec(L, script, false, preload)
}

func (m *VMManage) DoFileInLuaVM(id int, filepath string, preload PreloadFunc) error {
	L, err := m.getLVM(id)
	if err != nil {
		return err
	}

	return exec(L, filepath, true, preload)
}
