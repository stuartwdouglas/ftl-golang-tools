// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package copylock_test

import (
	"testing"

	"github.com/block/ftl-golang-tools/go/analysis/analysistest"
	"github.com/block/ftl-golang-tools/go/analysis/passes/copylock"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, copylock.Analyzer, "a", "typeparams", "issue67787")
}
