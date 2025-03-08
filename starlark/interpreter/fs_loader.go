// Copyright 2021 The LUCI Authors.
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
	"context"
	"errors"
	"io/fs"

	"go.starlark.net/starlark"
)

// FSLoader returns a loader that loads files from a fs.FS implementation.
func FSLoader(fsys fs.FS) Loader {
	return func(_ context.Context, path string) (_ starlark.StringDict, src string, err error) {
		switch body, err := fs.ReadFile(fsys, path); {
		case errors.Is(err, fs.ErrNotExist):
			return nil, "", ErrNoModule
		case err != nil:
			return nil, "", err
		default:
			return nil, string(body), nil
		}
	}
}
