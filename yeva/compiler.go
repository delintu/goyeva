package yeva

import (
	"fmt"
	"math"
	"os"
	"strconv"
)

type compile_context struct {
	lates map[string]late_bind
}

type late_bind struct {
	info *upval_info

	store_ops []int
	load_ops  []int
}

type compiler struct {
	*parser
	enclosing *compiler
	locals    []local
	scope     int
	loop      *loop
	fn        *fn_proto
	prefix    bool_stack16
	ctx       *compile_context
}

func new_compiler(src []byte) *compiler {
	return &compiler{
		parser: new_parser(src),
		fn:     &fn_proto{name: "@script"},
	}
}

func (c *compiler) new_sub_compiler(fn_name string) *compiler {
	return &compiler{
		parser:    c.parser,
		enclosing: c,
		fn:        &fn_proto{name: fn_name},
		ctx:       c.ctx,
	}
}

func (c *compiler) compile() *fn_proto {
	for !c.step_on(lx_eof) {
		c.decl()
	}
	c.emit_return()
	if c.had_error {
		return nil
	}
	return c.fn
}

func (c *compiler) decl() {
	func() {
		defer catch(func(p parse_error) { c.sync() })
		switch {
		case c.check_next(lx_name) &&
			c.step_on(lx_function):
			c.function_decl()
		case c.check_next(lx_name) &&
			c.step_on(lx_struct):
			c.structure_decl()
		case c.step_on(lx_variable):
			c.multiline_decl(c.variable_decl)
		default:
			c.stmt()
		}
	}()
}

func (c *compiler) multiline_decl(f func()) {
	many := c.step_on(lx_lparen)
	for {
		if many && c.step_on(lx_rparen) {
			c.expect_semi()
			break
		}
		f()
		if !many {
			break
		}
	}
}

func (c *compiler) variable_decl() {
	for {
		if c.step_on(lx_lbrace) {
			i, kc := 0, 0
			for {
				i++
				if i > math.MaxUint8 {
					panic(unreachable)
				}
				if c.step_on(lx_dot) {
					kname := c.expect_name()
					if c.step_on(lx_equal) {
						c.declare_variable(c.expect_name())
					} else {
						c.declare_variable(kname)
					}
					c.emit_value(yv_string(kname))
				} else if c.step_on(lx_lbrack) {
					c.expr(true)
					c.expect(lx_rbrack)
					c.expect(lx_equal)
					c.declare_variable(c.expect_name())
				} else {
					c.declare_variable(c.expect_name())
					c.emit_value(yv_number(kc))
					kc++
				}
				if !c.step_on(lx_comma) {
					break
				}
				if c.check(lx_rbrace) {
					break
				}
			}
			c.expect(lx_rbrace)
			c.expect(lx_equal)
			c.expr(false)
			c.emit(op_destruct, uint8(i))
		} else {
			c.declare_variable(c.expect_name())
			if c.step_on(lx_equal) {
				c.expr(false)
			} else {
				c.emit(op_nihil)
			}
		}
		if !c.step_on(lx_comma) {
			break
		}
	}
	c.define_variables()
	c.expect_semi()
}

func (c *compiler) function_decl() {
	c.declare_variable(c.expect_name())
	c.define_variables()
	if c.named_function(c.previous.literal, false) {
		c.expect_semi()
	}
}

func (c *compiler) structure_decl() {
	c.declare_variable(c.expect_name())
	c.define_variables()
	c.parse_structure(false)
	c.expect_semi()
}

func (c *compiler) stmt() {
	switch {
	case c.step_on(lx_semi), c.step_on(lx_line):
		/* pass */
	case c.step_on(lx_lbrace):
		c.begin_scope()
		c.block()
		c.end_scope()
	case c.step_on(lx_if):
		c.if_stmt(c.step_on(lx_bang))
	case c.step_on(lx_while):
		c.while_stmt(c.step_on(lx_bang), "")
	case c.step_on(lx_do):
		c.do_stmt("")
	case c.step_on(lx_for):
		c.for_stmt("")
	case c.step_on(lx_break):
		c.break_stmt()
	case c.step_on(lx_continue):
		c.continue_stmt()
	case c.step_on(lx_return):
		c.return_stmt()
	case c.step_on(lx_throw):
		c.throw_stmt()
	case c.check_next(lx_colon) &&
		c.step_on(lx_name):
		c.label_stmt()

	default:
		c.expr(true)
		c.emit(op_pop)
		c.expect_semi()
	}
}

func (c *compiler) block() {
	for !c.check(lx_rbrace) && !c.check(lx_eof) {
		c.decl()
	}
	c.expect(lx_rbrace)
}

func (c *compiler) if_stmt(reverse bool) {
	c.begin_scope()
	c.expect(lx_lparen)
	if c.step_on(lx_variable) {
		c.variable_decl()
	}
	c.expr(true)
	c.expect(lx_rparen)
	if reverse {
		c.emit(op_not)
	}
	then_jump := c.emit_goto(op_goto_if_false)
	c.emit(op_pop)
	c.ignore_line()
	c.stmt()
	else_jump := c.emit_goto(op_goto)
	c.patch_goto(then_jump)
	c.emit(op_pop)
	if c.step_on(lx_else) {
		c.stmt()
	}
	c.patch_goto(else_jump)
	c.end_scope()
}

func (c *compiler) while_stmt(reverse bool, label string) {
	c.begin_loop(label)
	c.expect(lx_lparen)
	c.expr(true)
	c.expect(lx_rparen)
	if reverse {
		c.emit(op_not)
	}
	exit_jump := c.emit_goto(op_goto_if_false)
	c.emit(op_pop)
	c.ignore_line()
	c.stmt()
	c.emit_goto_back(c.loop.start)
	c.patch_goto(exit_jump)
	c.emit(op_pop)
	c.end_loop()
}

func (c *compiler) do_stmt(label string) {
	c.begin_loop(label)
	c.ignore_line()
	c.stmt()
	c.expect(lx_while)
	reverse := c.step_on(lx_bang)
	c.expect(lx_lparen)
	c.expr(true)
	if reverse {
		c.emit(op_not)
	}
	exit_jump := c.emit_goto(op_goto_if_false)
	c.emit(op_pop)
	c.emit_goto_back(c.loop.start)
	c.expect(lx_rparen)
	c.expect_semi()
	c.patch_goto(exit_jump)
	c.emit(op_pop)
	c.end_loop()
}

func (c *compiler) for_stmt(label string) {
	c.begin_scope()
	c.expect(lx_lparen)
	if c.step_on(lx_semi) {
		/* pass */
	} else if c.step_on(lx_variable) {
		c.variable_decl()
	} else {
		c.expr(true)
		c.expect(lx_semi)
	}
	c.begin_loop(label)
	exit_jump := -1
	if !c.step_on(lx_semi) {
		c.expr(true)
		c.expect(lx_semi)
		exit_jump = c.emit_goto(op_goto_if_false)
		c.emit(op_pop)
	}
	if !c.step_on(lx_rparen) {
		body_jump := c.emit_goto(op_goto)
		inc_start := len(c.fn.code)
		c.expr(true)
		c.emit(op_pop)
		c.expect(lx_rparen)
		c.emit_goto_back(c.loop.start)
		c.loop.start = inc_start
		c.patch_goto(body_jump)
	}
	c.ignore_line()
	c.stmt()
	c.emit_goto_back(c.loop.start)
	if exit_jump != -1 {
		c.patch_goto(exit_jump)
		c.emit(op_pop)
	}
	c.end_loop()
	c.end_scope()
}

func (c *compiler) break_stmt() {
	if c.match_semi() {
		if c.loop != nil {
			c.end_loop_scopes(c.loop)
			slice_push(&c.loop.breaks, c.emit_goto(op_goto))
		} else {
			c.error_near_previous("'break' outside loop")
		}
	} else {
		label := c.expect_name()
		c.expect_semi()
		for loop := c.loop; loop != nil; loop = loop.enclosing {
			if loop.label == label {
				c.end_loop_scopes(loop)
				slice_push(&loop.breaks, c.emit_goto(op_goto))
				return
			}
		}
		c.error_near_previous("undefined label")
	}
}

func (c *compiler) continue_stmt() {
	if c.match_semi() {
		if c.loop != nil {
			c.end_loop_scopes(c.loop)
			c.emit_goto_back(c.loop.start)
		} else {
			c.error_near_previous("'continue' outside loop")
		}
	} else {
		label := c.expect_name()
		c.expect_semi()
		for loop := c.loop; loop != nil; loop = loop.enclosing {
			if loop.label == label {
				c.end_loop_scopes(loop)
				c.emit_goto_back(loop.start)
				return
			}
		}
		c.error_near_previous("undefined label")
	}
}

func (c *compiler) return_stmt() {
	if c.match_semi() {
		c.emit_return()
	} else {
		c.expr(true)
		c.expect_semi()
		c.emit(op_return)
	}
}

func (c *compiler) throw_stmt() {
	c.expr(true)
	c.emit(op_throw)
	c.expect_semi()
}

func (c *compiler) label_stmt() {
	name := c.previous.literal
	c.step()
	switch {
	case c.step_on(lx_while):
		c.while_stmt(false, name)
	case c.step_on(lx_do):
		c.do_stmt(name)
	case c.step_on(lx_for):
		c.for_stmt(name)
	default:
		c.stmt()
	}
}

func (c *compiler) expr(allow_comma bool) {
	if allow_comma {
		c.precedence(prec_comma)
	} else {
		c.precedence(prec_assign)
	}
}

func (c *compiler) precedence(prec precedence) {
	nud_fn := c.nud()
	c.step()
	if nud_fn == nil {
		c.error_near_current("expression expected")
		return
	}
	can_assign := prec <= prec_assign
	nud_fn(can_assign)
	for prec <= precedences[c.current.lx_type] {
		led_fn := c.led()
		c.step()
		led_fn(can_assign)
	}
	if c.current.lx_type == lx_minus_minus ||
		c.current.lx_type == lx_plus_plus {
		c.error_near_previous("invalid postfix")
	} else if can_assign && map_has(assign_lexemes, c.current.lx_type) {
		c.error_near_previous("invalid assignment")
	}
}

var assign_lexemes = map[lx_type]empty{
	lx_equal:         {},
	lx_plus_equal:    {},
	lx_minus_equal:   {},
	lx_star_equal:    {},
	lx_slash_equal:   {},
	lx_percent_equal: {},

	lx_pipe_equal:          {},
	lx_circum_equal:        {},
	lx_amper_equal:         {},
	lx_langle_langle_equal: {},
	lx_rangle_rangle_equal: {},

	lx_pipe_pipe_equal:   {},
	lx_amper_amper_equal: {},
}

func (c *compiler) nud() parse_func {
	switch c.current.lx_type {
	case lx_lparen:
		return c.parse_group
	case lx_name:
		return c.parse_name
	case lx_nihil, lx_false, lx_true:
		return c.parse_literal
	case lx_number:
		return c.parse_number
	case lx_string:
		return c.parse_string
	case lx_struct:
		return c.parse_structure
	case lx_function:
		return c.parse_function
	case lx_plus, lx_minus, lx_bang, lx_typeof, lx_tilde,
		lx_plus_plus, lx_minus_minus:
		return c.parse_prefix
	case lx_catch:
		return c.parse_catch
	default:
		return nil
	}
}

func (c *compiler) parse_group(can_assign bool) {
	c.expr(true)
	c.expect(lx_rparen)
}

func (c *compiler) parse_name(can_assign bool) {
	var store, load op_code
	var arg uint8
	name := c.previous.literal
	if idx, ok := c.resolve_local(name); ok {
		arg = uint8(idx)
		store = op_store_local
		load = op_load_local
	} else if idx, ok := c.resolve_upvalue(name); ok {
		// c.ctx.lates[name] = late_bind{}
		arg = uint8(idx)
		store = op_store_upvalue
		load = op_load_upvalue
	} else {
		arg = uint8(c.fn.add_value(yv_string(name)))
		// c.ctx.lates[name] = late_bind{}
		store = op_store_name
		load = op_load_name
	}
	c.assign(
		func() { c.emit(store, arg) },
		func() { c.emit(load, arg) },
		func() { c.emit(load, arg) },
		can_assign,
	)
}

func (c *compiler) resolve_local(name string) (int, bool) {
	for i := len(c.locals) - 1; i >= 0; i-- {
		if c.locals[i].name == name && c.locals[i].is_init {
			return i, true
		}
	}
	return 0, false
}

func (c *compiler) resolve_upvalue(name string) (int, bool) {
	if c.enclosing == nil {
		return 0, false
	} else if l, ok := c.enclosing.resolve_local(name); ok {
		c.enclosing.locals[l].is_upval = true
		return c.add_upvalue(l, true), true
	} else if u, ok := c.enclosing.resolve_upvalue(name); ok {
		return c.add_upvalue(u, false), true
	}
	return 0, false
}

func (c *compiler) add_upvalue(idx int, is_local bool) int {
	new_upv := upval_info{location: idx, is_local: is_local}
	for i, upv := range c.fn.upvals {
		if upv == new_upv {
			return i
		}
	}
	if len(c.fn.upvals) > math.MaxUint8 {
		c.error_near_previous("too many captured variables")
	}
	slice_push(&c.fn.upvals, new_upv)
	return len(c.fn.upvals) - 1
}

func (c *compiler) parse_literal(can_assign bool) {
	switch c.previous.lx_type {
	case lx_nihil:
		c.emit(op_nihil)
	case lx_true:
		c.emit(op_true)
	case lx_false:
		c.emit(op_false)
	default:
		panic(unreachable)
	}
}

func (c *compiler) parse_number(can_assign bool) {
	n, _ := strconv.ParseFloat(c.previous.literal, 64)
	c.emit_value(yv_number(n))
}

func (c *compiler) parse_string(can_assign bool) {
	s := []byte(c.previous.literal[1 : len(c.previous.literal)-1])
	r := make([]byte, 0, len(s))
	for i, cur := range s {
		if cur == '\\' {
			var next byte
			if i == len(s)-1 {
				next = nul
			} else {
				next = s[i+1]
			}
			switch next {
			case 'a':
				slice_push(&r, '\a')
			case 'b':
				slice_push(&r, '\b')
			case 'f':
				slice_push(&r, '\f')
			case 'n':
				slice_push(&r, '\n')
			case 'r':
				slice_push(&r, '\r')
			case 't':
				slice_push(&r, '\t')
			case 'v':
				slice_push(&r, '\v')
			case '"':
				slice_push(&r, '"')
			case '\\':
				slice_push(&r, '\\')
			default:
				c.error_near_previous("invalid escape")
			}
		} else {
			slice_push(&r, cur)
		}
	}
	c.emit_value(yv_string(r))
}

func (c *compiler) parse_structure(can_assign bool) {
	if c.step_on(lx_lparen) {
		c.expr(false)
		c.expect(lx_rparen)
	} else {
		c.emit(op_nihil)
	}
	c.emit(op_structure)
	c.expect(lx_lbrace)
	c.struct_body()
}

func (c *compiler) struct_body() {
	var i float64 = 0
	if !c.check(lx_rbrace) {
		for {
			if c.step_on(lx_dot) {
				name := c.expect_name()
				c.emit_value(yv_string(name))
				if c.step_on(lx_equal) {
					c.expr(false)
				} else if c.check(lx_lparen) ||
					c.check(lx_lbrace) ||
					c.check(lx_equal_rangle) {
					c.named_function(name, false)
				} else {
					c.parse_name(false)
				}
				c.emit(op_define_key)
			} else if c.step_on(lx_minus_rangle) {
				name := c.expect_name()
				c.emit_value(yv_string(name))
				if !c.check(lx_lparen) &&
					!c.check(lx_lbrace) &&
					!c.check(lx_equal_rangle) {
					c.error_near_current("'{' expected")
				}
				c.named_function(name, true)
				c.emit(op_define_key)
			} else if c.step_on(lx_lbrack) {
				c.expr(true)
				c.expect(lx_rbrack)
				c.expect(lx_equal)
				c.expr(false)
				c.emit(op_define_key)
			} else {
				c.expr(false)
				if c.step_on(lx_dot_dot_dot) {
					c.emit(op_define_key_spread)
				} else {
					c.emit_value(yv_number(i))
					i++
					c.emit(op_swap, op_define_key)
				}
			}
			c.ignore_line()
			if !c.step_on(lx_comma) {
				break
			}
			if c.check(lx_rbrace) {
				break
			}
		}
	}
	c.expect(lx_rbrace)
}

func (c *compiler) parse_function(can_assign bool) {
	name := "@anonymous"
	if last := slice_last(c.locals); last != nil && !last.is_init {
		name = last.name
	}
	c.named_function(name, false)
}

func (c *compiler) named_function(name string, is_method bool) bool {
	fc := c.new_sub_compiler(name)
	if is_method {
		fc.fn.paramc++
		fc.declare_variable(name_self)
		slice_last(fc.locals).is_init = true
	}
	if fc.step_on(lx_lparen) {
		fc.param_list()
	}
	is_arrow := fc.step_on(lx_equal_rangle)
	if is_arrow {
		fc.expr(false)
		fc.emit(op_return)
	} else {
		fc.expect(lx_lbrace)
		fc.block()
		fc.emit_return()
	}
	c.emit_closure(fc.fn)
	return is_arrow
}

func (c *compiler) parse_prefix(can_assign bool) {
	op := c.previous.lx_type
	prefix_len := c.prefix.len
	if (op == lx_plus_plus || op == lx_minus_minus) &&
		c.prefix.len == bool_stack16_max {
		c.prefix.clear()
		c.error_near_previous("too many nested prefixes")
	}
	switch op {
	case lx_plus_plus:
		c.prefix.push(true)
	case lx_minus_minus:
		c.prefix.push(false)
	}
	c.precedence(prec_un)
	switch op {
	case lx_bang:
		c.emit(op_not)
	case lx_plus:
		c.emit(op_pos)
	case lx_minus:
		c.emit(op_neg)
	case lx_typeof:
		c.emit(op_typeof)
	case lx_plus_plus, lx_minus_minus:
		if prefix_len != c.prefix.len {
			c.prefix.clear()
			c.error_near_previous("invalid prefix")
		}
	default:
		panic(unreachable)
	}
}

func (c *compiler) parse_catch(can_assign bool) {
	catch_jump := c.emit_goto(op_begin_catch)
	c.precedence(prec_un)
	c.emit(op_end_catch)
	c.patch_goto(catch_jump)
}

func (c *compiler) led() parse_func {
	switch c.current.lx_type {
	case lx_comma:
		return c.parse_comma
	case lx_plus, lx_minus,
		lx_star, lx_slash, lx_percent,
		lx_equal_equal, lx_bang_equal,
		lx_langle, lx_langle_equal,
		lx_rangle, lx_rangle_equal,
		lx_pipe, lx_circum, lx_amper,
		lx_langle_langle, lx_rangle_rangle:
		return c.parse_infix
	case lx_pipe_pipe: // || or
		return c.parse_or
	case lx_amper_amper: // && and
		return c.parse_and
	case lx_quest_quest: // ??
		return c.parse_nihillish
	case lx_quest: // ? then
		return c.parse_then
	case lx_lparen: // (
		return c.parse_call
	case lx_lbrack: // [
		return c.parse_index
	case lx_dot: // .
		return c.parse_dot
	case lx_minus_rangle: // ->
		return c.parse_arrow
	default:
		panic(unreachable)
	}
}

func (c *compiler) parse_comma(can_assign bool) {
	c.emit(op_pop)
	c.expr(true)
}

func (c *compiler) parse_infix(can_assign bool) {
	lxt := c.previous.lx_type
	c.precedence(precedences[lxt] + 1)
	switch lxt {
	case lx_bang_equal:
		c.emit(op_eq, op_not)
	case lx_equal_equal:
		c.emit(op_eq)
	case lx_langle:
		c.emit(op_lt)
	case lx_langle_equal:
		c.emit(op_le)
	case lx_rangle:
		c.emit(op_le, op_not)
	case lx_rangle_equal:
		c.emit(op_lt, op_not)
	case lx_plus:
		c.emit(op_add)
	case lx_minus:
		c.emit(op_sub)
	case lx_star:
		c.emit(op_mul)
	case lx_slash:
		c.emit(op_div)
	case lx_percent:
		c.emit(op_mod)
	case lx_pipe:
		c.emit(op_or)
	case lx_circum:
		c.emit(op_xor)
	case lx_amper:
		c.emit(op_and)
	case lx_langle_langle:
		c.emit(op_lsh)
	case lx_rangle_rangle:
		c.emit(op_rsh)
	default:
		panic(unreachable)
	}
}

func (c *compiler) parse_or(can_assign bool) {
	left_jump := c.emit_goto(op_goto_if_false)
	right_jump := c.emit_goto(op_goto)
	c.patch_goto(left_jump)
	c.emit(op_pop)
	c.precedence(prec_lor)
	c.patch_goto(right_jump)
}

func (c *compiler) parse_and(can_assign bool) {
	end_jump := c.emit_goto(op_goto_if_false)
	c.emit(op_pop)
	c.precedence(prec_land)
	c.patch_goto(end_jump)
}

func (c *compiler) parse_nihillish(can_assign bool) {
	c.nihillish(prec_lor)
}

func (c *compiler) nihillish(prec precedence) {
	left_jump := c.emit_goto(op_goto_if_nihil)
	right_jump := c.emit_goto(op_goto)
	c.patch_goto(left_jump)
	c.emit(op_pop)
	c.precedence(prec)
	c.patch_goto(right_jump)
}

func (c *compiler) parse_then(can_assign bool) {
	then_jump := c.emit_goto(op_goto_if_false)
	c.emit(op_pop)
	c.expr(true)
	else_jump := c.emit_goto(op_goto)
	if !c.step_on(lx_else) {
		c.expect(lx_colon)
	}
	c.patch_goto(then_jump)
	c.emit(op_pop)
	c.expr(false)
	c.patch_goto(else_jump)
}

func (c *compiler) parse_call(can_assign bool) {
	argc, is_spread := c.arg_list()
	var op = op_call
	if is_spread {
		op = op_call_spread
	}
	c.emit(op, uint8(argc))
}

func (c *compiler) parse_index(can_assign bool) {
	c.expr(true)
	c.expect(lx_rbrack)
	c.assign(
		func() { c.emit(op_store_key) },
		func() { c.emit(op_load_key) },
		func() { c.emit(op_dup2, op_load_key) },
		can_assign,
	)
}

func (c *compiler) parse_dot(can_assign bool) {
	c.expect(lx_name)
	c.emit_value(yv_string(c.previous.literal))
	c.assign(
		func() { c.emit(op_store_key) },
		func() { c.emit(op_load_key) },
		func() { c.emit(op_dup2, op_load_key) },
		can_assign,
	)
}

func (c *compiler) parse_arrow(can_assign bool) {
	c.emit(op_dup)
	for {
		c.emit_value(yv_string(c.expect_name()))
		c.emit(op_load_key)
		if !c.step_on(lx_minus_rangle) {
			c.emit(op_swap)
			break
		}
	}
	c.expect(lx_lparen)
	argc, is_spread := c.arg_list()
	if is_spread {
		c.emit(op_call_spread, argc+1)
	} else {
		c.emit(op_call, argc+1)
	}
}

func (c *compiler) declare_variable(name string) {
	for i := len(c.locals) - 1; i >= 0; i-- {
		if c.locals[i].scope < c.scope {
			break
		}
		if c.locals[i].name == name {
			c.error_near_previous("variable already declared")
		}
	}
	c.add_local(name)
}

func (c *compiler) define_variables() {
	for i := len(c.locals) - 1; i >= 0; i-- {
		if local := &c.locals[i]; !local.is_init {
			local.is_init = true
		} else {
			break
		}
	}
}

func (c *compiler) add_local(name string) {
	if len(c.locals) > math.MaxUint8 {
		c.error_near_previous("too many local variables")
	}
	slice_push(&c.locals, local{
		name:  name,
		scope: c.scope,
	})
}

func (c *compiler) emit(ops ...op_code) {
	for _, op := range ops {
		c.fn.write_code(op, c.previous.line)
	}
}

func (c *compiler) emit_value(v yv_value) {
	i := c.fn.add_value(v)
	if i > math.MaxUint8 {
		c.error_near_previous("too many local values")
	}
	c.emit(op_value, uint8(i))
}

func (c *compiler) emit_closure(f *fn_proto) {
	i := c.fn.add_fn(f)
	if i > math.MaxUint8 {
		c.error_near_previous("too many local functions")
	}
	c.emit(op_closure, uint8(i))
}

func (c *compiler) emit_return() {
	c.emit(op_nihil, op_return)
}

func (c *compiler) emit_goto(op op_code) int {
	c.emit(op, 0xff, 0xff)
	return len(c.fn.code)
}

func (c *compiler) patch_goto(start int) {
	jump := len(c.fn.code) - start
	if jump > math.MaxInt16 {
		c.error_near_previous("too long jump")
	}
	c.fn.code[start-2], c.fn.code[start-1] = u16tou8(uint16(jump))
}

func (c *compiler) emit_goto_back(start int) {
	c.emit(op_goto)
	jump := start - len(c.fn.code) - 2
	if jump < math.MinInt16 {
		c.error_near_previous("too long jump")
	}
	c.emit(u16tou8(uint16(jump)))
}

func (c *compiler) begin_scope() { c.scope++ }

func (c *compiler) end_scope() {
	c.scope--
	c.close_locals(c.scope, true)
}

func (c *compiler) end_loop_scopes(l *loop) {
	c.close_locals(l.scope, false)
}

func (c *compiler) close_locals(until int, cut bool) {
	for i := len(c.locals) - 1; i >= 0; i-- {
		if c.locals[i].scope > until {
			if c.locals[i].is_upval {
				c.emit(op_close_upvalue)
			} else {
				c.emit(op_pop)
			}
			if cut {
				slice_pop(&c.locals)
			}
		} else {
			break
		}
	}
}

func (c *compiler) begin_loop(label string) {
	c.loop = &loop{
		label:     label,
		scope:     c.scope,
		start:     len(c.fn.code),
		enclosing: c.loop,
	}
}

func (c *compiler) end_loop() {
	for _, b := range c.loop.breaks {
		c.patch_goto(b)
	}
	c.loop = c.loop.enclosing
}

func (c *compiler) param_list() {
	if !c.check(lx_rparen) {
		for {
			if c.step_on(lx_dot_dot_dot) {
				c.fn.vararg = true
			} else {
				c.fn.paramc++
			}
			c.declare_variable(c.expect_name())
			slice_last(c.locals).is_init = true
			if !c.fn.vararg && c.step_on(lx_equal) {
				c.emit(op_load_local, uint8(len(c.locals)-1))
				c.nihillish(prec_assign)
				c.emit(op_store_local, uint8(len(c.locals)-1), op_pop)
			}
			c.ignore_line()
			if !c.step_on(lx_comma) {
				break
			}
			if c.check(lx_rparen) || c.fn.vararg {
				break
			}
		}
	}
	c.expect(lx_rparen)
}

func (c *compiler) arg_list() (uint8, bool) {
	var argc int = 0
	is_spread := false
	if !c.check(lx_rparen) {
		for {
			if argc+1 > math.MaxUint8 {
				c.error_near_previous("too many arguments")
			}
			c.expr(false)
			if c.step_on(lx_dot_dot_dot) {
				is_spread = true
			} else {
				argc++
			}
			c.ignore_line()
			if !c.step_on(lx_comma) {
				break
			}
			if c.check(lx_rparen) || is_spread {
				break
			}
		}
	}
	c.expect(lx_rparen)
	return uint8(argc), is_spread
}

func (c *compiler) assign(set, get, getnp func(), can_assign bool) {
	if c.prefix.len != 0 && precedences[c.current.lx_type] <= prec_un {
		getnp()
		c.emit_value(yv_number(1))
		if c.prefix.pop() {
			c.emit(op_add)
		} else {
			c.emit(op_sub)
		}
		set()
	} else if c.step_on(lx_plus_plus) {
		getnp()
		c.emit(op_copy_to)
		c.emit_value(yv_number(1))
		c.emit(op_add)
		set()
		c.emit(op_copy_from)
	} else if c.step_on(lx_minus_minus) {
		getnp()
		c.emit(op_copy_to)
		c.emit_value(yv_number(1))
		c.emit(op_sub)
		set()
		c.emit(op_copy_from)
	} else if can_assign {
		switch {
		case c.step_on(lx_equal):
			c.expr(false)
			set()
		case map_has(equal_ops, c.current.lx_type):
			op := equal_ops[c.current.lx_type]
			c.step()
			getnp()
			c.expr(false)
			c.emit(op)
			set()
		case c.step_on(lx_pipe_pipe_equal):
			getnp()
			c.parse_or(false)
			set()
		case c.step_on(lx_amper_amper_equal):
			getnp()
			c.parse_and(false)
			set()
		case c.step_on(lx_quest_quest_equal):
			getnp()
			c.parse_nihillish(false)
			set()
		default:
			get()
		}
	} else {
		get()
	}
}

var equal_ops = map[lx_type]op_code{
	lx_plus_equal:          op_add,
	lx_minus_equal:         op_sub,
	lx_star_equal:          op_mul,
	lx_slash_equal:         op_div,
	lx_percent_equal:       op_mod,
	lx_pipe_equal:          op_or,
	lx_circum_equal:        op_xor,
	lx_amper_equal:         op_and,
	lx_langle_langle_equal: op_lsh,
	lx_rangle_rangle_equal: op_rsh,
}

type local struct {
	name  string
	scope int

	is_init  bool
	is_upval bool
}

type loop struct {
	scope     int
	start     int
	breaks    []int
	enclosing *loop
	label     string
}

type parse_func func(can_assign bool)

type parse_error empty

type parser struct {
	lexer lexer

	next     lexeme
	current  lexeme
	previous lexeme

	had_error bool
}

func new_parser(src []byte) *parser {
	p := &parser{lexer: new_lexer(src)}
	p.step()
	p.step()
	return p
}

func (p *parser) step() {
	p.previous = p.current
	p.current = p.next
	for {
		p.next = p.lexer.lex()
		if p.next.lx_type != lx_error {
			break
		}
		p.error_near(&p.next, "%s", "")
	}
}

func (p *parser) check(t lx_type) bool {
	return p.current.lx_type == t
}

func (p *parser) check_next(t lx_type) bool {
	return p.next.lx_type == t
}

func (p *parser) step_on(t lx_type) bool {
	if !p.check(t) {
		return false
	}
	p.step()
	return true
}

func (p *parser) match_semi() bool {
	if mode_autosemi {
		return p.step_on(lx_line) ||
			p.step_on(lx_semi) ||
			p.step_on(lx_eof)
	} else {
		return p.step_on(lx_semi)
	}
}

func (p *parser) expect(t lx_type) {
	if p.current.lx_type == t {
		p.step()
		return
	}
	p.error_near_current("'%s' expected", t)
}

func (p *parser) expect_name() string {
	p.expect(lx_name)
	return p.previous.literal
}

func (p *parser) expect_semi() {
	if mode_autosemi {
		if p.step_on(lx_line) || p.step_on(lx_eof) || p.check(lx_rbrace) {
			return
		}
	}
	p.expect(lx_semi)
}

func (p *parser) ignore_line() { p.step_on(lx_line) }

func (p *parser) error_near_previous(format string, a ...any) {
	p.error_near(&p.previous, format, a...)
}

func (p *parser) error_near_current(format string, a ...any) {
	p.error_near(&p.current, format, a...)
}

func (p *parser) error_near(lx *lexeme, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if !p.had_error {
		fmt.Fprint(os.Stderr, "parse error: ")
	} else {
		fmt.Fprint(os.Stderr, "\talso: ")
	}
	fmt.Fprintf(os.Stderr, "ln %d: %s", lx.line, msg)
	switch lx.lx_type {
	case lx_eof, lx_line:
		fmt.Fprintf(os.Stderr, " near end")
	case lx_error:
		/* pass */
	default:
		fmt.Fprintf(os.Stderr, " near '%s'", lx.literal)
	}
	fmt.Fprintln(os.Stderr)
	p.had_error = true
	panic(parse_error{})
}

func (p *parser) sync() {
	for p.current.lx_type != lx_eof {
		p.step()
		if p.previous.lx_type == lx_semi ||
			p.previous.lx_type == lx_line {
			return
		}
		if _, ok := sync_lexemes[p.current.lx_type]; ok {
			return
		}
	}
}

var sync_lexemes = map[lx_type]empty{
	lx_variable: {},
	lx_if:       {},
	lx_while:    {},
	lx_do:       {},
	lx_for:      {},
	lx_break:    {},
	lx_continue: {},
	lx_return:   {},
	lx_throw:    {},
}

type precedence int

const (
	prec_low precedence = iota

	prec_comma  // ,
	prec_assign // =
	prec_tern   // ? : then else
	prec_lor    // || or ??
	prec_land   // && and
	prec_or     // |
	prec_xor    // ^
	prec_and    // &
	prec_eq     // == != ~~ !~
	prec_comp   // < > <= >=
	prec_shift  // << >>
	prec_term   // + -
	prec_fact   // * / %
	prec_un     // ! not + - ~ ++ -- typeof catch
	prec_call   // . ?. () ?[ [] -> :: ++ --

	prec_high
)

var precedences = map[lx_type]precedence{
	lx_comma: prec_comma, // ,

	lx_quest: prec_tern, // ? :

	lx_pipe_pipe:   prec_lor, // ||
	lx_quest_quest: prec_lor, // ??

	lx_amper_amper: prec_land, // &&

	lx_pipe:   prec_or,  // |
	lx_circum: prec_xor, // ^
	lx_amper:  prec_and, // &

	lx_equal_equal: prec_eq, // ==
	lx_bang_equal:  prec_eq, // !=

	lx_langle:       prec_comp, // <
	lx_rangle:       prec_comp, // >
	lx_langle_equal: prec_comp, // <=
	lx_rangle_equal: prec_comp, // >=

	lx_langle_langle: prec_shift, // <<
	lx_rangle_rangle: prec_shift, // >>

	lx_plus:  prec_term, // +
	lx_minus: prec_term, // -

	lx_star:    prec_fact, // *
	lx_slash:   prec_fact, // /
	lx_percent: prec_fact, // %

	lx_lparen:       prec_call, // ()
	lx_lbrack:       prec_call, // []
	lx_dot:          prec_call, // .
	lx_minus_rangle: prec_call, // ->
}
