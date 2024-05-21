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

package rewriteimpl

import (
	"fmt"
	"go/ast"

	"github.com/dave/dst"
)

func usesImport(f *dst.File, packageName string) bool {
	targ := fmt.Sprintf("%q", packageName)
	for _, imp := range f.Imports {
		if imp.Path.Value == targ {
			return true
		}
	}
	return false
}

func restoreImports(newF *ast.File) {
	newF.Imports = nil

	ast.Inspect(newF, func(n ast.Node) bool {
		if imp, ok := n.(*ast.ImportSpec); ok {
			newF.Imports = append(newF.Imports, imp)
			return false
		}
		return true
	})
}
