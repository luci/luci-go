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
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/goccy/go-yaml"
	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/data/text/intsetexpr"
)

// See //internal/strutil.star for where these functions are referenced.
func init() {
	declNative("expand_int_set", func(call nativeCall) (starlark.Value, error) {
		var s starlark.String
		if err := call.unpack(1, &s); err != nil {
			return nil, err
		}
		res, err := intsetexpr.Expand(s.GoString())
		if err != nil {
			return nil, fmt.Errorf("expand_int_set: %s", err)
		}
		out := make([]starlark.Value, len(res))
		for i, r := range res {
			out[i] = starlark.String(r)
		}
		return starlark.NewList(out), nil
	})

	declNative("json_to_yaml", func(call nativeCall) (starlark.Value, error) {
		var json starlark.String
		if err := call.unpack(1, &json); err != nil {
			return nil, err
		}
		var buf any
		if err := yaml.UnmarshalWithOptions([]byte(json.GoString()), &buf, yaml.UseOrderedMap()); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON as YAML: %s", err)
		}
		out, err := yaml.MarshalWithOptions(buf, yaml.UseLiteralStyleIfMultiline(true))
		if err != nil {
			return nil, err
		}
		return starlark.String(out), nil
	})

	declNative("b64_encode", func(call nativeCall) (starlark.Value, error) {
		var s starlark.String
		if err := call.unpack(1, &s); err != nil {
			return nil, err
		}
		return starlark.String(base64.StdEncoding.EncodeToString([]byte(s.GoString()))), nil
	})

	declNative("b64_decode", func(call nativeCall) (starlark.Value, error) {
		var s starlark.String
		if err := call.unpack(1, &s); err != nil {
			return nil, err
		}
		raw, err := base64.StdEncoding.DecodeString(s.GoString())
		if err != nil {
			return nil, err
		}
		return starlark.String(string(raw)), nil
	})

	declNative("hex_encode", func(call nativeCall) (starlark.Value, error) {
		var s starlark.String
		if err := call.unpack(1, &s); err != nil {
			return nil, err
		}
		return starlark.String(hex.EncodeToString([]byte(s.GoString()))), nil
	})

	declNative("hex_decode", func(call nativeCall) (starlark.Value, error) {
		var s starlark.String
		if err := call.unpack(1, &s); err != nil {
			return nil, err
		}
		raw, err := hex.DecodeString(s.GoString())
		if err != nil {
			return nil, err
		}
		return starlark.String(string(raw)), nil
	})
}
