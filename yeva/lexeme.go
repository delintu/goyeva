package yeva

import "fmt"

type lx_type string

const (
	lx_lparen lx_type = "("
	lx_rparen lx_type = ")"
	lx_lbrace lx_type = "{"
	lx_rbrace lx_type = "}"
	lx_lbrack lx_type = "["
	lx_rbrack lx_type = "]"

	lx_langle       lx_type = "<"
	lx_rangle       lx_type = ">"
	lx_langle_equal lx_type = "<="
	lx_rangle_equal lx_type = ">="

	lx_langle_langle       lx_type = "<<"
	lx_rangle_rangle       lx_type = ">>"
	lx_langle_langle_equal lx_type = "<<="
	lx_rangle_rangle_equal lx_type = ">>="

	lx_semi  lx_type = ";"
	lx_equal lx_type = "="
	lx_bang  lx_type = "!"
	lx_dot   lx_type = "."
	lx_comma lx_type = ","
	lx_quest lx_type = "?"
	lx_colon lx_type = ":"
	lx_tilde lx_type = "~"

	lx_plus_plus   lx_type = "++"
	lx_minus_minus lx_type = "--"

	lx_minus_rangle lx_type = "->"
	lx_equal_rangle lx_type = "=>"

	lx_plus    lx_type = "+"
	lx_minus   lx_type = "-"
	lx_star    lx_type = "*"
	lx_slash   lx_type = "/"
	lx_percent lx_type = "%"
	lx_pipe    lx_type = "|"
	lx_circum  lx_type = "^"
	lx_amper   lx_type = "&"

	lx_plus_equal    lx_type = "+="
	lx_minus_equal   lx_type = "-="
	lx_star_equal    lx_type = "*="
	lx_slash_equal   lx_type = "/="
	lx_percent_equal lx_type = "%="
	lx_pipe_equal    lx_type = "|="
	lx_circum_equal  lx_type = "^="
	lx_amper_equal   lx_type = "&="

	lx_equal_equal lx_type = "=="
	lx_bang_equal  lx_type = "!="

	lx_pipe_pipe   lx_type = "||"
	lx_amper_amper lx_type = "&&"
	lx_quest_quest lx_type = "??"

	lx_pipe_pipe_equal   lx_type = "||="
	lx_amper_amper_equal lx_type = "&&="
	lx_quest_quest_equal lx_type = "??="

	lx_dot_dot_dot lx_type = "..."

	lx_name   lx_type = "name"
	lx_number lx_type = "number"
	lx_string lx_type = "string"

	lx_variable lx_type = "variable"
	lx_constant lx_type = "constant"
	lx_function lx_type = "function"

	lx_nihil    lx_type = "nihil"
	lx_false    lx_type = "false"
	lx_true     lx_type = "true"
	lx_if       lx_type = "if"
	lx_else     lx_type = "else"
	lx_while    lx_type = "while"
	lx_do       lx_type = "do"
	lx_for      lx_type = "for"
	lx_break    lx_type = "break"
	lx_continue lx_type = "continue"
	lx_return   lx_type = "return"
	lx_catch    lx_type = "catch"
	lx_throw    lx_type = "throw"
	lx_typeof   lx_type = "typeof"
	lx_struct   lx_type = "struct"

	lx_line lx_type = "line"

	lx_error lx_type = "__error"
	lx_eof   lx_type = "__eof"
)

type lexeme struct {
	lx_type lx_type
	// start   int
	// length  int
	line    int
	literal string
}

// func (l lexeme) log(src []byte) string {
// 	lit := string(src[l.start : l.start+l.length])
// 	if lit != "" {
// 		lit = "> " + short_string(lit, 32)
// 	}
// 	return fmt.Sprintf("%04d   | %-20s |%s", l.line, l.lx_type, lit)
// }

func (l lexeme) String() string {
	var lit string
	if l.literal != "" {
		lit = "> " + short_string(l.literal, 32)
	}
	return fmt.Sprintf("%04d   | %-20s |%s", l.line, l.lx_type, lit)
}
