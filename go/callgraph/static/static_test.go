// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package static_test

import (
	"fmt"
	"go/parser"
	"reflect"
	"sort"
	"testing"

	"github.com/TBD54566975/golang-tools/go/callgraph"
	"github.com/TBD54566975/golang-tools/go/callgraph/static"
	"github.com/TBD54566975/golang-tools/go/loader"
	"github.com/TBD54566975/golang-tools/go/ssa"
	"github.com/TBD54566975/golang-tools/go/ssa/ssautil"
)

const input = `package P

type C int
func (C) f()

type I interface{f()}

func f() {
	p := func() {}
	g()
	p() // SSA constant propagation => static

	if unknown {
		p = h
	}
	p() // dynamic

	C(0).f()
}

func g() {
	var i I = C(0)
	i.f()
}

func h()

var unknown bool
`

const genericsInput = `package P

type I interface {
	F()
}

type A struct{}

func (a A) F() {}

type B struct{}

func (b B) F() {}

func instantiated[X I](x X) {
	x.F()
}

func Bar() {}

func f(h func(), a A, b B) {
	h()

	instantiated[A](a)
	instantiated[B](b)
}
`

func TestStatic(t *testing.T) {
	for _, e := range []struct {
		input string
		want  []string
		// typeparams must be true if input uses type parameters
		typeparams bool
	}{
		{input, []string{
			"(*C).f -> (C).f",
			"f -> (C).f",
			"f -> f$1",
			"f -> g",
		}, false},
		{genericsInput, []string{
			"(*A).F -> (A).F",
			"(*B).F -> (B).F",
			"f -> instantiated[P.A]",
			"f -> instantiated[P.B]",
			"instantiated[P.A] -> (A).F",
			"instantiated[P.B] -> (B).F",
		}, true},
	} {
		conf := loader.Config{ParserMode: parser.ParseComments}
		f, err := conf.ParseFile("P.go", e.input)
		if err != nil {
			t.Error(err)
			continue
		}

		conf.CreateFromFiles("P", f)
		iprog, err := conf.Load()
		if err != nil {
			t.Error(err)
			continue
		}

		P := iprog.Created[0].Pkg

		prog := ssautil.CreateProgram(iprog, ssa.InstantiateGenerics)
		prog.Build()

		cg := static.CallGraph(prog)

		var edges []string
		callgraph.GraphVisitEdges(cg, func(e *callgraph.Edge) error {
			edges = append(edges, fmt.Sprintf("%s -> %s",
				e.Caller.Func.RelString(P),
				e.Callee.Func.RelString(P)))
			return nil
		})
		sort.Strings(edges)

		if !reflect.DeepEqual(edges, e.want) {
			t.Errorf("Got edges %v, want %v", edges, e.want)
		}
	}
}
