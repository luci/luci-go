// Copyright 2019 The LUCI Authors.
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

package lucicfg

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"hash/fnv"
	"text/template"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/builtins"
)

type templateValue struct {
	tmpl *template.Template
	hash uint32
}

// String returns the string representation of the value.
func (t *templateValue) String() string { return "template(...)" }

// Type returns a short string describing the value's type.
func (t *templateValue) Type() string { return "template" }

// Freeze does nothing since templateValue is already immutable.
func (t *templateValue) Freeze() {}

// Truth returns the truth value of an object.
func (t *templateValue) Truth() starlark.Bool { return starlark.True }

// Hash returns a function of x such that Equals(x, y) => Hash(x) == Hash(y).
func (t *templateValue) Hash() (uint32, error) { return t.hash, nil }

// AttrNames returns all .<attr> of this object.
func (t *templateValue) AttrNames() []string {
	return []string{"render"}
}

// Attr returns a .<name> attribute of this object or nil.
func (t *templateValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "render":
		return templateRenderBuiltin.BindReceiver(t), nil
	default:
		return nil, nil
	}
}

// render implements template rendering using given value as input.
func (t *templateValue) render(data any) (string, error) {
	buf := bytes.Buffer{}
	if err := t.tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Implementation of template.render(**dict) builtin.
var templateRenderBuiltin = starlark.NewBuiltin("render", func(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("render: expecting only keyword arguments, got %d positional", len(args))
	}

	// Convert kwargs to a real dict and then to a go nested map reusing to_json
	// machinery.
	data := starlark.NewDict(len(kwargs))
	for _, tup := range kwargs {
		if len(tup) != 2 {
			panic(fmt.Sprintf("impossible kwarg with len %d", len(tup)))
		}
		if err := data.SetKey(tup[0], tup[1]); err != nil {
			panic(fmt.Sprintf("impossible bad kwarg %s %s", tup[0], tup[1]))
		}
	}
	obj, err := builtins.ToGoNative(data)
	if err != nil {
		return nil, fmt.Errorf("render: %s", err)
	}

	out, err := fn.Receiver().(*templateValue).render(obj)
	if err != nil {
		return nil, fmt.Errorf("render: %s", err)
	}
	return starlark.String(out), nil
})

////////////////////////////////////////////////////////////////////////////////

type templateCache struct {
	cache map[string]*templateValue // SHA256 of body => parsed template
}

func (tc *templateCache) get(body string) (starlark.Value, error) {
	blob := []byte(body)
	hash := sha256.Sum256(blob)
	cacheKey := string(hash[:])
	if t, ok := tc.cache[cacheKey]; ok {
		return t, nil
	}

	tmpl, err := template.New("<str>").Parse(body)
	if err != nil {
		return nil, err // note: the error is already prefixed by "template: ..."
	}
	val := &templateValue{tmpl: tmpl}

	fh := fnv.New32a()
	fh.Write(blob)
	val.hash = fh.Sum32()

	if tc.cache == nil {
		tc.cache = make(map[string]*templateValue, 1)
	}
	tc.cache[cacheKey] = val
	return val, nil
}

// See //internal/strutil.star for where this is used.
func init() {
	declNative("template", func(call nativeCall) (starlark.Value, error) {
		var body starlark.String
		if err := call.unpack(1, &body); err != nil {
			return nil, err
		}
		return call.State.templates.get(body.GoString())
	})
}
