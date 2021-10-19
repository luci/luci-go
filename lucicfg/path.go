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

// cleanRelativePath does path.Join and returns an error if 'rel' is absolute or
// (if allowDots is false) the resulting path starts with "..".
func cleanRelativePath(base, rel string, allowDots bool) (string, error) {
	if path.IsAbs(rel) {
		return "", fmt.Errorf("absolute path %q is not allowed", rel)
	}
	rel = path.Join(base, rel)
	if !allowDots && (rel == ".." || strings.HasPrefix(rel, "../")) {
		return "", fmt.Errorf("path %q is not allowed, must not start with \"../\"", rel)
	}
	return rel, nil
}

func init() {
	// Used in lucicfg.star, strutil.star and validate.star.
	declNative("clean_relative_path", func(call nativeCall) (starlark.Value, error) {
		var base starlark.String
		var path starlark.String
		var allowDots starlark.Bool
		if err := call.unpack(3, &base, &path, &allowDots); err != nil {
			return nil, err
		}
		res, err := cleanRelativePath(base.GoString(), path.GoString(), bool(allowDots))
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		return starlark.Tuple{starlark.String(res), starlark.String(errStr)}, nil
	})
}
