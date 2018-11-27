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

package interpreter

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/starlarkproto"
)

// FileSystemLoader returns a loader that loads files from the file system.
func FileSystemLoader(root string) Loader {
	root, err := filepath.Abs(root)
	if err != nil {
		panic(err)
	}
	return func(path string) (_ starlark.StringDict, src string, err error) {
		abs := filepath.Join(root, filepath.FromSlash(path))
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return nil, "", errors.Annotate(err, "failed to calculate relative path").Err()
		}
		if strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, "", errors.New("outside the package root")
		}
		body, err := ioutil.ReadFile(abs)
		if os.IsNotExist(err) {
			return nil, "", ErrNoModule
		}
		return nil, string(body), err
	}
}

// MemoryLoader returns a loader that loads files from the given map.
//
// Useful together with 'assets' package to embed Starlark code into the
// interpreter binary.
func MemoryLoader(files map[string]string) Loader {
	return func(path string) (_ starlark.StringDict, src string, err error) {
		body, ok := files[path]
		if !ok {
			return nil, "", ErrNoModule
		}
		return nil, body, nil
	}
}

// ProtoLoader returns a loader that is capable of loading protobuf modules.
//
// Takes a mapping from a starlark module name to a corresponding *.proto file,
// as registered in the process's protobuf registry (look for
// proto.RegisterFile(...) calls in *.pb.go).
//
// For example, passing {"service/api.proto": "go.chromium.org/.../api.proto"}
// and using the resulting loader as @proto package, would allow to load API
// protos via load("@proto//service/api.proto", ...).
//
// This decouples observable Starlark proto module layout from a layout of Go
// packages in the interpreter.
//
// If a requested module is not found in 'modules', returns ErrNoModule. Thus
// 'modules' also acts as a whitelist of what proto packages can be accessed
// from Starlark.
//
// On success the returned dict has a single struct named after the proto
// package. It has all message constructors defined inside. See 'starlarkproto'
// package for more info.
func ProtoLoader(modules map[string]string) Loader {
	return func(path string) (dict starlark.StringDict, _ string, err error) {
		p := modules[path]
		if p == "" {
			return nil, "", ErrNoModule
		}
		dict, err = starlarkproto.LoadProtoModule(p)
		return
	}
}
