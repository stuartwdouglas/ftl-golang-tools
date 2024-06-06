// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// The copylock command applies the github.com/TBD54566975/x/tools/go/analysis/passes/copylock
// analysis to the specified packages of Go source code.
package main

import (
	"github.com/TBD54566975/x/tools/go/analysis/passes/copylock"
	"github.com/TBD54566975/x/tools/go/analysis/singlechecker"
)

func main() { singlechecker.Main(copylock.Analyzer) }
