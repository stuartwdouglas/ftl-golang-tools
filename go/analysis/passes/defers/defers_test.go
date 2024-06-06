// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package defers_test

import (
	"testing"

	"github.com/TBD54566975/golang-tools/go/analysis/analysistest"
	"github.com/TBD54566975/golang-tools/go/analysis/passes/defers"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, defers.Analyzer, "a")
}
