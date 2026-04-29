// Copyright 2026 The LUCI Authors.
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

package cli

import (
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd/digests"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// loadClientVersion reads a version string from a file.
func loadClientVersion(path string) (string, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return "", cipderr.IO.Apply(errors.Fmt("reading client version file: %w", err))
	}
	version := strings.TrimSpace(string(blob))
	if err := common.ValidateInstanceVersion(version); err != nil {
		return "", err
	}
	return version, nil
}

// loadClientDigests loads the *.digests file with client binary hashes.
func loadClientDigests(path string) (*digests.ClientDigestsFile, error) {
	switch f, err := os.Open(path); {
	case os.IsNotExist(err):
		base := filepath.Base(path)
		return nil,

			cipderr.Stale.Apply(errors.Fmt("the file with pinned client hashes (%s) doesn't exist, "+
				"use 'cipd selfupdate-roll -version-file %s' to generate it",
				base, strings.TrimSuffix(base, digestsSfx)))
	case err != nil:
		return nil, cipderr.IO.Apply(errors.Fmt("error reading client digests file: %w", err))
	default:
		defer f.Close()
		return digests.ParseClientDigestsFile(f)
	}
}
