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
	"fmt"
	"path"
	"strings"

	"go.starlark.net/starlark"
)

// cleanRelativePath does path.Clean and returns an error if 'p' is absolute or
// (if allowDots is false) starts with "..[/]".
func cleanRelativePath(p string, allowDots bool) (string, error) {
	switch p = path.Clean(p); {
	case path.IsAbs(p):
		return "", fmt.Errorf("absolute path %q is not allowed", p)
	case !allowDots && (p == ".." || strings.HasPrefix(p, "../")):
		return "", fmt.Errorf("path %q is not allowed, must not start with \"../\"", p)
	default:
		return p, nil
	}
}

func init() {
	// Used in //internal/lucicfg.star and in //internal/validate.star.
	declNative("clean_relative_path", func(call nativeCall) (starlark.Value, error) {
		var s starlark.String
		var allowDots starlark.Bool
		if err := call.unpack(2, &s, &allowDots); err != nil {
			return nil, err
		}
		res, err := cleanRelativePath(s.GoString(), bool(allowDots))
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		return starlark.Tuple{starlark.String(res), starlark.String(errStr)}, nil
	})
}
