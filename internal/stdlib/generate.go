// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore
// +build ignore

// The generate command reads all the GOROOT/api/go1.*.txt files and
// generates a single combined manifest.go file containing the Go
// standard library API symbols along with versions.
package main

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/format"
	"go/types"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/block/ftl-golang-tools/go/packages"
)

func main() {
	pkgs := make(map[string]map[string]symInfo) // package -> symbol -> info
	symRE := regexp.MustCompile(`^pkg (\S+).*?, (var|func|type|const|method \([^)]*\)) ([\pL\p{Nd}_]+)(.*)`)

	// parse parses symbols out of GOROOT/api/*.txt data, with the specified minor version.
	// Errors are reported against filename.
	parse := func(filename string, data []byte, minor int) {
		for linenum, line := range strings.Split(string(data), "\n") {
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			m := symRE.FindStringSubmatch(line)
			if m == nil {
				log.Fatalf("invalid input: %s:%d: %s", filename, linenum+1, line)
			}
			path, kind, sym, rest := m[1], m[2], m[3], m[4]

			if _, recv, ok := strings.Cut(kind, "method "); ok {
				// e.g. "method (*Func) Pos() token.Pos"
				kind = "method"

				recv := removeTypeParam(recv) // (*Foo[T]) -> (*Foo)

				sym = recv + "." + sym // (*T).m

			} else if _, field, ok := strings.Cut(rest, " struct, "); ok && kind == "type" {
				// e.g. "type ParenExpr struct, Lparen token.Pos"
				kind = "field"
				name, typ, _ := strings.Cut(field, " ")

				// The api script uses the name
				// "embedded" (ambiguously) for
				// the name of an anonymous field.
				if name == "embedded" {
					// Strip "*pkg.T" down to "T".
					typ = strings.TrimPrefix(typ, "*")
					if _, after, ok := strings.Cut(typ, "."); ok {
						typ = after
					}
					typ = removeTypeParam(typ) // embedded Foo[T] -> Foo
					name = typ
				}

				sym += "." + name // T.f
			}

			symbols, ok := pkgs[path]
			if !ok {
				symbols = make(map[string]symInfo)
				pkgs[path] = symbols
			}

			// Don't overwrite earlier entries:
			// enums are redeclared in later versions
			// as their encoding changes;
			// deprecations count as updates too.
			if _, ok := symbols[sym]; !ok {
				symbols[sym] = symInfo{kind, minor}
			}
		}
	}

	// Read and parse the GOROOT/api manifests.
	for minor := 0; ; minor++ {
		base := "go1.txt"
		if minor > 0 {
			base = fmt.Sprintf("go1.%d.txt", minor)
		}
		filename := filepath.Join(runtime.GOROOT(), "api", base)
		data, err := os.ReadFile(filename)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// All caught up.
				// Synthesize one final file from any api/next/*.txt fragments.
				// (They are consolidated into a go1.%d file some time between
				// the freeze and the first release candidate.)
				filenames, err := filepath.Glob(filepath.Join(runtime.GOROOT(), "api/next/*.txt"))
				if err != nil {
					log.Fatal(err)
				}
				var next bytes.Buffer
				for _, filename := range filenames {
					data, err := os.ReadFile(filename)
					if err != nil {
						log.Fatal(err)
					}
					next.Write(data)
				}
				parse(filename, next.Bytes(), minor) // (filename is a lie)
				break
			}
			log.Fatal(err)
		}
		parse(filename, data, minor)
	}

	// The APIs of the syscall/js and unsafe packages need to be computed explicitly,
	// because they're not included in the GOROOT/api/go1.*.txt files at this time.
	pkgs["syscall/js"] = loadSymbols("syscall/js", "GOOS=js", "GOARCH=wasm")
	pkgs["unsafe"] = exportedSymbols(types.Unsafe) // TODO(adonovan): set correct versions

	// Write the combined manifest.
	var buf bytes.Buffer
	buf.WriteString(`// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated by generate.go. DO NOT EDIT.

package stdlib

var PackageSymbols = map[string][]Symbol{
`)

	for _, path := range sortedKeys(pkgs) {
		pkg := pkgs[path]
		fmt.Fprintf(&buf, "\t%q: {\n", path)
		for _, name := range sortedKeys(pkg) {
			info := pkg[name]
			fmt.Fprintf(&buf, "\t\t{%q, %s, %d},\n",
				name, strings.Title(info.kind), info.minor)
		}
		fmt.Fprintln(&buf, "},")
	}
	fmt.Fprintln(&buf, "}")
	fmtbuf, err := format.Source(buf.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("manifest.go", fmtbuf, 0666); err != nil {
		log.Fatal(err)
	}
}

type symInfo struct {
	kind  string // e.g. "func"
	minor int    // go1.%d
}

// loadSymbols computes the exported symbols in the specified package
// by parsing and type-checking the current source.
func loadSymbols(pkg string, extraEnv ...string) map[string]symInfo {
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedTypes,
		Env:  append(os.Environ(), extraEnv...),
	}, pkg)
	if err != nil {
		log.Fatalln(err)
	} else if len(pkgs) != 1 {
		log.Fatalf("got %d packages, want one package %q", len(pkgs), pkg)
	}
	return exportedSymbols(pkgs[0].Types)
}

func exportedSymbols(pkg *types.Package) map[string]symInfo {
	symbols := make(map[string]symInfo)
	for _, name := range pkg.Scope().Names() {
		if obj := pkg.Scope().Lookup(name); obj.Exported() {
			var kind string
			switch obj.(type) {
			case *types.Func, *types.Builtin:
				kind = "func"
			case *types.Const:
				kind = "const"
			case *types.Var:
				kind = "var"
			case *types.TypeName:
				kind = "type"
				// TODO(adonovan): expand fields and methods of syscall/js.*
			default:
				log.Fatalf("unexpected object type: %v", obj)
			}
			symbols[name] = symInfo{kind: kind, minor: 0} // pretend go1.0
		}
	}
	return symbols
}

func sortedKeys[M ~map[K]V, K cmp.Ordered, V any](m M) []K {
	r := make([]K, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	slices.Sort(r)
	return r
}

func removeTypeParam(s string) string {
	i := strings.IndexByte(s, '[')
	j := strings.LastIndexByte(s, ']')
	if i > 0 && j > i {
		s = s[:i] + s[j+len("["):]
	}
	return s
}
