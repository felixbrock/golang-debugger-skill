// Package rpncalc is a tiny RPN calculator: parse a space-separated
// expression, then evaluate it.
package rpncalc

import (
	"strconv"
	"strings"
)

type kind int

const (
	num kind = iota
	add
	sub
	mul
	div
)

type tok struct {
	kind kind
	val  int64
}

func parse(src string) []tok {
	var toks []tok
	for _, w := range strings.Fields(src) {
		switch w {
		case "+":
			toks = append(toks, tok{kind: add})
		case "-":
			toks = append(toks, tok{kind: sub})
		case "*":
			toks = append(toks, tok{kind: mul})
		case "/":
			toks = append(toks, tok{kind: div})
		default:
			n, err := strconv.ParseInt(w, 10, 64)
			if err != nil {
				panic("number: " + w)
			}
			toks = append(toks, tok{kind: num, val: n})
		}
	}
	return toks
}

func eval(tokens []tok) int64 {
	var stack []int64
	pop := func() int64 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return v
	}
	for _, t := range tokens {
		switch t.kind {
		case num:
			stack = append(stack, t.val)
		case add:
			b := pop()
			a := pop()
			stack = append(stack, a+b)
		case sub:
			b := pop()
			a := pop()
			stack = append(stack, b-a)
		case mul:
			b := pop()
			a := pop()
			stack = append(stack, a*b)
		case div:
			b := pop()
			a := pop()
			stack = append(stack, b/a)
		}
	}
	return pop()
}

// Calc evaluates an RPN expression such as "10 3 - 2 * 1 +".
func Calc(src string) int64 {
	return eval(parse(src))
}
