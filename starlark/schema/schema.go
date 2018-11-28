// Copyright 2018 The LUCI Authors.
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

// Package schema defines a starlark library for validating string-keyed
// JSON-ish dict structures.
//
// Usage:
//
//    s = {
//      'int': schema.int(default = 10),
//      'required': schema.string(required = True),
//      'list_of_dicts': [
//          {
//            'key': schema.string(),
//          },
//      ],
//    }
//
//    validated, err = schema.validate({'required': 'value'}, s)
//    assert.eq(err, None)
//    assert.eq(validated['int'], 10)
//    assert.eq(validated['list_of_dicts'], [])
package schema

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// SchemaLib() returns a dict with single struct named "schema" that holds API
// of SchemaLib 'schema' library.
func SchemaLib() starlark.StringDict {
	return starlark.StringDict{
		"schema": starlarkstruct.FromStringDict(starlark.String("schema"), starlark.StringDict{
			"bool":   starlark.NewBuiltin("bool", scalarValidatorBuiltin),
			"float":  starlark.NewBuiltin("float", scalarValidatorBuiltin),
			"int":    starlark.NewBuiltin("int", scalarValidatorBuiltin),
			"string": starlark.NewBuiltin("string", scalarValidatorBuiltin),

			"validate": starlark.NewBuiltin("validate", validateBuiltin),
		}),
	}
}

// scalarValidator is a starlark.Value that represents validators for scalar
// types. It's what schema.int(...) etc return.
type scalarValidator struct {
	typ string         // an expected value type, as in type(val) == typ
	req starlark.Bool  // true if the value must be provided
	def starlark.Value // a default value for the field if it wasn't provided
}

func (v *scalarValidator) String() string {
	return fmt.Sprintf("schema.%s(required=%s, default=%s)", v.typ, v.req, v.def)
}

func (v *scalarValidator) Freeze() {
	v.def.Freeze()
}

func (*scalarValidator) Type() string          { return "schema.validator" }
func (*scalarValidator) Truth() starlark.Bool  { return starlark.True }
func (*scalarValidator) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable") }

var zeros = map[string]starlark.Value{
	"bool":   starlark.Bool(false),
	"float":  starlark.Float(0),
	"int":    starlark.MakeInt(0),
	"string": starlark.String(""),
}

// scalarValidatorBuiltin is schema.<scalar>(**opts) starlark func.
//
// It returns a corresponding *scalarValidator, depending on the name of the
// function. E.g. if fn.Name() is int, the returned validator works with ints.
//
// All arguments should be passed with kwargs, and all are optional.
func scalarValidatorBuiltin(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	typ := fn.Name()        // expected type to be validated, matches the function name
	name := "schema." + typ // name of the function for error messages

	if len(args) != 0 {
		return nil, fmt.Errorf("%s: not expecting positional arguments", name)
	}

	var req starlark.Bool
	var def starlark.Value
	if err := starlark.UnpackArgs(name, nil, kwargs, "required?", &req, "default?", &def); err != nil {
		return nil, err
	}

	// If 'default' is not explicitly given or None, set it to the corresponding
	// zero, otherwise verify it has the matching type.
	if def == nil || def == starlark.None {
		def = zeros[typ]
	} else if def.Type() != typ {
		return nil, fmt.Errorf("%s: default value has wrong type %s", name, def.Type())
	}

	return &scalarValidator{
		typ: typ,
		req: req,
		def: def,
	}, nil
}

// validateBuiltin is schema.validate(<value>, <schema>) starlark func.
//
// It's an entry point into the validation logic.
//
// It returns a tuple:
//   On success: (validated <value> with all defaults filled in, None)
//   On error:   (None, error message string).
func validateBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var val, schema starlark.Value
	if err := starlark.UnpackPositionalArgs("validate", args, kwargs, 2, &val, &schema); err != nil {
		return nil, err
	}
	val, err := validate("", val, schema)
	if err != nil {
		return starlark.Tuple{starlark.None, starlark.String(err.Error())}, nil
	}
	return starlark.Tuple{val, starlark.None}, nil
}

// validate implements the actual validation, recursively.
//
// Arguments:
//   ctx - a string for error messages with current path within validated dict
//   val - a value to validate (or None if it's unset)
//   schema - a schema to validate against.
//
// On successful validation it returns a validated copy of 'val' with all
// defaults filled in.
func validate(ctx string, val, schema starlark.Value) (starlark.Value, error) {
	switch s := schema.(type) {
	case *scalarValidator:
		return visitScalar(ctx, val, s)
	case *starlark.List:
		return visitList(ctx, val, s)
	case *starlark.Dict:
		return visitDict(ctx, val, s)
	default:
		return nil, validationErr(ctx, "unknown schema element of type %s", s.Type())
	}
}

func visitScalar(ctx string, val starlark.Value, schema *scalarValidator) (starlark.Value, error) {
	if val == starlark.None {
		if schema.req {
			return nil, validationErr(ctx, "a value is required")
		}
		return schema.def, nil
	}
	if val.Type() != schema.typ {
		return nil, validationErr(ctx, "got %s (%s), expecting %s", val.Type(), val, schema.typ)
	}
	return val, nil
}

func visitList(ctx string, val starlark.Value, schema *starlark.List) (starlark.Value, error) {
	if schema.Len() != 1 {
		return nil, validationErr(ctx, "bad list schema: expecting exactly one item with elements schema")
	}
	elemSchema := schema.Index(0)

	if val == starlark.None {
		return starlark.NewList(nil), nil
	}

	seq, ok := val.(starlark.Sequence)
	if !ok {
		return nil, validationErr(ctx, "got %s (%s), expecting a list or a tuple", val.Type(), val)
	}
	iter := seq.Iterate()
	defer iter.Done()

	out := make([]starlark.Value, 0, seq.Len())

	var x starlark.Value
	for iter.Next(&x) {
		val, err := validate(joinCtx(ctx, "[%d]", len(out)), x, elemSchema)
		if err != nil {
			return nil, err
		}
		out = append(out, val)
	}

	return starlark.NewList(out), nil
}

func visitDict(ctx string, val starlark.Value, schema *starlark.Dict) (starlark.Value, error) {
	if val == starlark.None {
		val = &starlark.Dict{}
	}

	// The value must be a dict.
	dict, ok := val.(*starlark.Dict)
	if !ok {
		return nil, validationErr(ctx, "got %s (%s), expecting a dict", val.Type(), val)
	}
	_ = dict

	// Collect all expected keys in the schema. They should all be strings.
	keys := make([]string, 0, schema.Len())
	for _, k := range schema.Keys() {
		str, ok := k.(starlark.String)
		if !ok {
			return nil, validationErr(ctx, "bad dict schema: got %s (%s) key, expecting a string", k.Type(), k)
		}
		keys = append(keys, str.GoString())
	}

	// TODO

	return nil, nil
}

func validationErr(ctx, msg string, args ...interface{}) error {
	if ctx != "" {
		msg = "in %s: " + msg
		args = append([]interface{}{ctx}, args...)
	}
	return fmt.Errorf(msg, args...)
}

func joinCtx(ctx, msg string, args ...interface{}) string {
	msg = fmt.Sprintf(msg, args...)
	if ctx == "" {
		return msg
	}
	return ctx + "." + msg
}
