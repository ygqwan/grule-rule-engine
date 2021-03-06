package ast

//go:generate mockgen -destination=../mocks/ast/DataContext.go -package=mocksAst . IDataContext

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hyperjumptech/grule-rule-engine/pkg"
)

// NewDataContext will create a new DataContext instance
func NewDataContext() IDataContext {
	return &DataContext{
		ObjectStore: make(map[string]interface{}),

		retracted:           make([]string, 0),
		variableChangeCount: 0,
	}
}

// DataContext holds all structs instance to be used in rule execution environment.
type DataContext struct {
	ObjectStore map[string]interface{}

	retracted           []string
	variableChangeCount uint64
	complete            bool
}

// Complete marks the DataContext as completed, telling the engine to stop processing rules
func (ctx *DataContext) Complete() {
	ctx.complete = true
}

// IsComplete checks whether the DataContext has been completed
func (ctx *DataContext) IsComplete() bool {
	return ctx.complete
}

// IDataContext is the interface for the DataContext struct.
type IDataContext interface {
	ResetVariableChangeCount()
	IncrementVariableChangeCount()
	HasVariableChange() bool

	Add(key string, obj interface{}) error

	Retract(key string)
	IsRetracted(key string) bool
	Complete()
	IsComplete() bool
	Retracted() []string
	Reset()

	ExecMethod(methodName string, args []reflect.Value) (reflect.Value, error)

	GetType(variable string) (reflect.Type, error)

	GetValue(variable string) (reflect.Value, error)
	SetValue(variable string, newValue reflect.Value) error

	//only when the data ctx useless, will free for gc optimize
	ResetAllFiledZero()
}

// ResetVariableChangeCount will reset the variable change count
func (ctx *DataContext) ResetVariableChangeCount() {
	ctx.variableChangeCount = 0
}

// IncrementVariableChangeCount will increment the variable change count
func (ctx *DataContext) IncrementVariableChangeCount() {
	ctx.variableChangeCount++
}

// HasVariableChange returns true if there are variable changes
func (ctx *DataContext) HasVariableChange() bool {
	return ctx.variableChangeCount > 0
}

// Add will add struct instance into rule execution context
func (ctx *DataContext) Add(key string, obj interface{}) error {
	objVal := reflect.ValueOf(obj)
	if objVal.Kind() != reflect.Ptr || objVal.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("you can only insert a pointer to struct as fact. objVal = %s", objVal.Kind().String())
	}
	ctx.ObjectStore[key] = obj
	return nil
}

// Retract temporary retract a fact from data context, making it unavailable for evaluation or modification.
func (ctx *DataContext) Retract(key string) {
	ctx.retracted = append(ctx.retracted, key)
}

// IsRetracted checks if a key fact is currently retracted.
func (ctx *DataContext) IsRetracted(key string) bool {
	for _, v := range ctx.retracted {
		if v == key {
			return true
		}
	}
	return false
}

// Retracted returns list of retracted key facts.
func (ctx *DataContext) Retracted() []string {
	return ctx.retracted
}

// Reset will un-retract all fact, making them available for evaluation and modification.
func (ctx *DataContext) Reset() {
	ctx.retracted = make([]string, 0)
}

// ExecMethod will execute instance member variable using the supplied arguments.
func (ctx *DataContext) ExecMethod(methodName string, args []reflect.Value) (reflect.Value, error) {
	varArray := strings.Split(methodName, ".")
	if val, ok := ctx.ObjectStore[varArray[0]]; ok {
		if !ctx.IsRetracted(varArray[0]) {
			return traceMethod(val, varArray[1:], args)
		}
		return reflect.ValueOf(nil), fmt.Errorf("fact is retracted")
	}
	return reflect.ValueOf(nil), fmt.Errorf("fact [%s] not found while execute method", varArray[0])
}

// GetType will extract type information of data in this context.
func (ctx *DataContext) GetType(variable string) (reflect.Type, error) {
	varArray := strings.Split(variable, ".")
	if val, ok := ctx.ObjectStore[varArray[0]]; ok {
		if !ctx.IsRetracted(varArray[0]) {
			return traceType(val, varArray[1:])
		}
		return nil, fmt.Errorf("fact is retracted")
	}
	return nil, fmt.Errorf("fact [%s] not found while obtaining type", variable)
}

// GetValue will get member variables Value information.
// Used by the rule execution to obtain variable value.
func (ctx *DataContext) GetValue(variable string) (reflect.Value, error) {
	varArray := strings.Split(variable, ".")
	if val, ok := ctx.ObjectStore[varArray[0]]; ok {
		if !ctx.IsRetracted(varArray[0]) {
			vval, err := traceValue(val, varArray[1:])
			if err != nil {
				fmt.Printf("blah %s = %v\n", variable, vval)
			}
			return vval, err
		}
		return reflect.ValueOf(nil), fmt.Errorf("fact is retracted")
	}
	return reflect.ValueOf(nil), fmt.Errorf("fact [%s] not found while retrieving value", varArray[0])
}

// SetValue will set variable value of an object instance in this data context, Used by rule script to set values.
func (ctx *DataContext) SetValue(variable string, newValue reflect.Value) error {
	varArray := strings.Split(variable, ".")
	if val, ok := ctx.ObjectStore[varArray[0]]; ok {
		if !ctx.IsRetracted(varArray[0]) {
			err := traceSetValue(val, varArray[1:], newValue)
			if err == nil {
				ctx.variableChangeCount++
			}
			return err
		}
		return fmt.Errorf("fact is retracted")
	}
	return fmt.Errorf("fact [%s] not found while setting value", varArray[0])
}

func (ctx *DataContext) ResetAllFiledZero() {
	ctx.complete = false
	ctx.ObjectStore = make(map[string]interface{})
	ctx.retracted = make([]string, 0)
	ctx.variableChangeCount = 0
}

func traceType(obj interface{}, path []string) (reflect.Type, error) {
	switch length := len(path); {
	case length == 1:
		return pkg.GetAttributeType(obj, path[0])
	case length > 1:
		objVal, err := pkg.GetAttributeValue(obj, path[0])
		if err != nil {
			return nil, err
		}
		return traceType(pkg.ValueToInterface(objVal), path[1:])
	default:
		return reflect.TypeOf(obj), nil
	}
}

func traceValue(obj interface{}, path []string) (reflect.Value, error) {
	switch length := len(path); {
	case length == 1:
		return pkg.GetAttributeValue(obj, path[0])
	case length > 1:
		objVal, err := pkg.GetAttributeValue(obj, path[0])
		if err != nil {
			return objVal, err
		}
		return traceValue(pkg.ValueToInterface(objVal), path[1:])
	default:
		return reflect.ValueOf(obj), nil
	}
}

func traceSetValue(obj interface{}, path []string, newValue reflect.Value) error {
	switch length := len(path); {
	case length == 1:
		return pkg.SetAttributeValue(obj, path[0], newValue)
	case length > 1:
		objVal, err := pkg.GetAttributeValue(obj, path[0])
		if err != nil {
			return err
		}
		return traceSetValue(objVal, path[1:], newValue)
	default:
		return fmt.Errorf("no attribute path specified")
	}
}

func traceMethod(obj interface{}, path []string, args []reflect.Value) (reflect.Value, error) {

	switch length := len(path); {
	case length == 1:
		// this obj is reflect.Value... it should not.
		types, variad, err := pkg.GetFunctionParameterTypes(obj, path[0])
		if err != nil {
			return reflect.ValueOf(nil),
				fmt.Errorf("error while fetching function %s() parameter types. Got %v", path[0], err)
		}

		if len(types) != len(args) && !variad {
			return reflect.ValueOf(nil),
				fmt.Errorf("invalid argument count for function %s(). need %d argument while there are %d", path[0], len(types), len(args))
		}
		iargs := make([]interface{}, len(args))
		for i, t := range types {
			if variad && i == len(types)-1 {
				break
			}
			if t.Kind() != args[i].Kind() {
				if t.Kind() == reflect.Interface {
					iargs[i] = pkg.ValueToInterface(args[i])
				} else {
					return reflect.ValueOf(nil),
						fmt.Errorf("invalid argument types for function %s(). argument #%d, require %s but %s", path[0], i, t.Kind().String(), args[i].Kind().String())
				}
			} else {
				iargs[i] = pkg.ValueToInterface(args[i])
			}
		}
		if variad {
			typ := types[len(types)-1].Elem().Kind()
			for i := len(types) - 1; i < len(args); i++ {
				if args[i].Kind() != typ {
					return reflect.ValueOf(nil),
						fmt.Errorf("invalid variadic argument types for function %s(). argument #%d, require %s but %s", path[0], i, typ.String(), args[i].Kind().String())
				}
				iargs[i] = pkg.ValueToInterface(args[i])
			}
		}
		rets, err := pkg.InvokeFunction(obj, path[0], iargs)
		if err != nil {
			return reflect.ValueOf(nil), err
		}
		switch retLen := len(rets); {
		case retLen > 1:
			return reflect.ValueOf(rets[0]), fmt.Errorf("multiple return value for function %s(). ", path[0])
		case retLen == 1:
			return reflect.ValueOf(rets[0]), nil
		default:
			return reflect.ValueOf(nil), nil
		}
	case length > 1:
		objVal, err := pkg.GetAttributeValue(obj, path[0])
		if err != nil {
			return reflect.ValueOf(nil), err
		}
		return traceMethod(objVal, path[1:], args)
	default:
		return reflect.ValueOf(nil), fmt.Errorf("no function path specified")
	}
}
