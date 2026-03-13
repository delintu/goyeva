package yeva

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type yv_value interface {
	typeof() yv_string
}

type yv_nihil empty
type yv_boolean bool
type yv_number float64
type yv_string string

func string_format(format string, a ...any) yv_string {
	return yv_string(fmt.Sprintf(format, a...))
}

type struct_proto interface {
	yv_value
	load(key yv_value) yv_value
}

func (n yv_nihil) load(key yv_value) yv_value {
	return yv_nihil{}
}

type yv_structure struct {
	data  map[yv_value]yv_value
	proto struct_proto
}

func new_structure(proto struct_proto) *yv_structure {
	return &yv_structure{
		data:  make(map[yv_value]yv_value, struct_cap),
		proto: proto,
	}
}

func (s *yv_structure) store(k yv_value, v yv_value) {
	if is[yv_nihil](v) {
		delete(s.data, k)
	} else {
		s.data[k] = v
	}
}

func (s *yv_structure) load(k yv_value) (v yv_value) {
	if v, ok := s.data[k]; ok {
		return v
	} else {
		return s.proto.load(k)
	}
}

type upval_info struct {
	location int
	is_local bool
}

type fn_proto struct {
	name   string
	code   []uint8
	lines  []int
	values []yv_value
	fns    []*fn_proto
	paramc int
	upvals []upval_info
	localc int
	vararg bool
}

func (f *fn_proto) write_code(op op_code, line int) {
	slice_push(&f.code, op)
	slice_push(&f.lines, line)
}

func (f *fn_proto) add_value(v yv_value) int {
	for i, sv := range f.values {
		if sv == v {
			return i
		}
	}
	slice_push(&f.values, v)
	return len(f.values) - 1
}

func (f *fn_proto) add_fn(fp *fn_proto) int {
	slice_push(&f.fns, fp)
	return len(f.fns) - 1
}

type upvalue struct {
	loc    int
	ref    *[]yv_value
	fr     *call_frame
	clsd   yv_value
	is_mut bool
}

func (u *upvalue) is_init() bool {
	if u.loc != -1 {
		return u.fr.initc > u.loc-u.fr.slots
	} else {
		return true
	}
}

func (u *upvalue) close() {
	clsd, _ := u.load()
	u.clsd, u.loc = clsd, -1
}

func (u *upvalue) store(v yv_value) yv_value {
	if !u.is_mut {
		return rte_store_constant
	} else if !u.is_init() {
		return rte_store_uninit
	}
	if u.loc != -1 {
		(*u.ref)[u.loc] = v
	} else {
		u.clsd = v
	}
	return nil
}

func (u *upvalue) load() (yv_value, yv_value) {
	if !u.is_init() {
		return nil, rte_load_uninit
	}
	if u.loc != -1 {
		return (*u.ref)[u.loc], nil
	} else {
		return u.clsd, nil
	}
}

type yv_closure struct {
	fn     *fn_proto
	upvals []*upvalue
}

type native_result int

const (
	ResultOk native_result = iota
	ResultThrow
)

type yv_native func(*executor, []yv_value) native_result

func (v yv_nihil) typeof() yv_string     { return nihil_literal }
func (v yv_boolean) typeof() yv_string   { return "boolean" }
func (v yv_number) typeof() yv_string    { return "number" }
func (v yv_string) typeof() yv_string    { return "string" }
func (v yv_structure) typeof() yv_string { return "structure" }
func (v *yv_closure) typeof() yv_string  { return "function" }
func (v yv_native) typeof() yv_string    { return "function" }

func fmt_value(v yv_value) string {
	switch v := v.(type) {
	case yv_nihil:
		return nihil_literal
	case yv_boolean:
		return strconv.FormatBool(bool(v))
	case yv_number:
		return fmt_number(v)
	case yv_string:
		return string(v)
	case *yv_structure:
		return fmt.Sprintf("<structure %p>", v)
		// return fmt_structure(v)
	case *yv_closure:
		return fmt.Sprintf("<function %p>", v)
	case yv_native:
		return fmt.Sprintf("<function %p>", v)
	default:
		panic(unreachable)
	}
}

func fmt_number(n yv_number) string {
	f := float64(n)
	if math.IsNaN(f) {
		return "nan"
	} else if math.IsInf(f, 0) {
		if math.IsInf(f, 1) {
			return "inf"
		} else {
			return "-inf"
		}
	} else {
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
}

func fmt_structure(s *yv_structure) string {
	var f func(s *yv_structure, ref map[*yv_structure]empty) string
	f = func(s *yv_structure, ref map[*yv_structure]empty) string {
		if len(s.data) == 0 {
			return "{}"
		}
		if ref == nil {
			ref = map[*yv_structure]empty{s: {}}
		} else {
			ref[s] = empty{}
		}
		var r strings.Builder
		r.WriteString("{")
		prev := false
		for k, v := range s.data {
			if prev {
				r.WriteString(", ")
			}
			if struk, ok := k.(*yv_structure); ok {
				if map_has(ref, struk) {
					fmt.Fprintf(&r, "{...}: ")
				} else {
					fmt.Fprintf(&r, "%s: ", f(struk, ref))
				}
			} else if strk, ok := k.(yv_string); ok {
				fmt.Fprintf(&r, "\"%s\": ", strk)
			} else {
				fmt.Fprintf(&r, "%s: ", fmt_value(k))
			}
			if struv, ok := v.(*yv_structure); ok {
				if map_has(ref, struv) {
					fmt.Fprintf(&r, "{...}")
				} else {
					fmt.Fprintf(&r, "%s", f(struv, ref))
				}
			} else if strv, ok := v.(yv_string); ok {
				fmt.Fprintf(&r, "\"%s\"", strv)
			} else {
				fmt.Fprintf(&r, "%s", fmt_value(v))
			}
			prev = true
		}
		r.WriteString("}")
		return r.String()
	}
	return f(s, nil)
}

func to_boolean(v yv_value) yv_boolean {
	switch v := v.(type) {
	case yv_nihil:
		return false
	case yv_boolean:
		return v
	case yv_number:
		return v != 0
	case yv_string:
		return v != ""
	default:
		return true
	}
}
