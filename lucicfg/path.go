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
	"path/filepath"
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

	// package_dir(from_dir, main_pkg_path) returns a relative path from the
	// given `from_dir` to the main package root.
	//
	// `from_dir` is itself given as relative path from the main package root
	// and it is allowed to have ".." in it and be outside of the main package
	// root (but it should still be within the repository containing the main
	// package).
	//
	// This function essentially reverses the relative path.
	//
	// For example, imagine the following repository layout:
	//   generated/
	//      luci/
	//         project.cfg
	//         ...
	//   starlark/    <- the main package root
	//       main.star
	//
	// Then a reasonable question to ask is what is a relative path from the
	// generated files to the package root. Here `from_dir` will be
	// "../generated/luci" and package_dir("../generated/luci") return value
	// will be "../../starlark".
	//
	// Answering this question requires knowing the path to the main package
	// within its hosting repository. This is what `main_pkg_path` is for.
	//
	// Returns (rel_path, True) on success and ("", False) if the "from_dir" is
	// outside of the repository.
	declNative("package_dir", func(call nativeCall) (starlark.Value, error) {
		var fromDir starlark.String
		var mainPkgPath starlark.String
		if err := call.unpack(2, &fromDir, &mainPkgPath); err != nil {
			return nil, err
		}

		// Cleaned path from the repository root to the requested directory.
		repoPath := path.Join(mainPkgPath.GoString(), fromDir.GoString())
		if strings.HasPrefix(repoPath, "../") {
			return starlark.Tuple{starlark.String(""), starlark.False}, nil
		}

		// Relative path from the requested directory to the main package root.
		// Note that filepath.Rel doesn't actually use cwd nor touches file system,
		// so it is fine to use it.
		rel, err := filepath.Rel(
			filepath.FromSlash(repoPath),
			filepath.FromSlash(mainPkgPath.GoString()),
		)
		if err != nil {
			return starlark.Tuple{starlark.String(""), starlark.False}, nil
		}

		// filepath.Rel seems to produce paths that end in "/." if the second
		// argument is ".". Normalize them by dropping meaningless "/.".
		rel = strings.TrimSuffix(filepath.ToSlash(rel), "/.")

		return starlark.Tuple{
			starlark.String(rel),
			starlark.True,
		}, nil
	})
}
