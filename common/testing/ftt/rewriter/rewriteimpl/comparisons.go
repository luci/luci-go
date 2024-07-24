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
	"go/token"

	"github.com/dave/dst"
)

// argState is used to indicate for some comparison if it has no args,
// a vararg, or a constant set of arguments.
type argState int

const (
	argStateInvalid argState = iota

	// Has specific positional arguments.
	// So(actual, ShouldEqual, something)
	hasArgs

	// Has no additional arguments.
	// So(actual, ShouldBeTrue)
	noArgs

	// Has a variadic argument
	// So(actual, ShouldBeIn, args...)
	hasVararg
)

// mappedComp describes a comparison implementation in the `should` package.
type mappedComp struct {
	// The name of the function in the `should` package.
	name string

	// The type of arguments the comparison in the `should` package expects.
	argState argState

	// A hook to allow you to handle some comparison in a special way.
	//
	// This takes the `name` field value, and all extra arguments which were
	// previously on the left hand side of the `So(x, ShouldBlah, y)` assertion.
	//
	// It must return the new comparison name, it's argState and the extra
	// arguments to use for it.
	special func(name string, extraArgs []dst.Expr) (newName string, argState argState, newExtraArgs []dst.Expr)
}

// handle parses a "So" call expression (e.g. `So(x, ShouldSomething, y)`, and
// returns adapted=true if this function was able to map it to a truth
// assertion.
//
// This should modify the soCall expression directly into the new form.
//
// conveyImportName is the name that the convey package was imported under.
func (m *mappedComp) handle(soCall *dst.CallExpr, conveyContextName string) (adapted bool) {
	isMapped := m != nil

	// So(actual, ShouldSomething, original, expected, ...)
	// extraArgs contains "original, expected, ..."
	extraArgs := soCall.Args[2:]

	// So( -> assert.Loosely(
	origDeco := soCall.Fun.Decorations()
	soCall.Fun = &dst.SelectorExpr{
		X:   &dst.Ident{Name: "assert"},
		Sel: &dst.Ident{Name: "Loosely"},
		Decs: dst.SelectorExprDecorations{
			NodeDecs: *origDeco,
		},
	}

	// assert.Loosely(actual, should.Whatever, original, expected, ...) ->
	// assert.Loosely(actual, should.Whatever)
	soCall.Args = soCall.Args[:2]

	if !isMapped {
		// assert.Loosely(actual, ShouldCustom) ->
		// assert.Loosely(actual, adapt.Convey(ShouldCustom)(original, expected, ...))
		soCall.Args[1] = &dst.CallExpr{
			Fun: &dst.CallExpr{
				Fun: &dst.SelectorExpr{
					X:   &dst.Ident{Name: "convey"},
					Sel: &dst.Ident{Name: "Adapt"},
				},
				Args: []dst.Expr{soCall.Args[1]},
			},
			Args: extraArgs,
		}
	} else {
		name := m.name
		argState := m.argState
		if m.special != nil {
			name, argState, extraArgs = m.special(name, extraArgs)
		}
		switch argState {
		case argStateInvalid:
			panic(fmt.Errorf("mappedComp(%#v) has invalid argState", *m))
		case hasArgs:
			// assert.Loosely(actual, ShouldCustom) ->
			// assert.Loosely(actual, should.Mapped(original, expected, ...))
			soCall.Args[1] = &dst.CallExpr{
				Fun: &dst.SelectorExpr{
					X:   &dst.Ident{Name: "should"},
					Sel: &dst.Ident{Name: name},
				},
				Args: extraArgs,
			}

		case noArgs:
			// NOTE: This case with multiple expected args already returned an error up
			// top.
			// assert.Loosely(actual, ShouldThing) ->
			// assert.Loosely(actual, should.Mapped)
			soCall.Args[1] = &dst.SelectorExpr{
				X:   &dst.Ident{Name: "should"},
				Sel: &dst.Ident{Name: name},
			}

		case hasVararg:
			// assert.Loosely(actual, ShouldCustom, some, args...) ->
			// assert.Loosely(actual, should.Mapped(some, args...))
			soCall.Args[1] = &dst.CallExpr{
				Fun: &dst.SelectorExpr{
					X:   &dst.Ident{Name: "should"},
					Sel: &dst.Ident{Name: name},
				},
				Args:     extraArgs,
				Ellipsis: true,
			}
		}
	}

	// assert.Loosely(actual, ... -> assert.Loosely(t, actual, ...
	testContextName := "t"
	if conveyContextName != "." {
		testContextName = conveyContextName
	}
	soCall.Args = append([]dst.Expr{&dst.Ident{Name: testContextName}}, soCall.Args...)

	return !isMapped
}

type assertionKey struct {
	pkg  string
	name string
}

var assertionMap = map[assertionKey]*mappedComp{
	{originalConveyPkg, "ShouldAlmostEqual"}:      {name: "AlmostEqual", argState: hasArgs},
	{originalConveyPkg, "ShouldBeBetween"}:        {name: "BeBetween", argState: hasArgs},
	{originalConveyPkg, "ShouldBeBetweenOrEqual"}: {name: "ShouldBeBetweenOrEqual", argState: hasArgs},
	{originalConveyPkg, "ShouldBeEmpty"}:          {name: "BeEmpty", argState: noArgs},
	{originalConveyPkg, "ShouldBeBlank"}:          {name: "BeZero", argState: noArgs},
	{originalConveyPkg, "ShouldNotBeEmpty"}:       {name: "NotBeEmpty", argState: noArgs},
	{originalConveyPkg, "ShouldBeFalse"}:          {name: "BeFalse", argState: noArgs},
	{originalConveyPkg, "ShouldBeNil"}:            {name: "BeNil", argState: noArgs},
	{originalConveyPkg, "ShouldBeTrue"}:           {name: "BeTrue", argState: noArgs},
	{originalConveyPkg, "ShouldContain"}:          {name: "Contain", argState: hasArgs},
	{originalConveyPkg, "ShouldContainKey"}:       {name: "ContainKey", argState: hasArgs},
	{originalConveyPkg, "ShouldContainSubstring"}: {name: "ContainSubstring", argState: hasArgs},
	{originalConveyPkg, "ShouldEqual"}: {
		name:     "Equal",
		argState: hasArgs,
		special: func(name string, extraArgs []dst.Expr) (newName string, argState argState, newExtraArgs []dst.Expr) {
			if len(extraArgs) == 1 {
				arg := extraArgs[0]
				if ident, ok := arg.(*dst.Ident); ok && ident.Name == "nil" {
					// should.Equal(nil) does not work because it requires a type argument,
					// switch this to
					return "BeNil", noArgs, nil
				}
				if lit, ok := arg.(*dst.BasicLit); ok {
					if lit.Kind == token.INT && lit.Value == "0" {
						return "BeZero", noArgs, nil
					}
					if lit.Kind == token.STRING && len(lit.Value) == 2 {
						// covers "" and ``
						return "BeEmpty", noArgs, nil
					}
				}
			}
			// everything else just let it through
			return name, hasArgs, extraArgs
		}},

	{originalConveyPkg, "ShouldHaveLength"}: {name: "HaveLength", argState: hasArgs},
	{originalConveyPkg, "ShouldNotBeNil"}:   {name: "NotBeNil", argState: noArgs},
	{originalConveyPkg, "ShouldNotEqual"}:   {name: "NotEqual", argState: hasArgs},
	{originalConveyPkg, "ShouldResemble"}: {
		name: "Resemble",
		special: func(name string, extraArgs []dst.Expr) (newName string, argState argState, newExtraArgs []dst.Expr) {
			if len(extraArgs) == 1 {
				arg := extraArgs[0]
				if ident, ok := arg.(*dst.Ident); ok && ident.Name == "nil" {
					// Simplify ShouldResemble(nil) -> BeNil.
					return "BeNil", noArgs, nil
				}
				if lit, ok := arg.(*dst.BasicLit); ok {
					// Simplify some common ShouldResemble usages.
					if lit.Kind == token.INT && lit.Value == "0" {
						return "BeZero", noArgs, nil
					}
					if lit.Kind == token.STRING && len(lit.Value) == 2 {
						// covers "" and ``
						return "BeEmpty", noArgs, nil
					}
					// This is a basic non-zero literal type - this means that the left
					// hand side should not need the extra complexity of should.Resemble,
					// so use should.Match instead.
					return "Match", hasArgs, extraArgs
				}
			}
			// They probably actually need "should.Resemble", so let it go through.
			return name, hasArgs, extraArgs
		}},

	{originalConveyPkg, "ShouldBeGreaterOrEqualTo"}: {name: "BeGreaterThanOrEqualTo", argState: hasArgs},
	{originalConveyPkg, "ShouldBeGreaterThan"}:      {name: "BeGreaterThan", argState: hasArgs},
	{originalConveyPkg, "ShouldBeLessOrEqualTo"}:    {name: "BeLessThanOrEqualTo", argState: hasArgs},
	{originalConveyPkg, "ShouldBeLessThan"}:         {name: "BeLessThan", argState: hasArgs},

	// These two are a little silly - they either take a collection as a single
	// expected argument OR they take a series of elements...
	{originalConveyPkg, "ShouldBeIn"}: {
		name: "BeIn",
		special: func(name string, extraArgs []dst.Expr) (newName string, argState argState, newExtraArgs []dst.Expr) {
			if len(extraArgs) == 1 {
				return name, hasVararg, extraArgs
			}
			return name, hasArgs, extraArgs
		}},
	{originalConveyPkg, "ShouldNotBeIn"}: {
		name: "NotBeIn",
		special: func(name string, extraArgs []dst.Expr) (newName string, argState argState, newExtraArgs []dst.Expr) {
			if len(extraArgs) == 1 {
				return name, hasVararg, extraArgs
			}
			return name, hasArgs, extraArgs
		}},

	{originalConveyPkg, "ShouldContain"}:    {name: "Contain", argState: hasArgs},
	{originalConveyPkg, "ShouldNotContain"}: {name: "NotContain", argState: hasArgs},

	{originalConveyPkg, "ShouldStartWith"}: {name: "HavePrefix", argState: hasArgs},
	{originalConveyPkg, "ShouldEndWith"}:   {name: "HaveSuffix", argState: hasArgs},

	{originalConveyPkg, "ShouldPanic"}: {name: "Panic", argState: noArgs},

	{originalConveyPkg, "ShouldHappenBefore"}:      {name: "HappenBefore", argState: hasArgs},
	{originalConveyPkg, "ShouldHappenOnOrBefore"}:  {name: "HappenOnOrBefore", argState: hasArgs},
	{originalConveyPkg, "ShouldHappenAfter"}:       {name: "HappenAfter", argState: hasArgs},
	{originalConveyPkg, "ShouldHappenOnOrAfter"}:   {name: "HappenOnOrAfter", argState: hasArgs},
	{originalConveyPkg, "ShouldHappenOnOrBetween"}: {name: "HappenOnOrBetween", argState: hasArgs},
	{originalConveyPkg, "ShouldHappenWithin"}:      {name: "HappenWithin", argState: hasArgs},

	{originalAssertionsPkg, "ShouldResembleProto"}: {name: "Resemble", argState: hasArgs},
	{originalAssertionsPkg, "ShouldErrLike"}:       {name: "ErrLike", argState: hasArgs},
	{originalAssertionsPkg, "ShouldPanicLike"}:     {name: "PanicLike", argState: hasArgs},
	{originalAssertionsPkg, "ShouldUnwrapTo"}:      {name: "ErrLike", argState: hasArgs},
}
