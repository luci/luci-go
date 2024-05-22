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
	"log"
	"strings"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"go.chromium.org/luci/common/data/stringset"
	"golang.org/x/tools/go/ast/astutil"
)

// isConveyCall checks to see if `c` is call expression which is based on
// a selector expression, the lefthand side is one of `selectorNames` and whose
// target is `target`.
//
// If one of selectorNames is '.' this will also recognize a call directly to
// `target`.
//
// Returns the CallExpr or nil, if this expression is not a matching call.
func isConveyCall(c dst.Node, selectorNames []string, target string) *dst.CallExpr {
	exp, ok := c.(*dst.ExprStmt)
	if !ok {
		return nil
	}

	call, ok := exp.X.(*dst.CallExpr)
	if !ok {
		return nil
	}

	for _, selName := range selectorNames {
		if selName == "." {
			if fun, ok := call.Fun.(*dst.Ident); ok && fun.Name == target {
				return call
			}
		} else if sel, ok := call.Fun.(*dst.SelectorExpr); ok {
			if x, ok := sel.X.(*dst.Ident); ok && x.Name == selName && sel.Sel.Name == target {
				return call
			}
		}
	}
	return nil
}

// selectorOrIdent looks to see if `c` is either
//
//   - an dst.Ident, if importName is "." OR
//   - an dst.SelectorExpr whose .X is an Ident matches importName
//
// If either matches, returns the *dst.Ident.Name.
// Otherwise returns "".
func selectorOrIdent(c dst.Node, importName string) string {
	if importName != "" {
		if importName == "." {
			if ident, ok := c.(*dst.Ident); ok {
				return ident.Name
			}
		} else if sel, ok := c.(*dst.SelectorExpr); ok {
			if ident, ok := sel.X.(*dst.Ident); ok && ident.Name == importName {
				return sel.Sel.Name
			}
		}
	}
	return ""
}

// getImportName returns the file-local name for a given package import.
//
// This incorrectly assumes that the default local name for a package "a/b/c" is
// "c", but this is good enough for the rewriter tool's use.
//
// If the given package is not imported, returns "".
func getImportName(f *dst.File, pkg string) string {
	for _, imp := range f.Imports {
		if imp.Path.Value == fmt.Sprintf(`"%s"`, originalConveyPkg) {
			if imp.Name != nil {
				// user set some name or ".", return that
				return imp.Name.Name
			}
			// assume the package name is the ldst bit after the /... not actually
			// correct, but close enough for the two packages we care about, convey
			// and assertions.
			toks := strings.Split(pkg, "/")
			return toks[len(toks)-1]
		}
	}
	return ""
}

func rewriteConvey(convCall *dst.CallExpr, conveyContextName string) (newContextName string, warned bool) {
	newContextName = conveyContextName

	adjustConvCall := func() {
		if len(convCall.Args) == 3 {
			convCall.Fun = &dst.SelectorExpr{
				X:   &dst.Ident{Name: "ftt"},
				Sel: &dst.Ident{Name: "Run"},
			}
		} else {
			convCall.Fun = &dst.SelectorExpr{
				X:   &dst.Ident{Name: conveyContextName},
				Sel: &dst.Ident{Name: "Run"},
			}
		}
	}

	if numArgs := len(convCall.Args); numArgs == 2 || numArgs == 3 {
		switch x := convCall.Args[len(convCall.Args)-1].(type) {
		case *dst.FuncLit:
			adjustConvCall()
			if conveyContextName == "." {
				newContextName = "t"
			} else {
				newContextName = conveyContextName
			}
			if len(x.Type.Params.List) == 1 {
				if names := x.Type.Params.List[0].Names; len(names) == 1 {
					newContextName = names[0].Name
				}
			}
			x.Type.Params.List = []*dst.Field{
				{
					Names: []*dst.Ident{{Name: newContextName}},
					Type: &dst.StarExpr{
						X: &dst.SelectorExpr{
							X:   &dst.Ident{Name: "ftt"},
							Sel: &dst.Ident{Name: "Test"},
						},
					},
				},
			}
			return

		case *dst.Ident:
			if x.Name == "nil" {
				adjustConvCall()
				// artifacts from convey composer in webbrowser... there are some
				// tests which still have `nil` in them.
				//
				// ignore
				return
			}
		}
	}

	// TODO: print filename:lineno - difficult with dst
	log.Println("WARN: Could not convert convey suite callback function")
	warned = true
	return
}

// rewriteSoCall rewrites a So call like:
//
//	So(something, ShouldCompareTo, expected...)
//
// to
//
//	assert.Loosely(t,
func rewriteSoCall(soCall *dst.CallExpr, conveyImportName, assertionsImportName, conveyContextName string, adaptedAssertions stringset.Set) (usedAssertionsLib, usedAdapter bool) {
	origComparison := soCall.Args[1]

	// First pick the remapper, checking for:
	//
	//   conveyImportName.ShouldSomething
	//   assertionsImportName.ShouldSomething
	//
	// Where conveyImportName or assertionsImportName could be "." (i.e. dot
	// imported)
	comparisonID := selectorOrIdent(origComparison, conveyImportName)
	remapper := assertionMap[assertionKey{originalConveyPkg, comparisonID}]
	if remapper == nil {
		comparisonID = selectorOrIdent(origComparison, assertionsImportName)
		remapper = assertionMap[assertionKey{originalAssertionsPkg, comparisonID}]
	}

	// Now attempt the remapping. This will tell us if it was successful and if it
	// was, if the convey.Adapt function was used to wrap some existing goconvey
	// comparison function.
	if usedAdapter = remapper.handle(soCall, conveyContextName); usedAdapter {
		var adaptedRepr string
		switch x := origComparison.(type) {
		case *dst.Ident:
			// x
			adaptedRepr = x.Name
		case *dst.SelectorExpr:
			if xID, ok := x.X.(*dst.Ident); ok {
				// x.y
				adaptedRepr = fmt.Sprintf("%s.%s", xID.Name, x.Sel.Name)
			}
		}
		if adaptedRepr != "" {
			if adaptedAssertions.Add(adaptedRepr) {
				// TODO: print filename:lineno - difficult with dst
				log.Printf("FYI: Adapting %s", adaptedRepr)
			}
		}

		if adaptedAssertions.Has(comparisonID) {
			usedAssertionsLib = true
		}
	}
	return
}

// Rewrite rewrites a single file in `dec` to use:
//
//	assert.Loosely instead of So
//	ftt.Run / t.Run instead of Convey
//	should.X instead of ShouldX
//
// adaptedAssertions is an input and output parameter which is state of the
// assertions that have been adapted with convey.Adapt(old). This is used by
// rewrite to only emit an FYI message for the first time each unique assertion
// is adapted.
//
// Returns the new file (even if no rewrite took place).
// Rewrote will be true if the file was rewritten.
// Warned will be true if the file contained warnings (e.g. convey suites or
// SoMsg assertions that we couldn't translate).
func Rewrite(dec *decorator.Decorator, f *dst.File, adaptedAssertions stringset.Set) (newFile *ast.File, rewrote, warned bool, err error) {
	res := decorator.NewRestorer()
	res.Fset = dec.Fset

	if !usesImport(f, originalConveyPkg) {
		newFile, err = res.RestoreFile(f)
		return
	}
	rewrote = true

	conveyImportName := getImportName(f, originalConveyPkg)
	assertionsImportName := getImportName(f, originalAssertionsPkg)

	needAssertions := false
	needShould := false
	needAdapter := false

	for _, decl := range f.Decls {
		// This is a hack for keeping track of code like:
		//
		//    Convey("name", func(c C) {
		//       ...
		//    })
		//
		// In order to keep track of `c`. We want to replace this with `t`, but we
		// need to be able to find `c.So` and `c.SoMsg` calls. Note that this is
		// only set on the way down the stack, meaning if there are nested conveys
		// with different names for C, this would be wrong... but we also expect
		// that to be very rare/non-existant.
		//
		// If this is ".", it means that the original convey suite was using the
		// implicit context (which is 99% of cases).
		conveyContextName := "."

		dst.Inspect(decl, func(c dst.Node) (needRecursion bool) {
			if convCall := isConveyCall(c, []string{conveyImportName, conveyContextName}, "Convey"); convCall != nil {
				// conveyImportName.Convey(name, t, func() { ... })
				// conveyContextName.Convey(name, func() { ... })
				// conveyImportName.Convey(name, t, func(c C) { ... })
				// conveyContextName.Convey(name, func(c C) { ... })
				newContextName, warning := rewriteConvey(convCall, conveyContextName)
				conveyContextName = newContextName
				warned = warned || warning

				// always recurse into Convey suites.
				return true
			}

			// We ALWAYS look for "c" to pick up most of the helper functions which
			// pass `c C`.
			//
			// In addition, we also look for the conveyImportName because even though
			// a callback function uses `c C`, it could still use the global name :/.
			if soCall := isConveyCall(c, []string{"c", conveyImportName, conveyContextName}, "So"); soCall != nil {
				// conveyContextName.So(...)
				usedAssertionsLib, usedAdapter := rewriteSoCall(
					soCall, conveyImportName, assertionsImportName, conveyContextName,
					adaptedAssertions)

				needAdapter = needAdapter || usedAdapter
				needShould = needShould || !usedAdapter
				needAssertions = needAssertions || usedAssertionsLib

				// never recurse into So calls.
				return false
			}
			if soMsgCall := isConveyCall(c, []string{"c", conveyImportName, conveyContextName}, "SoMsg"); soMsgCall != nil {
				// there are so few of these we'll do them by hand, so just print it out
				// here:
				// TODO: indicate filename/lineno - this is easy (ish) with `ast`, but
				// difficult with `dst`.
				//
				// However, we still adapt the assertion.
				log.Println("WARN: found SoMsg (needs manual fix `if check.Loosely(...) { t.Fatal(msg) }`)")
				warned = true

				// never recurse into SoMsg calls.
				return false
			}

			// Otherwise always recurse
			return true
		})
	}

	newFile, err = res.RestoreFile(f)
	if err != nil {
		return
	}

	// BUG: apparently when restoring a file, the Imports are not restored
	restoreImports(newFile)

	astutil.DeleteImport(dec.Fset, newFile, originalConveyPkg)
	astutil.DeleteNamedImport(dec.Fset, newFile, ".", originalConveyPkg)
	if !needAssertions {
		astutil.DeleteImport(dec.Fset, newFile, originalAssertionsPkg)
		astutil.DeleteNamedImport(dec.Fset, newFile, ".", originalAssertionsPkg)
	}
	astutil.AddImport(dec.Fset, newFile, fttPkg)
	if needAdapter {
		astutil.AddImport(dec.Fset, newFile, conveyAdapterPkg)
	}
	if needShould {
		astutil.AddImport(dec.Fset, newFile, shouldPkg)
	}
	astutil.AddImport(dec.Fset, newFile, assertPkg)

	return
}
