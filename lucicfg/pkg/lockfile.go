// Copyright 2025 The LUCI Authors.
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

package pkg

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/lucicfg/internal"
	"go.chromium.org/luci/lucicfg/lockfilepb"
)

// LogLockfileDeps logs dependencies stored in the lockfile.
func LogLockfileDeps(ctx context.Context, lockfile *lockfilepb.Lockfile) {
	deps := lockfile.Packages[1:] // skip the main package itself
	if len(deps) == 0 {
		return
	}
	logging.Debugf(ctx, "Dependencies:")
	for _, dep := range deps {
		logging.Debugf(ctx, "  %s:", dep.Name)
		if dep.Source.Repo == "" {
			logging.Debugf(ctx, "    path = %q", dep.Source.Path)
		} else {
			logging.Debugf(ctx, "    repo = %q", dep.Source.Repo)
			logging.Debugf(ctx, "    path = %q", dep.Source.Path)
			logging.Debugf(ctx, "    revision = %q", dep.Source.Revision)
		}
	}
}

// IsLockfileOverridden is true if the lockfile references repos overridden
// via "-repo-override" mechanism.
func IsLockfileOverridden(lockfile *lockfilepb.Lockfile) bool {
	for _, dep := range lockfile.Packages {
		if dep.Source.GetRevision() == OverriddenVersion {
			return true
		}
	}
	return false
}

// CheckLockfileStaleness loads the existing PACKAGE.lock and compares it to the
// given lockfile (using either byte-to-byte or semantic comparison).
func CheckLockfileStaleness(pkgDir string, lockfile *lockfilepb.Lockfile, semanticEq bool) (fresh bool, err error) {
	blob, err := os.ReadFile(filepath.Join(pkgDir, LockfileName))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	case err != nil:
		return false, errors.Fmt("checking %s staleness: %w", LockfileName, err)
	}

	if semanticEq {
		var existing lockfilepb.Lockfile
		if err := protojson.Unmarshal(blob, &existing); err != nil {
			return false, nil // not JSONPB => stale and should be overwritten
		}
		return internal.SemanticProtoEqual(lockfile, &existing), nil
	}

	current, err := serializeLockfile(lockfile)
	if err != nil {
		return false, err
	}
	return bytes.Equal(current, blob), nil
}

// WriteLockfile writes the lockfile for the package in the given directory.
func WriteLockfile(pkgDir string, lockfile *lockfilepb.Lockfile) error {
	blob, err := serializeLockfile(lockfile)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(pkgDir, LockfileName), blob, 0666)
}

// serializeLockfile serializes the lockfile in a reproducible way.
func serializeLockfile(lockfile *lockfilepb.Lockfile) ([]byte, error) {
	blob, err := protojson.Marshal(lockfile)
	if err != nil {
		return nil, errors.Fmt("serializing %s: %w", LockfileName, err)
	}
	// protojson randomly injects spaces into the generate output. Pass it through
	// a formatter to get rid of them.
	var out bytes.Buffer
	if err := json.Indent(&out, blob, "", "\t"); err != nil {
		return nil, errors.Fmt("formatting %s: %w", LockfileName, err)
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
