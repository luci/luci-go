// Copyright 2024 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package rewriteimpl will take one or more existing Go test files on the command-line and
// rewrite "goconvey" tests in them to use
// go.chromium.org/luci/common/testing/ftt.
//
// This makes the following changes:
//   - Converts all Convey function calls to ftt.Run calls.
//     This will also rewrite the callback from `func() {...}` to
//     `func(t *ftt.Test) {...}`. If the test already had an explicit
//     context variable (e.g. `func(c C)`), this will preserve that name.
//   - Removes dot-imported convey.
//   - Adds imports for go.chromium.org/luci/testing/common/truth/assert and
//     go.chromium.org/luci/testing/common/truth/should.
//   - Rewrites So and SoMsg calls.
//
// Note that 'fancy' uses of Convey/So will likely be missed by this tool, and
// will require hand editing... however this does complete successfully for all
// code in luci-go, infra and infra_internal at the time of writing.
package rewriteimpl

import (
	"go/parser"
	"go/token"
	"os"

	// TODO: Switch back to pure go/ast when https://github.com/golang/go/issues/20744
	// is fixed.
	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
)

// ParseOne just parses go code from a local file path.
//
// Returns a File and it's matching FileSet (containing ONLY `fname`).
func ParseOne(fname string) (*decorator.Decorator, *dst.File, error) {
	fset := token.NewFileSet()
	dat, err := os.ReadFile(fname)
	if err != nil {
		return nil, nil, err
	}

	dec := decorator.NewDecorator(fset)
	fil, err := dec.ParseFile(fname, dat, parser.ParseComments)
	return dec, fil, err
}
