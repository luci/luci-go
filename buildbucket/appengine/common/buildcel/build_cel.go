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

package buildcel

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var env *buildCELEnv

func init() {
	var err error
	env, err = newBuildCELEnv()
	if err != nil {
		panic(errors.Annotate(err, "create CEL environment").Err())
	}
}

// buildCELEnv is a CEL environment with Build proto as its variable.
type buildCELEnv struct {
	env *cel.Env
}

func fullName(msg proto.Message) string {
	return string(msg.ProtoReflect().Descriptor().FullName())
}

// newBuildCELEnv creates a CEL environment to work with Build proto.
func newBuildCELEnv() (*buildCELEnv, error) {
	bldMsg := &pb.Build{}
	spM := &pb.StringPair{}
	env, err := cel.NewEnv(
		cel.Types(bldMsg, spM),
		cel.Variable("build", cel.ObjectType(fullName(bldMsg))),
		// `build.tags.get_value("key")`
		cel.Function(
			"get_value",
			cel.MemberOverload(
				"string_pairs_get_value",
				[]*cel.Type{
					cel.ListType(cel.ObjectType(fullName(spM))),
					cel.StringType,
				},
				cel.StringType,
				cel.BinaryBinding(stringPairsGetValue),
			),
		),
	)
	if err != nil {
		return nil, err
	}
	return &buildCELEnv{env: env}, nil
}

// compile parses and checks the expression `expr` against the CEL type in build
// CEL environment for validation.
// If succeed, it will return the compiled cel.Ast (Abstract Syntax Tree) for
// future use.
func (bce *buildCELEnv) compile(expr string, celType *cel.Type) (*cel.Ast, error) {
	ast, iss := bce.env.Compile(expr)
	if iss.Err() != nil {
		return ast, iss.Err()
	}

	if !ast.OutputType().IsExactType(celType) {
		return ast, errors.Reason("expect %s, got %s", celType, ast.OutputType()).Err()
	}
	return ast, nil
}

// Program generates an evaluable instance of the Ast within buildCELEnv.
func (bce *buildCELEnv) program(ast *cel.Ast) (cel.Program, error) {
	return bce.env.Program(ast)
}

type base struct {
	prg cel.Program
}

func newBaseBuildCEL(expr string, celType *cel.Type) (*base, error) {
	bc := &base{}

	// Compile
	ast, err := env.compile(expr, celType)
	if err != nil {
		return nil, errors.Annotate(err, "error compiling expression %q", expr).Err()
	}

	// Program
	bc.prg, err = env.program(ast)
	if err != nil {
		return nil, errors.Annotate(err, "program error").Err()
	}
	return bc, nil
}

func (bc *base) eval(b *pb.Build) (ref.Val, *cel.EvalDetails, error) {
	return bc.prg.Eval(map[string]any{
		"build": b,
	})
}

// Bool is a CEL program to evaluate a Build message against a list of
// bool expressions.
//
// Currently it can support checking if:
// * build has a field, e.g. `has(build.tags)`,
// * input/output properties has a key, e.g. `has(build.input.properties.pro_key)`
// * an input/output property with key "key" has "value": `string(build.output.properties.key) == "value"`
//   - Note: because input/output properties are Struct, we have to cast
//     the value to string.
//
// * experiments includes an experiment, e.g. `build.input.experiments.exists(e, e=="luci.buildbucket.exp")`
// * tags includes a tag with key "key", and there are two ways:
//   - `build.tags.get_value("key")!=""`
//   - `build.tags.exists(t, t.key=="key")`
type Bool struct {
	base *base
}

// generateExpression validates each predicate then concatenates them to
// generate one expression string.
func (bbc *Bool) generateExpression(predicates []string) (string, error) {
	if len(predicates) == 0 {
		return "", errors.Reason("predicates are required").Err()
	}

	// Validate
	merr := make(errors.MultiError, len(predicates))
	for i, p := range predicates {
		_, merr[i] = env.compile(p, cel.BoolType)
	}
	if merr.First() != nil {
		return "", merr
	}

	// Generate
	expr := strings.Join(predicates, ") && (")
	return "(" + expr + ")", nil

}

// NewBool generates a CEL program to evaluate a Build message against a
// list of bool expressions (aka `predicates`).
// The predicates are concatenated with "&&", meaning that the Build needs to
// match all predicates to pass `Eval`.
func NewBool(predicates []string) (*Bool, error) {
	bbc := &Bool{}
	expr, err := bbc.generateExpression(predicates)
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate CEL expression").Err()
	}
	bbc.base, err = newBaseBuildCEL(expr, cel.BoolType)
	if err != nil {
		return nil, err
	}
	return bbc, nil
}

// Eval evaluates the given build.
// If the build matchs all predicates, Eval returns true; Otherwise false.
func (bbc *Bool) Eval(b *pb.Build) (bool, error) {
	out, _, err := bbc.base.eval(b)
	if err != nil {
		return false, err
	}
	return out.Value().(bool), nil
}

// StringMap is a CEL program to return the requested information of
// a Build message in a string map.
//
// Currently it can support getting values for:
// * value of any build string field, e.g. `build.summary_markdown`
// * value of an input/output property, e.g. `string(build.input.properties.key)`
//   - Note: because input/output properties are Struct, we have to cast
//     the value to string.
//
// * value of a tag, e.g. `build.tags.get_value("key")`
type StringMap struct {
	base *base
}

// generateExpression converts a string map to a string so it can be used as an
// CEL expression.
func (smbc *StringMap) generateExpression(strMap map[string]string) (string, error) {
	if len(strMap) == 0 {
		return "", errors.Reason("string map is required").Err()
	}
	// Validate
	merr := make(errors.MultiError, len(strMap))
	idx := 0
	for _, s := range strMap {
		_, merr[idx] = env.compile(s, cel.StringType)
		idx++
	}
	if merr.First() != nil {
		return "", merr
	}

	// Generate
	expr := `{`
	remaining := len(strMap)
	for k, v := range strMap {
		remaining -= 1
		expr += fmt.Sprintf("%q:%s", k, v)
		if remaining > 0 {
			expr += ","
		}
	}
	expr += `}`
	return expr, nil
}

// NewStringMap generates a CEL program to evaluate a Build message
// against a string map. The values of the map can be either a CEL expression
// for the build which should output a string, like `build.summary_markdown`, or
// a string literal like `"random string literal"`.
func NewStringMap(strMap map[string]string) (*StringMap, error) {
	smbc := &StringMap{}
	expr, err := smbc.generateExpression(strMap)
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate CEL expression").Err()
	}
	smbc.base, err = newBaseBuildCEL(expr, cel.MapType(cel.StringType, cel.StringType))
	if err != nil {
		return nil, err
	}
	return smbc, nil
}

// Eval evaluates the build and returns the evaluation result.
func (smbc *StringMap) Eval(b *pb.Build) (map[string]string, error) {
	out, _, err := smbc.base.eval(b)
	if err != nil {
		return nil, err
	}

	outVal, err := out.ConvertToNative(reflect.TypeOf(map[string]string{}))
	if err != nil {
		return nil, err
	}

	return outVal.(map[string]string), nil
}

// custom functions.

// stringPairsGetValue implements the custom function
// `build.tags.get_value("key") string`.
func stringPairsGetValue(strPairsVal ref.Val, keyVal ref.Val) ref.Val {
	sPirs := strPairsVal.(traits.Lister)
	key := string(keyVal.(types.String))
	for it := sPirs.Iterator(); it.HasNext() == types.True; {
		sp := it.Next().Value().(*pb.StringPair)
		if sp.Key == key {
			return types.String(sp.Value)
		}
	}
	return types.String("")
}
