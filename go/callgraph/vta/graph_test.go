// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vta

import (
	"fmt"
	"go/types"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/block/ftl-golang-tools/go/callgraph/cha"
	"github.com/block/ftl-golang-tools/go/ssa"
	"github.com/block/ftl-golang-tools/go/ssa/ssautil"
	"github.com/block/ftl-golang-tools/internal/aliases"
)

func TestNodeInterface(t *testing.T) {
	// Since ssa package does not allow explicit creation of ssa
	// values, we use the values from the program testdata/src/simple.go:
	//   - basic type int
	//   - struct X with two int fields a and b
	//   - global variable "gl"
	//   - "main" function and its
	//   - first register instruction t0 := *gl
	prog, _, err := testProg("testdata/src/simple.go", ssa.BuilderMode(0))
	if err != nil {
		t.Fatalf("couldn't load testdata/src/simple.go program: %v", err)
	}

	pkg := prog.AllPackages()[0]
	main := pkg.Func("main")
	reg := firstRegInstr(main) // t0 := *gl
	X := pkg.Type("X").Type()
	gl := pkg.Var("gl")
	glPtrType, ok := aliases.Unalias(gl.Type()).(*types.Pointer)
	if !ok {
		t.Fatalf("could not cast gl variable to pointer type")
	}
	bint := glPtrType.Elem()

	pint := types.NewPointer(bint)
	i := types.NewInterface(nil, nil)

	voidFunc := main.Signature.Underlying()

	for _, test := range []struct {
		n node
		s string
		t types.Type
	}{
		{constant{typ: bint}, "Constant(int)", bint},
		{pointer{typ: pint}, "Pointer(*int)", pint},
		{mapKey{typ: bint}, "MapKey(int)", bint},
		{mapValue{typ: pint}, "MapValue(*int)", pint},
		{sliceElem{typ: bint}, "Slice([]int)", bint},
		{channelElem{typ: pint}, "Channel(chan *int)", pint},
		{field{StructType: X, index: 0}, "Field(testdata.X:a)", bint},
		{field{StructType: X, index: 1}, "Field(testdata.X:b)", bint},
		{global{val: gl}, "Global(gl)", gl.Type()},
		{local{val: reg}, "Local(t0)", bint},
		{indexedLocal{val: reg, typ: X, index: 0}, "Local(t0[0])", X},
		{function{f: main}, "Function(main)", voidFunc},
		{nestedPtrInterface{typ: i}, "PtrInterface(interface{})", i},
		{nestedPtrFunction{typ: voidFunc}, "PtrFunction(func())", voidFunc},
		{panicArg{}, "Panic", nil},
		{recoverReturn{}, "Recover", nil},
	} {
		if test.s != test.n.String() {
			t.Errorf("want %s; got %s", test.s, test.n.String())
		}
		if test.t != test.n.Type() {
			t.Errorf("want %s; got %s", test.t, test.n.Type())
		}
	}
}

func TestVtaGraph(t *testing.T) {
	// Get the basic type int from a real program.
	prog, _, err := testProg("testdata/src/simple.go", ssa.BuilderMode(0))
	if err != nil {
		t.Fatalf("couldn't load testdata/src/simple.go program: %v", err)
	}

	glPtrType, ok := prog.AllPackages()[0].Var("gl").Type().(*types.Pointer)
	if !ok {
		t.Fatalf("could not cast gl variable to pointer type")
	}
	bint := glPtrType.Elem()

	n1 := constant{typ: bint}
	n2 := pointer{typ: types.NewPointer(bint)}
	n3 := mapKey{typ: types.NewMap(bint, bint)}
	n4 := mapValue{typ: types.NewMap(bint, bint)}

	// Create graph
	//   n1   n2
	//    \  / /
	//     n3 /
	//     | /
	//     n4
	g := make(vtaGraph)
	g.addEdge(n1, n3)
	g.addEdge(n2, n3)
	g.addEdge(n3, n4)
	g.addEdge(n2, n4)
	// for checking duplicates
	g.addEdge(n1, n3)

	want := vtaGraph{
		n1: map[node]bool{n3: true},
		n2: map[node]bool{n3: true, n4: true},
		n3: map[node]bool{n4: true},
	}

	if !reflect.DeepEqual(want, g) {
		t.Errorf("want %v; got %v", want, g)
	}

	for _, test := range []struct {
		n node
		l int
	}{
		{n1, 1},
		{n2, 2},
		{n3, 1},
		{n4, 0},
	} {
		if sl := len(g[test.n]); sl != test.l {
			t.Errorf("want %d successors; got %d", test.l, sl)
		}
	}
}

// vtaGraphStr stringifies vtaGraph into a list of strings
// where each string represents an edge set of the format
// node -> succ_1, ..., succ_n. succ_1, ..., succ_n are
// sorted in alphabetical order.
func vtaGraphStr(g vtaGraph) []string {
	var vgs []string
	for n, succ := range g {
		var succStr []string
		for s := range succ {
			succStr = append(succStr, s.String())
		}
		sort.Strings(succStr)
		entry := fmt.Sprintf("%v -> %v", n.String(), strings.Join(succStr, ", "))
		vgs = append(vgs, entry)
	}
	return vgs
}

// setdiff returns the set difference of `X-Y` or {s | s ∈ X, s ∉ Y }.
func setdiff(X, Y []string) []string {
	y := make(map[string]bool)
	var delta []string
	for _, s := range Y {
		y[s] = true
	}

	for _, s := range X {
		if _, ok := y[s]; !ok {
			delta = append(delta, s)
		}
	}
	sort.Strings(delta)
	return delta
}

func TestVTAGraphConstruction(t *testing.T) {
	for _, file := range []string{
		"testdata/src/store.go",
		"testdata/src/phi.go",
		"testdata/src/type_conversions.go",
		"testdata/src/type_assertions.go",
		"testdata/src/fields.go",
		"testdata/src/node_uniqueness.go",
		"testdata/src/store_load_alias.go",
		"testdata/src/phi_alias.go",
		"testdata/src/channels.go",
		"testdata/src/generic_channels.go",
		"testdata/src/select.go",
		"testdata/src/stores_arrays.go",
		"testdata/src/maps.go",
		"testdata/src/ranges.go",
		"testdata/src/closures.go",
		"testdata/src/function_alias.go",
		"testdata/src/static_calls.go",
		"testdata/src/dynamic_calls.go",
		"testdata/src/returns.go",
		"testdata/src/panic.go",
	} {
		t.Run(file, func(t *testing.T) {
			prog, want, err := testProg(file, ssa.BuilderMode(0))
			if err != nil {
				t.Fatalf("couldn't load test file '%s': %s", file, err)
			}
			if len(want) == 0 {
				t.Fatalf("couldn't find want in `%s`", file)
			}

			g, _ := typePropGraph(ssautil.AllFunctions(prog), cha.CallGraph(prog))
			got := vtaGraphStr(g)
			if diff := setdiff(want, got); len(diff) > 0 {
				t.Errorf("`%s`: want superset of %v;\n got %v\ndiff: %v", file, want, got, diff)
			}
		})
	}
}
