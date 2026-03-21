package yeva

import (
	"fmt"
	"slices"

	_ "embed"
)

type empty struct{}

type version struct {
	Major int
	Minor int
	Patch int
}

func (v version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

const (
	nul = '\x00'

	mode_autosemi bool = false

	dbg_lex  = false
	dbg_code = false
	dbg_exe  = false

	// dbg_lex  = true
	// dbg_code = true
	// dbg_exe  = true

	unreachable = "unreachable"

	struct_cap = 32

	nihil_literal    = "void"
	variable_literal = "var"
	function_literal = "function"

	key_result  yv_string = "result"
	key_catched yv_string = "catched"

	name_self = "this"

	rte_not_defined yv_string = "variable is not defined"
)

//go:embed embed/embed.yv
var embed []byte

func trim(s string, l int) string {
	r := []rune(s)
	if len(r) <= l {
		return s
	}
	return string(r[:l])
}

func cover(s string, w int, c rune) string {
	cvr := w - (len(s) + 2)
	if cvr < 0 {
		return s
	}
	l := mul_rune(c, cvr/2)
	r := mul_rune(c, cvr/2+cvr%2)
	return fmt.Sprintf("%s %s %s", l, s, r)
}

func mul_rune(r rune, c int) string {
	rs := make([]rune, c)
	for i := range rs {
		rs[i] = r
	}
	return string(rs)
}

func btoi(b bool) int {
	if b {
		return 1
	} else {
		return 0
	}
}

func itob(i int) bool {
	return i != 0
}

func catch[E any](on_catch func(E)) {
	if p := recover(); p != nil {
		if pe, ok := p.(E); ok {
			on_catch(pe)
		} else {
			panic(p)
		}
	}
}

func zero[T any]() (t T) { return }

func is[T yv_value](v yv_value) bool {
	_, ok := v.(T)
	return ok
}

func assert2[T yv_value](v1, v2 yv_value) (T, T, bool) {
	v1t, ok1 := v1.(T)
	v2t, ok2 := v2.(T)
	if !ok1 || !ok2 {
		return zero[T](), zero[T](), false
	}
	return v1t, v2t, true
}

func is_index(v yv_value) (int, bool) {
	num, ok := v.(yv_number)
	if !ok {
		return 0, false
	}
	idx := int(num)
	if num != yv_number(idx) {
		return 0, false
	}
	if idx < 0 {
		return 0, false
	}
	return idx, true
}

func map_has[K comparable, V any](m map[K]V, k K) bool {
	_, ok := m[k]
	return ok
}

func slice_push[T any](s *[]T, v ...T) {
	*s = append(*s, v...)
}

func slice_pop[T any](s *[]T) (t T) {
	v := (*s)[len(*s)-1]
	*s = (*s)[:len(*s)-1]
	return v
}

func slice_cut[T any](s *[]T, i int) {
	if i < 0 {
		(*s) = (*s)[:len(*s)+i]
	} else {
		(*s) = (*s)[i:len(*s)]
	}
}

func slice_last[T any](s []T) (t *T) {
	if len(s) == 0 {
		return
	}
	return &s[len(s)-1]
}

const bool_stack16_max = 16

type bool_stack16 struct {
	len  int16
	data int16
}

func (bs *bool_stack16) push(b bool) {
	if bs.len == 16 {
		panic("bool stack overflow")
	}
	var mask int16 = 1 << bs.len
	bs.len++
	bs.data |= int16(btoi(b)) * mask
}

func (bs *bool_stack16) pop() bool {
	if bs.len == 0 {
		panic("bool stack underflow")
	}
	bs.len--
	var mask int16 = 1 << bs.len
	b := (bs.data & mask) != 0
	bs.data ^= mask
	return b
}

func (bs *bool_stack16) clear() { bs.len = 0 }

func u16tou8(u uint16) (b, s uint8) {
	return uint8((u >> 8) & 0xff), uint8(u & 0xff)
}

func u8tou16(b, s uint8) uint16 {
	return uint16(b)<<8 | uint16(s)
}

func u32tou8(u uint32) (b, mb, ms, s uint8) {
	return uint8((u >> 24) & 0xff),
		uint8((u >> 16) & 0xff),
		uint8((u >> 8) & 0xff),
		uint8(u & 0xff)
}

func u8tou32(b, mb, ms, s uint8) uint32 {
	return uint32(b)<<24 |
		uint32(mb)<<16 |
		uint32(ms)<<8 |
		uint32(s)
}

func encode(n uint) []uint8 {
	if n == 0 {
		return []uint8{0}
	}
	var bs []uint8
	for n > 0 {
		bs = append(bs, uint8(n&0x7f))
		n >>= 7
	}
	slices.Reverse(bs)
	for i := range len(bs) - 1 {
		bs[i] |= 1 << 7
	}
	return bs
}

func decode(b []byte) (int, int) {
	r := 0
	for i := range b {
		r |= int(b[i] & 0x7f)
		if b[i]&(1<<7) == 0 {
			return r, i + 1
		}
		r <<= 7
	}
	return r, len(b)
}
