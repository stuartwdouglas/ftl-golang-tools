// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The defers command runs the defers analyzer.
package main

import (
	"github.com/block/ftl-golang-tools/go/analysis/passes/defers"
	"github.com/block/ftl-golang-tools/go/analysis/singlechecker"
)

func main() { singlechecker.Main(defers.Analyzer) }
