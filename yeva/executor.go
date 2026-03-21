package yeva

import (
	"fmt"
	"maps"
	"math"
	"math/bits"
	"os"
)

const (
	frames_limit = 0xff
)

type call_frame struct {
	closure *yv_closure
	pc      int
	slots   int
}

func (cf *call_frame) read_var() int {
	v, a := decode(cf.closure.fn.code[cf.pc:])
	cf.pc += a
	return v
}

func (cf *call_frame) read_byte() uint8 {
	cf.pc++
	return cf.closure.fn.code[cf.pc-1]
}

func (cf *call_frame) read_short() uint16 {
	return u8tou16(cf.read_byte(), cf.read_byte())
}

func (cf *call_frame) read_long() uint32 {
	return u8tou32(
		cf.read_byte(),
		cf.read_byte(),
		cf.read_byte(),
		cf.read_byte(),
	)
}

func (cf *call_frame) read_value() yv_value {
	return cf.closure.fn.values[cf.read_var()]
}

func (cf *call_frame) read_function() *fn_proto {
	return cf.closure.fn.funcs[cf.read_var()]
}

func (cf *call_frame) read_string() yv_string {
	return cf.read_value().(yv_string)
}

type catch_handler struct {
	recover        int
	stack_len      int
	call_stack_len int
}

type linked_node[T any] struct {
	value T
	next  *linked_node[T]
}

type exe_status int

const (
	status_suspended = iota
	status_running
	status_dead
)

type executor struct {
	globals        map[yv_string]yv_value
	call_stack     []call_frame
	stack          []yv_value
	catch_handlers []catch_handler
	temp           yv_value
	open_upvals    *linked_node[*upvalue]
	status         exe_status
}

func (e *executor) add_open_upval(loc int) *upvalue {
	fr := e.current_frame()
	loc = fr.slots + loc // absolute location
	var prev *linked_node[*upvalue] = nil
	cur := e.open_upvals
	for cur != nil && cur.value.abs_loc > loc {
		prev = cur
		cur = cur.next
	}
	if cur != nil && cur.value.abs_loc == loc {
		return cur.value
	}
	new_upvalue := &upvalue{
		abs_loc: loc,
		ref:     &e.stack,
	}
	new_node := &linked_node[*upvalue]{value: new_upvalue, next: cur}
	if prev != nil {
		prev.next = new_node
	} else {
		e.open_upvals = new_node
	}
	return new_upvalue
}

func (e *executor) close_upvals(loc_of_last int) {
	var save *linked_node[*upvalue] = nil
	var cur_save *linked_node[*upvalue] = save
	for e.open_upvals != nil && e.open_upvals.value.abs_loc >= loc_of_last {
		if e.open_upvals.value.is_init {
			e.open_upvals.value.close()
		} else if cur_save != nil {
			cur_save.next = e.open_upvals
			cur_save = cur_save.next
		} else {
			save = e.open_upvals
		}
		e.open_upvals = e.open_upvals.next
	}
	if save != nil {
		cur_save.next = e.open_upvals
		e.open_upvals = save
	}
}

func (e *executor) bind_upvalue2(upval *upvalue, info upval2_info) {
	switch info := info.(type) {
	case upval2_name_info:
		upval.up2 = up2(info)
	case upval2_open_info:
		fr := e.call_stack[len(e.call_stack)-1-info.back]
		upval.up2 = up2(fr.slots + info.loc)
	case nil:
		upval.is_init = true
	default:
		panic(unreachable)
	}
}

func (e *executor) call_value(callee yv_value, argc int) yv_value {
	switch callee := callee.(type) {
	case yv_native:
		return e.call_native(callee, argc)
	case *yv_closure:
		return e.call_closure(callee, argc)
	default:
		return string_format("attempt to call %s", callee.typeof())
	}
}

func (e *executor) call_native(nat_fn yv_native, argc int) (throw yv_value) {
	switch nat_fn(e, e.stack[len(e.stack)-argc:]) {
	case ResultOk:
		r := e.pop()
		slice_cut(&e.stack, -argc-1)
		e.push(r)
		return nil
	case ResultThrow:
		return e.pop()
	default:
		panic(unreachable)
	}
}

func (e *executor) call_closure(
	cls *yv_closure,
	argc int,
) yv_value {
	if len(e.call_stack) == frames_limit {
		panic(StackOverflow{})
	}
	new_frame := call_frame{
		closure: cls,
		pc:      0,
		slots:   len(e.stack) - argc,
	}
	slice_push(&e.call_stack, new_frame)
	e.balance_args(argc, cls.fn.paramc, cls.fn.vararg)
	return nil
}

func (e *executor) balance_args(argc int, paramc int, vararg bool) {
	var varg *yv_structure
	if vararg {
		varg = new_structure(yv_nihil{})
	}
	if argc <= paramc {
		for range paramc - argc {
			e.push(yv_nihil{})
		}
	} else {
		shift := argc - paramc
		if vararg {
			for i := range shift {
				varg.store(yv_number(shift-1-i), e.pop())
			}
		} else {
			slice_cut(&e.stack, -shift)
		}
	}
	if vararg {
		e.push(varg)
	}
}

func (e *executor) store_local(cf *call_frame, idx int, v yv_value) {
	e.stack[cf.slots+idx] = v
}

func (e *executor) load_local(cf *call_frame, idx int) yv_value {
	return e.stack[cf.slots+idx]
}

func (e *executor) store_global(name yv_string, v yv_value) yv_value {
	_, ok := e.globals[name]
	if !ok {
		return rte_not_defined
	}
	e.globals[name] = v
	return nil
}

func (e *executor) load_global(name yv_string) (yv_value, yv_value) {
	v, ok := e.globals[name]
	if !ok {
		return nil, rte_not_defined
	}
	return v, nil
}

func (e *executor) push(v yv_value) {
	slice_push(&e.stack, v)
}

func (e *executor) pushf(format string, a ...any) {
	e.push(yv_string(fmt.Sprintf(format, a...)))
}

func (e *executor) pop() (v yv_value) {
	return slice_pop(&e.stack)
}

func (e *executor) peek1() (v yv_value) {
	return e.stack[len(e.stack)-1]
}

func (e *executor) peek2() (v yv_value) {
	return e.stack[len(e.stack)-2]
}

func (e *executor) peek(i int) (v yv_value) {
	return e.stack[len(e.stack)+i]
}

func (e *executor) current_frame() *call_frame {
	return slice_last(e.call_stack)
}

func (e *executor) unwind(fr **call_frame) bool {
	if len(e.catch_handlers) != 0 {
		handler := slice_pop(&e.catch_handlers)
		e.stack = e.stack[:handler.stack_len]
		e.call_stack = e.call_stack[:handler.call_stack_len]
		*fr = e.current_frame()
		(*fr).pc = handler.recover
		return true
	}
	return false
}

func (e *executor) runtime_error(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	fmt.Fprintf(os.Stderr, "stack trace:\n")
	for i := len(e.call_stack) - 1; i >= 0; i-- {
		frame := &e.call_stack[i]
		fn := frame.closure.fn
		line := fn.lines[frame.pc-1]
		fmt.Fprintf(os.Stderr, "  ln %d: in function %s\n", line, fn.name)
	}
	os.Exit(1) /* ??? */
}

type Context struct {
	started bool
	locals  []local
}

func (e *executor) Interpret(ctx *Context, src []byte) {
	c := new_compiler(src)
	c.locals = ctx.locals
	fn := c.compile()
	ctx.locals = c.locals
	if fn == nil {
		return
	}
	if dbg_code {
		log_fn(fn)
	}
	cls := &yv_closure{fn: fn}
	if dbg_exe {
		fmt.Println(cover("@execution", 30, '=') + "|")
	}
	if !ctx.started {
		ctx.started = true
		e.push(cls)
		e.call_closure(cls, 0)
	} else {
		e.stack[0] = cls
		fr := e.current_frame()
		fr.closure = cls
		fr.pc = 0
	}
	e.execute()
}

func (e *executor) execute() {
	e.status = status_running
	fr := e.current_frame()
	if dbg_exe {
		defer func() {
			for _, v := range e.stack {
				fmt.Printf("[%s]", fmt_value(v))
			}
			fmt.Println()
		}()
	}
	for {
		if dbg_exe {
			log_opcode(fr.closure.fn, fr.pc)
			for _, v := range e.stack {
				fmt.Printf("[%s]", fmt_value(v))
			}
			fmt.Println()
		}
		switch op := fr.read_byte(); op {
		case op_pop:
			e.pop()
		case op_dup:
			e.push(e.peek1())
		case op_dup2:
			e.push(e.peek2())
			e.push(e.peek2())
		case op_swap:
			e.stack[len(e.stack)-1],
				e.stack[len(e.stack)-2] =
				e.peek2(),
				e.peek1()
		case op_begin_catch:
			rcvr := int(int16(fr.read_short()))
			slice_push(&e.catch_handlers, catch_handler{
				recover:        fr.pc + rcvr,
				stack_len:      len(e.stack),
				call_stack_len: len(e.call_stack),
			})
		case op_end_catch:
			slice_pop(&e.catch_handlers)
			v := e.pop()
			r := new_structure(yv_nihil{})
			r.store(key_result, v)
			e.push(r)
		case op_throw:
			goto unwind
		case op_nihil:
			e.push(yv_nihil{})
		case op_false:
			e.push(yv_boolean(false))
		case op_true:
			e.push(yv_boolean(true))
		case op_value:
			e.push(fr.read_value())
		case op_copy_to:
			e.temp = e.peek1()
		case op_copy_from:
			e.stack[len(e.stack)-1] = e.temp
		case op_destruct:
			c := fr.read_var()
			d := e.pop()
			if s, ok := d.(*yv_structure); ok {
				for i := range c {
					k := e.peek(-1 - i)
					e.stack[len(e.stack)-1-i] = s.load(k)
				}
			} else {
				e.pushf("attempt to destructure '%s'", d.typeof())
				goto unwind
			}
		case op_store_local:
			e.store_local(fr, fr.read_var(), e.peek1())
		case op_load_local:
			e.push(e.load_local(fr, fr.read_var()))
		case op_store_name:
			if err := e.store_global(fr.read_string(), e.peek1()); err != nil {
				e.push(err)
				goto unwind
			}
		case op_load_name:
			v, err := e.load_global(fr.read_string())
			if err != nil {
				e.push(err)
				goto unwind
			}
			e.push(v)
		case op_store_upvalue:
			err := fr.closure.upvals[fr.read_var()].store(e, e.peek1())
			if err != nil {
				e.push(err)
				goto unwind
			}
		case op_load_upvalue:
			v, err := fr.closure.upvals[fr.read_var()].load(e)
			if err != nil {
				e.push(err)
				goto unwind
			}
			e.push(v)
		case op_closure:
			fn := fr.read_function()
			cls := &yv_closure{fn: fn, upvals: make([]*upvalue, len(fn.upvals))}
			for i, upv := range fn.upvals {
				if upv.is_local {
					cls.upvals[i] = e.add_open_upval(upv.location)
				} else {
					cls.upvals[i] = fr.closure.upvals[upv.location]
				}
				e.bind_upvalue2(cls.upvals[i], upv.upval2)
			}
			e.push(cls)
		case op_close_upvalue:
			e.close_upvals(len(e.stack) - 1)
			e.pop()
		case op_init_upvalue:
			cur := e.open_upvals
			for cur.value.abs_loc != len(e.stack)-1 {
				cur = cur.next
			}
			cur.value.is_init = true
		case op_structure:
			v := e.pop()
			if p, ok := v.(struct_proto); !ok {
				e.pushf("can't use %s as structure prototype", v.typeof())
				goto unwind

			} else {
				e.push(new_structure(p))
			}
		case op_define_key:
			v := e.pop()
			k := e.pop()
			s := e.peek1().(*yv_structure)
			s.data[k] = v
		case op_define_key_spread:
			v := e.pop()
			s := e.peek1().(*yv_structure)
			vs, ok := v.(*yv_structure)
			if !ok {
				e.pushf("attempt to spread %s", v.typeof())
				goto unwind
			}
			maps.Copy(s.data, vs.data)
		case op_store_key:
			v := e.pop()
			k := e.pop()
			to := e.pop()
			s, ok := to.(*yv_structure)
			if !ok {
				e.pushf("attempt to store key to %s", to.typeof())
				goto unwind
			}
			s.store(k, v)
			e.push(v)
		case op_load_key:
			k := e.pop()
			to := e.pop()
			s, ok := to.(*yv_structure)
			if !ok {
				e.pushf("attempt to load key from %s", to.typeof())
				goto unwind
			}
			e.push(s.load(k))
		case op_typeof:
			e.push(e.pop().typeof())
		case op_not:
			e.push(!to_boolean(e.pop()))
		case op_rev, op_neg, op_pos:
			if v, ok := e.pop().(yv_number); ok {
				e.push(un_num_ops[op-op_rev](v))
			} else {
				e.pushf("attempt to %s %s", op_names[op], v.typeof())
				goto unwind
			}
		case op_eq:
			v2 := e.pop()
			v1 := e.pop()
			e.push(yv_boolean(v1 == v2))
		case op_add, op_sub, op_mul, op_div, op_mod,
			op_or, op_xor, op_and, op_lsh, op_rsh,
			op_lt, op_le:
			v2 := e.pop()
			v1 := e.pop()
			if v1n, v2n, ok := assert2[yv_number](v1, v2); ok {
				e.push(bin_num_ops[op-op_add](v1n, v2n))
			} else if op == op_add {
				if v1s, v2s, ok := assert2[yv_string](v1, v2); ok {
					e.push(v1s + v2s)
					break
				}
				e.pushf(
					"attempt to %s %s and %s",
					op_names[op], v1.typeof(), v2.typeof(),
				)
				goto unwind
			}
		case op_goto:
			fr.pc += int(int16(fr.read_short()))
		case op_goto_if_false:
			jump := int(int16(fr.read_short()))
			if !to_boolean(e.peek1()) {
				fr.pc += jump
			}
		case op_goto_if_nihil:
			jump := int(int16(fr.read_short()))
			if is[yv_nihil](e.peek1()) {
				fr.pc += jump
			}
		case op_call:
			argc := fr.read_var()
			callee := e.peek(-1 - argc)
			if err := e.call_value(callee, argc); err != nil {
				e.push(err)
				goto unwind
			}
			fr = e.current_frame()
		case op_call_spread:
			argc := fr.read_var()
			if spr, ok := e.pop().(*yv_structure); ok {
				var i yv_number
				for i = 0; ; i++ {
					v := spr.load(i)
					if !is[yv_nihil](v) {
						e.push(v)
						argc++
					} else {
						break
					}
				}
			} else {
				e.pushf("attempt to spread %s", spr.typeof())
				goto unwind
			}
			callee := e.peek(-1 - argc)
			if err := e.call_value(callee, argc); err != nil {
				e.push(err)
				goto unwind
			}
			fr = e.current_frame()
		case op_return:
			r := e.pop()
			e.close_upvals(fr.slots)
			e.stack = e.stack[:fr.slots-1]
			e.push(r)
			slice_pop(&e.call_stack)
			if len(e.call_stack) == 0 {
				e.status = status_dead
				return
			}
			fr = e.current_frame()
		case op_suspend:
			e.status = status_suspended
			return
		default:
			panic(unreachable)
		}
		continue
	unwind:
		v := e.pop()
		if !e.unwind(&fr) {
			e.runtime_error("uncaught: %s", fmt_value(v))
			return
		}
		r := new_structure(yv_nihil{})
		r.store(key_catched, v)
		e.push(r)
	}
}

var un_num_ops = [...]func(v yv_number) yv_value{
	0:               rev,
	op_neg - op_rev: neg,
	op_pos - op_rev: pos,
}

func rev(v yv_number) yv_value {
	return yv_number(int(bits.Reverse(uint(v))))
}

func neg(v yv_number) yv_value {
	return -v
}

func pos(v yv_number) yv_value {
	return yv_number(math.Abs(float64(v)))
}

var bin_num_ops = [...]func(v1, v2 yv_number) yv_value{
	0:               add,
	op_sub - op_add: sub,
	op_mul - op_add: mul,
	op_div - op_add: div,
	op_mod - op_add: mod,
	op_or - op_add:  or,
	op_xor - op_add: xor,
	op_and - op_add: and,
	op_lsh - op_add: lsh,
	op_rsh - op_add: rsh,
	op_lt - op_add:  lt,
	op_le - op_add:  le,
}

func add(v1, v2 yv_number) yv_value {
	return v1 + v2
}

func sub(v1, v2 yv_number) yv_value {
	return v1 - v2
}

func mul(v1, v2 yv_number) yv_value {
	return v1 * v2
}

func div(v1, v2 yv_number) yv_value {
	return v1 / v2
}

func mod(v1, v2 yv_number) yv_value {
	return yv_number(math.Mod(float64(v1), float64(v2)))
}

func or(v1, v2 yv_number) yv_value {
	return yv_number(int(v1) | int(v2))
}

func xor(v1, v2 yv_number) yv_value {
	return yv_number(int(v1) ^ int(v2))
}

func and(v1, v2 yv_number) yv_value {
	return yv_number(int(v1) & int(v2))
}

func lsh(v1, v2 yv_number) yv_value {
	return yv_number(int(v1) << int(v2))
}

func rsh(v1, v2 yv_number) yv_value {
	return yv_number(int(v1) >> int(v2))
}

func lt(v1, v2 yv_number) yv_value {
	return yv_boolean(v1 < v2)
}

func le(v1, v2 yv_number) yv_value {
	return yv_boolean(v1 <= v2)
}
