// Copyright 2017 The LUCI Authors.
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

package ensure

import (
	"io"
	"os"
	"path/filepath"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/common/cipderr"
)

// LoadEnsureFile loads the ensure file from the given path or stdin (if 'path'
// is "-").
//
// If the ensure file has $ResolvedVersions directive, the returned File will
// have ResolvedVersions field set to an absolute path to the resolved versions
// file. Its presence or correctness is not checked.
func LoadEnsureFile(path string) (*File, error) {
	var f io.ReadCloser
	var basePath string
	if path == "-" {
		var err error
		if basePath, err = os.Getwd(); err != nil {
			return nil, errors.Annotate(err, "failed to get cwd").Tag(cipderr.IO).Err()
		}
		f = os.Stdin
	} else {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, errors.Annotate(err, "bad ensure file path").Tag(cipderr.BadArgument).Err()
		}
		basePath = filepath.Dir(abs)
		if f, err = os.Open(path); err != nil {
			return nil, errors.Annotate(err, "opening ensure file").Tag(cipderr.IO).Err()
		}
		defer f.Close()
	}

	pf, err := ParseFile(f)
	if err != nil {
		return nil, err
	}

	if pf.ResolvedVersions != "" {
		pf.ResolvedVersions = filepath.FromSlash(pf.ResolvedVersions)
		if !filepath.IsAbs(pf.ResolvedVersions) {
			pf.ResolvedVersions = filepath.Join(basePath, pf.ResolvedVersions)
		}
	}

	return pf, nil
}
