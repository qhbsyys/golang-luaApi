//

//
package base

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/yuin/gluamapper"
	"github.com/yuin/gopher-lua"
)

var (
	errorInterface = reflect.TypeOf((*error)(nil)).Elem()
)

type LuaModuler interface {
	Globals() map[string]string
	Funcs() map[string]lua.LGFunction
}

func FuncModule(api LuaModuler) lua.LGFunction {
	return func(L *lua.LState) int {
		t := L.NewTable()
		for k, v := range api.Globals() {
			t.RawSetString(k, lua.LString(v))
		}
		L.SetFuncs(t, api.Funcs())
		mt := L.NewTable()
		//Ignore the case of the function name
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
	}
}

func ParseStruct(s interface{}, funcs map[string]lua.LGFunction) {
	tpApi := reflect.TypeOf(s)
	numField := 0
	if tpApi.Kind() == reflect.Struct {
		numField = tpApi.NumField()
	}
	if tpApi.Kind() == reflect.Ptr {
		for i := 0; i < tpApi.NumMethod(); i++ {
			f := tpApi.Method(i)
			v := reflect.ValueOf(s).MethodByName(f.Name)
			if v.Kind() != reflect.Invalid {
				funcs[strings.ToLower(f.Name)] = call(v, v.Type())
			}
		}
	}
	for i := 0; i < numField; i++ {
		f := tpApi.Field(i)
		v := reflect.ValueOf(s).FieldByName(f.Name)
		switch f.Type.Kind() {
		case reflect.Ptr:
			if v.Elem().Kind() == reflect.Struct {
				v = v.Elem()
				ParseStruct(v.Interface(), funcs)
			}
		case reflect.Struct:
			ParseStruct(v.Interface(), funcs)
		case reflect.Func:
			funcs[strings.ToLower(f.Name)] = call(v, f.Type)
		case reflect.String:
			//
		case reflect.Interface:
			// KindOf v.Interface() is Ptr, will parse by range Methods
			ParseStruct(v.Interface(), funcs)
		default:
			//showField(f.Type.Name(), v)
			logger.Warn("\t+++%v, name=%v", f.Type.Kind(), f.Type.Name())
		}
	}
}

func RegisterUserData(L *lua.LState, demo interface{}) {
	tpName := lowerTypeName(demo)
	if L.GetTypeMetatable(tpName) != lua.LNil {
		return //had Register
	}

	if reflect.TypeOf(demo).Kind() == reflect.Ptr {
		demo = reflect.ValueOf(demo).Elem().Interface()
	}
	logger.Debug("RegisterUserData:%s", tpName)
	tps := reflect.TypeOf(demo)
	mt := L.NewTypeMetatable(tpName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), initGetterSetter(tps)))
	L.SetField(mt, "__tostring", L.NewFunction(func(L2 *lua.LState) int {
		ud := L.CheckUserData(1)
		L2.Push(lua.LString(tipsString(ud.Value)))
		return 1
	}))
	L.SetField(mt, "new", L.NewFunction(func(L2 *lua.LState) int {
		if L2.GetTop() > tps.NumField() {
			L2.ArgError(L2.GetTop(),
				fmt.Sprintf("%s,%v", tipsUsage(tps), L2.CheckAny(L2.GetTop())),
			)
			return 0
		}
		//pack
		rv := reflect.New(tps)
		for i := 0; i < L2.GetTop(); i++ {
			if fv, err := ParseLValue(L2, L2.CheckAny(i+1), tps.Field(i).Type); err != nil {
				L2.ArgError(i+1, err.Error()+tipsUsage(tps) /*err.Error()*/)
			} else {
				reflect.Indirect(rv).Field(i).Set(fv)
			}
		}
		L2.Push(createUserData(L2, rv.Interface(), tpName))
		return 1
	}))
	L.SetGlobal(tpName, mt)
}

//==================================
func tipsString(v interface{}) string {
	if bt, err := json.Marshal(v); err != nil {
		return fmt.Sprintf("%#v", v)
	} else {
		return fmt.Sprintf("%s", bt)
	}
}

func tipsUsage(tps reflect.Type) string {
	tips := " Usage: .new("
	for i := 0; i < tps.NumField(); i++ {
		tips = tips + tps.Field(i).Type.Kind().String() + ","
	}
	return tips[:len(tips)-1] + ") or .new()"
}

func initGetterSetter(tps reflect.Type) map[string]lua.LGFunction {
	fucs := map[string]lua.LGFunction{}
	for i := 0; i < tps.NumField(); i++ {
		field := tps.Field(i)
		fName := field.Name
		if len(fName) == 0 || fName[0] < 'A' || fName[0] > 'Z' {
			continue //ignore private fields
		}
		fucs["get"+fName] = func(L *lua.LState) int {
			ptr := L.CheckUserData(1).Value
			value := reflect.Indirect(reflect.ValueOf(ptr)).FieldByName(fName)
			PushLValue(L, value)
			return 1
		}
		fucs["set"+fName] = func(L *lua.LState) int {
			if L.GetTop() != 2 {
				L.ArgError(2, fmt.Sprintf("A param was needed to call. 'fvpair:set%s()'", field.Name))
			}
			ptr := L.CheckUserData(1).Value
			value := reflect.Indirect(reflect.ValueOf(ptr)).FieldByName(fName)
			tmpV, _ := ParseLValue(L, L.CheckAny(2), field.Type)
			value.Set(tmpV)
			return 0
		}
	}

	return fucs
}

//==================================
func call(f reflect.Value, fType reflect.Type) func(*lua.LState) int {
	return func(L *lua.LState) int {
		inputs, err := CheckGetInputs(L, fType)
		if err != nil {
			if fType.NumOut() == 0 {
				L.ArgError(1, err.Error())
				return 0
			}
			return SetErrorOutputs(L, fType.NumOut(), err)
		}
		var rets []reflect.Value
		if !fType.IsVariadic() || len(inputs) < fType.NumIn() {
			rets = f.Call(inputs)
		} else {
			rets = f.CallSlice(inputs)
		}
		return CheckSetOutputs(L, fType, rets)
	}
}

//assert f.Kind() == Func
func CheckGetInputs(L *lua.LState, f reflect.Type) ([]reflect.Value, error) {
	var (
		numIn        = L.GetTop()
		atLeastNumIn = f.NumIn()
		i            = 0
	)
	if f.IsVariadic() {
		atLeastNumIn-- //The last parameter would be resolved later
	}
	if numIn < atLeastNumIn {
		return []reflect.Value{}, fmt.Errorf("Invalid input arguments. Need %d inputs at least.", atLeastNumIn)
	}

	inputs := make([]reflect.Value, atLeastNumIn, f.NumIn())
	for ; i < numIn && i < atLeastNumIn; i++ {
		if argv, err := ParseLValue(L, L.CheckAny(i+1), f.In(i)); err != nil {
			return []reflect.Value{}, err
		} else {
			inputs[i] = argv
		}
	}
	//check Variadic
	if f.IsVariadic() && numIn > atLeastNumIn {
		argv, err := ParseLValue(L, L.CheckAny(i+1), f.In(i).Elem(), f.In(i))
		if err != nil {
			return []reflect.Value{}, err
		}
		if argv.IsValid() && argv.Type() == f.In(i) {
			inputs = append(inputs, argv)
		} else {
			n := numIn - atLeastNumIn
			last := reflect.MakeSlice(f.In(i), n, n)
			if argv.IsValid() {
				last.Index(0).Set(argv)
			}
			for j := 1; j < n; j++ {
				argv2, err := ParseLValue(L, L.CheckAny(i+j+1), f.In(i).Elem())
				if err != nil {
					return []reflect.Value{}, err
				}
				if argv2.IsValid() {
					last.Index(j).Set(argv2)
				}
			}
			inputs = append(inputs, last)
		}
	}
	return inputs, nil
}

func CheckSetOutputs(L *lua.LState, f reflect.Type, vs []reflect.Value) int {
	numOut := f.NumOut()

	if len(vs) != numOut {
		return SetErrorOutputs(L, numOut, fmt.Errorf("invalid outputs."))
	}
	i := 0
	for ; i < numOut; i++ {
		PushLValue(L, vs[i])
	}
	return numOut
}

func SetErrorOutputs(L *lua.LState, numOut int, err error) int {
	i := 0
	for ; i < numOut-1; i++ {
		L.Push(lua.LNil)
	}
	L.Push(lua.LString(err.Error()))
	return numOut
}

//------------------------------
func lowerTypeName(v interface{}) string {
	if v == nil {
		return ""
	}
	return strings.ToLower(reflect.TypeOf(v).Name())
}

//lua type --> golang type
// expect==reflect.Interface will ignore type detection
// ret.Type()!=Slice expects[0]==ret.Type(); ret.Type()==Slice
//     expects[0]=ret.Type().Elem() expects[1]=ret.Type()
func ParseLValue(L *lua.LState, v lua.LValue, expects ...reflect.Type) (ret reflect.Value, err error) {
	ret = lua2GoValue(L, v)
	if !ret.IsValid() {
		if expects[0].Kind() == reflect.Interface {
			return
		}
		err = fmt.Errorf("invalid value type, expect %v, got nil.", expects)
		return
	}

	tp := ret.Type()
	expKd := expects[0].Kind()

	match := true
	switch tp.Kind() {
	case reflect.Int64:
		n := reflect.Indirect(ret).Interface().(int64)
		switch expKd {
		case reflect.Int:
			ret = reflect.ValueOf(int(n))
		case reflect.Int8:
			ret = reflect.ValueOf(int8(n))
		case reflect.Int16:
			ret = reflect.ValueOf(int16(n))
		case reflect.Int32:
			ret = reflect.ValueOf(int32(n))
		case reflect.Int64:
		case reflect.Uint:
			ret = reflect.ValueOf(uint(n))
		case reflect.Uint8:
			ret = reflect.ValueOf(uint8(n))
		case reflect.Uint16:
			ret = reflect.ValueOf(uint16(n))
		case reflect.Uint32:
			ret = reflect.ValueOf(uint32(n))
		case reflect.Uint64:
			ret = reflect.ValueOf(uint64(n))
		}
	case reflect.Float64:
		switch expKd {
		case reflect.Float32:
			n := reflect.Indirect(ret).Interface().(float64)
			ret = reflect.ValueOf(float32(n))
		case reflect.Float64:
		}
	case reflect.Ptr:
		ret = ret.Elem()
		tp = ret.Type()
	default:
		//reflect.Bool .String .Slice .Array .Map
	}
	//
	if match = (ret.Type() == expects[0]); !match {
		i := 1
		for ; i < len(expects) && ret.Type() != expects[i]; i++ {
		}
		match = i < len(expects)
	}

	if !match && reflect.Interface != expKd {
		err = fmt.Errorf("invalid value type, expect %v, got %s.", expects[0], tp)
	}
	return
}

func lua2GoValue(L *lua.LState, v lua.LValue) (ret reflect.Value) {
	switch v.Type() {
	case lua.LTNil:
		ret = reflect.ValueOf(nil)
	case lua.LTBool:
		ret = reflect.ValueOf(lua.LVAsBool(v))
	case lua.LTNumber:
		n := lua.LVAsNumber(v)
		if float64(n) == float64(int64(n)) {
			ret = reflect.ValueOf(int64(n))
		} else {
			ret = reflect.ValueOf(float64(n))
		}
	case lua.LTString:
		ret = reflect.ValueOf(lua.LVAsString(v))
	case lua.LTTable:
		tmp := v.(*lua.LTable)
		if tmp.Len() > 0 {
			//to Slice
			first := reflect.Indirect(lua2GoValue(L, tmp.RawGetInt(1)))
			tp := first.Type()
			gv := reflect.MakeSlice(reflect.SliceOf(tp), tmp.Len(), tmp.Len())
			gv.Index(0).Set(first)
			for i := 1; i < tmp.Len(); i++ {
				others := reflect.Indirect(lua2GoValue(L, tmp.RawGetInt(i+1)))
				gv.Index(i).Set(others)
			}
			ret = gv
		} else {
			gv := make(map[interface{}]interface{})
			gluamapper.Map(v.(*lua.LTable), &gv)
			ret = reflect.ValueOf(gv)
		}
	case lua.LTUserData:
		lv, _ := v.(*lua.LUserData)
		tp := reflect.TypeOf(reflect.Indirect(reflect.ValueOf(lv.Value)).Interface())
		rv := reflect.New(tp)
		reflect.Indirect(rv).Set(reflect.Indirect(reflect.ValueOf(lv.Value)))
		ret = rv // = reflect.ValueOf(lv.Value).Elem()
	//case lua.LTFunction, lua.LTThread, lua.LTChannel:
	default:
		L.ArgError(1, "Unsupport type:"+v.Type().String())
	}
	return
}

/**
func lua2GoValue(L *lua.LState, v lua.LValue) (ret reflect.Value) {
	switch lv := v.(type) {
	//case lua.LNil,
	case lua.LBool:
		ret = reflect.ValueOf(bool(lv))
	case lua.LString:
		ret = reflect.ValueOf(string(lv))
	case lua.LNumber:
		if float64(lv) == float64(int64(lv)) {
			ret = reflect.ValueOf(int64(lv))
		} else {
			ret = reflect.ValueOf(lv)
		}
	case *lua.LTable:
		var arr []reflect.Value
		var obj map[string]reflect.Value

		lv.ForEach(func(tk, tv lua.LValue) {
			i, isNum := tk.(lua.LNumber)
			// try to convert to Slice firstly
			if isNum && obj == nil {
				index := int(i) - 1
				if index == len(arr) {
					arr = append(arr, lua2GoValue(L, tv))
					return
				}
				// map out of order; convert to map
			}
			if obj == nil {
				obj = make(map[string]reflect.Value)
				for i, value := range arr {
					obj[strconv.Itoa(i+1)] = value
				}
			}
			obj[tk.String()] = lua2GoValue(L, tv)
		})
		if obj != nil {
			//is a map
			ret = reflect.ValueOf(obj)
		} else {
			ret = reflect.ValueOf(arr)
		}
		//TODO 这种写法丢失了类型 如何改进?
		//invalid value type, expect pub.FVPair, got []reflect.Value

	case *lua.LUserData:
		tp := reflect.TypeOf(reflect.Indirect(reflect.ValueOf(lv.Value)).Interface())
		rv := reflect.New(tp)
		reflect.Indirect(rv).Set(reflect.ValueOf(lv.Value).Elem())
		//fmt.Printf("lua2GoValue:%#v\n", rv)
		ret = rv // = reflect.ValueOf(lv.Value).Elem()

	//case lua.LTFunction, lua.LTThread, lua.LTChannel:
	default:
		L.ArgError(1, "Unsupport type:"+v.Type().String())
	}
	return
}
*/
// golang type --> lua type
func PushLValue(L *lua.LState, vs ...reflect.Value) int {
	for _, v := range vs {
		L.Push(go2LuaValue(L, v))
	}
	return len(vs)
}

func go2LuaValue(L *lua.LState, rv reflect.Value) lua.LValue {
	if !rv.IsValid() {
		return lua.LNil
	}
	v := rv.Interface() //reflect.Indirect(rv)
	if v == nil {
		return lua.LNil
	}
	switch rv.Kind() {
	case reflect.Bool:
		return lua.LBool(v.(bool))
	case reflect.Int:
		return lua.LNumber(v.(int))
	case reflect.Int8:
		return lua.LNumber(v.(int8))
	case reflect.Int16:
		return lua.LNumber(v.(int16))
	case reflect.Int32:
		return lua.LNumber(v.(int32))
	case reflect.Int64:
		return lua.LNumber(v.(int64))
	case reflect.Uint:
		return lua.LNumber(v.(uint))
	case reflect.Uint8:
		return lua.LNumber(v.(uint8))
	case reflect.Uint16:
		return lua.LNumber(v.(uint16))
	case reflect.Uint32:
		return lua.LNumber(v.(uint32))
	case reflect.Uint64:
		return lua.LNumber(v.(uint64))
	case reflect.Float32:
		return lua.LNumber(v.(float32))
	case reflect.Float64:
		return lua.LNumber(v.(float64))
	case reflect.String:
		return lua.LString(v.(string))
	case reflect.Ptr:
		//fmt.Printf("go2LuaValue ptr-->%v\n", v)
		return go2LuaValue(L, rv.Elem())
	case reflect.Struct:
		//第二个参数用 &v 会导致类型丢失，导致调用get set报错
		//call of reflect.Value.FieldByName on interface Value
		//fmt.Printf("go2LuaValue Struct-->%#v\n", v)
		if L.GetTypeMetatable(lowerTypeName(v)) != nil {
			return createUserData(L, rv.Interface(), lowerTypeName(v))
		} else {
			lt := L.CreateTable(0, rv.NumField())
			tpv := rv.Type()
			for i := 0; i < rv.NumField(); i++ {
				fn := tpv.Field(i).Name
				//ignore private field
				if len(fn) == 0 || fn[0] < 'A' || fn[0] > 'Z' {
					continue
				}
				lt.RawSetString(fn, go2LuaValue(L, rv.Field(i)))
			}
			return lt
		}
	case reflect.Slice, reflect.Array:
		lt := L.CreateTable(rv.Len(), 0)
		for i := 0; i < rv.Len(); i++ {
			lt.RawSetInt(i+1, go2LuaValue(L, rv.Index(i)))
		}
		return lt
	case reflect.Map:
		lt := L.CreateTable(0, rv.Len())
		for _, k := range rv.MapKeys() {
			lt.RawSet(go2LuaValue(L, k), go2LuaValue(L, rv.MapIndex(k)))
		}
		return lt
	case reflect.Interface:
		if rv.Type() == errorInterface {
			return lua.LString(reflect.Indirect(rv).Interface().(error).Error())
		} else {
			return go2LuaValue(L, reflect.ValueOf(rv.Interface()))
		}
	default:
		logger.Warn("default Go2LuaValue kind='%v', '%s', value='%v'", rv.Kind(), lowerTypeName(v), v)
		return lua.LNil
	}
}

func createUserData(L *lua.LState, value interface{}, mtName string) *lua.LUserData {
	if len(mtName) > 0 && L.GetTypeMetatable(mtName) == lua.LNil {
		RegisterUserData(L, value)
	}
	ud := L.NewUserData()
	ud.Value = value
	if len(mtName) > 0 {
		L.SetMetatable(ud, L.GetTypeMetatable(mtName))
	}
	return ud
}
