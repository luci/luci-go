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
	"bytes"
	"context"
	"os"
	"sort"
	"sync"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// Ensure-file related helpers.

func resolveEnsureFile(ctx context.Context, f *ensure.File, clientOpts clientOptions) (map[string][]pinInfo, ensure.VersionsFile, error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer client.Close(ctx)

	out := ensure.VersionsFile{}
	mu := sync.Mutex{}

	resolver := cipd.Resolver{
		Client:         client,
		VerifyPresence: true,
		Visitor: func(pkg, ver, iid string) {
			mu.Lock()
			out.AddVersion(pkg, ver, iid)
			mu.Unlock()
		},
	}
	results, err := resolver.ResolveAllPlatforms(ctx, f)
	if err != nil {
		return nil, nil, err
	}
	return resolvedFilesToPinMap(results), out, nil
}

func resolvedFilesToPinMap(res map[template.Platform]*ensure.ResolvedFile) map[string][]pinInfo {
	pinMap := map[string][]pinInfo{}
	for plat, resolved := range res {
		for subdir, resolvedPins := range resolved.PackagesBySubdir {
			pins := pinMap[subdir]
			for _, pin := range resolvedPins {
				// Put a copy into 'pins', otherwise they all end up pointing to the
				// same variable living in the outer scope.
				pins = append(pins, pinInfo{
					Pkg:      pin.PackageName,
					Pin:      &pin,
					Platform: plat.String(),
				})
			}
			pinMap[subdir] = pins
		}
	}

	// Sort pins by (package name, platform) for deterministic output.
	for _, v := range pinMap {
		sort.Slice(v, func(i, j int) bool {
			if v[i].Pkg == v[j].Pkg {
				return v[i].Platform < v[j].Platform
			}
			return v[i].Pkg < v[j].Pkg
		})
	}
	return pinMap
}

func loadVersionsFile(path, ensureFile string) (ensure.VersionsFile, error) {
	switch f, err := os.Open(path); {
	case os.IsNotExist(err):
		return nil,
			cipderr.BadArgument.Apply(errors.Fmt("the resolved versions file doesn't exist, "+
				"use 'cipd ensure-file-resolve -ensure-file %q' to generate it", ensureFile))
	case err != nil:
		return nil, cipderr.IO.Apply(errors.Fmt("reading resolved versions file: %w", err))
	default:
		defer f.Close()
		return ensure.ParseVersionsFile(f)
	}
}

func saveVersionsFile(path string, v ensure.VersionsFile) error {
	buf := bytes.Buffer{}
	if err := v.Serialize(&buf); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0666); err != nil {
		return cipderr.IO.Apply(errors.Fmt("writing versions file: %w", err))
	}
	return nil
}
