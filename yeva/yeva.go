package yeva

import (
	"fmt"
	"time"
)

const (
	Name = "yeva"
)

var (
	Version = version{0, 1, 0}
)

type StackOverflow empty

type Yeva = executor

func New() *Yeva {
	e := &executor{
		globals: map[yv_string]global_cell{
			"print": {yv_native(native_print), false},
			"clock": {yv_native(native_clock), false},
		},
	}
	e.Interpret(embed)
	e.globals["assert"] = global_cell{e.pop(), false}
	return e
}

func (y *Yeva) PushNihil()           { y.push(yv_nihil{}) }
func (y *Yeva) PushBoolean(b bool)   { y.push(yv_boolean(b)) }
func (y *Yeva) PushNumber(n float64) { y.push(yv_number(n)) }
func (y *Yeva) PushString(s string)  { y.push(yv_string(s)) }
func (y *Yeva) PushNewStructure()    { y.push(new_structure(yv_nihil{})) }

func (y *Yeva) IsNihil() bool     { return is[yv_nihil](y.peek1()) }
func (y *Yeva) IsBoolean() bool   { return is[yv_boolean](y.peek1()) }
func (y *Yeva) IsNumber() bool    { return is[yv_number](y.peek1()) }
func (y *Yeva) IsString() bool    { return is[yv_string](y.peek1()) }
func (y *Yeva) IsStructure() bool { return is[yv_structure](y.peek1()) }

func (y *Yeva) StoreStructure() {
	v := y.pop()
	k := y.pop()
	s := y.peek1().(*yv_structure)
	s.store(k, v)
	y.push(v)
}
func (y *Yeva) LoadStructure() {
	k := y.pop()
	s := y.peek1().(*yv_structure)
	y.push(s.load(k))
}

func native_print(y *Yeva, args []yv_value) native_result {
	for i, arg := range args {
		fmt.Print(fmt_value(arg))
		if i != len(args)-1 {
			fmt.Print(" ")
		}
	}
	fmt.Println()
	y.PushNihil()
	return ResultOk
}

func native_clock(y *Yeva, args []yv_value) native_result {
	y.PushNumber(float64(time.Now().UnixNano()) / float64(time.Second))
	return ResultOk
}
