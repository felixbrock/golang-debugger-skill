package daemon

import (
	"reflect"
	"strings"
	"testing"

	"github.com/felixbrock/golang-debugger-skill/internal/dlv"
)

func v(name, typ string, kind reflect.Kind, value string, children ...dlv.Variable) dlv.Variable {
	return dlv.Variable{Name: name, Type: typ, Kind: uint(kind), Value: value,
		Len: int64(len(children)), Children: children}
}

func TestInlineVar(t *testing.T) {
	item := v("it", "main.Item", reflect.Struct, "",
		v("Name", "string", reflect.String, "apple"),
		v("Qty", "int", reflect.Int, "3"))
	got := inlineVar(item, 200)
	want := `main.Item{Name: "apple", Qty: 3}`
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}

	slice := v("items", "[]main.Item", reflect.Slice, "", item)
	if got := inlineVar(slice, 200); got != `[main.Item{Name: "apple", Qty: 3}]` {
		t.Errorf("slice: %q", got)
	}

	if got := inlineVar(v("s", "string", reflect.String, "hi"), 200); got != `"hi"` {
		t.Errorf("string: %q", got)
	}

	long := inlineVar(item, 10)
	if n := len([]rune(long)); n > 10 {
		t.Errorf("not capped: %q (%d runes)", long, n)
	}
}

func TestRenderVarsTree(t *testing.T) {
	item := v("it", "main.Item", reflect.Struct, "",
		v("Name", "string", reflect.String, "apple"),
		v("Qty", "int", reflect.Int, "3"))
	out := renderVars([]dlv.Variable{item})
	for _, want := range []string{"it: main.Item", `Name: string = "apple"`, "Qty: int = 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderMap(t *testing.T) {
	m := dlv.Variable{Name: "m", Type: "map[string]int", Kind: uint(reflect.Map), Len: 1,
		Children: []dlv.Variable{
			v("", "string", reflect.String, "a"),
			v("", "int", reflect.Int, "1"),
		}}
	if got := inlineVar(m, 200); got != `map["a": 1]` {
		t.Errorf("map: %q", got)
	}
}

func TestNilPointer(t *testing.T) {
	p := dlv.Variable{Name: "p", Type: "*main.Item", Kind: uint(reflect.Ptr), Value: "0"}
	if got := inlineVar(p, 200); got != "nil" && got != "0" {
		t.Errorf("nil ptr: %q", got)
	}
}
