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
	"context"
	"fmt"
	"os"
	"strings"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// Support for running operations concurrently.

// batchOperation defines what to do with a packages matching a prefix.
type batchOperation struct {
	client        cipd.Client
	packagePrefix string   // a package name or a prefix
	packages      []string // packages to operate on, overrides packagePrefix
	callback      func(pkg string) (common.Pin, error)
}

// expandPkgDir takes a package name or '<prefix>/' and returns a list
// of matching packages (asking backend if necessary). Doesn't recurse, returns
// only direct children.
func expandPkgDir(ctx context.Context, c cipd.Client, packagePrefix string) ([]string, error) {
	if !strings.HasSuffix(packagePrefix, "/") {
		return []string{packagePrefix}, nil
	}
	pkgs, err := c.ListPackages(ctx, packagePrefix, false, false)
	if err != nil {
		return nil, err
	}
	// Skip directories.
	var out []string
	for _, p := range pkgs {
		if !strings.HasSuffix(p, "/") {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil,
			cipderr.RPC.WithDetails(cipderr.Details{
				Package: packagePrefix,
			}).Apply(errors.Fmt("no packages under %s", packagePrefix))
	}
	return out, nil
}

// performBatchOperation expands a package prefix into a list of packages and
// calls callback for each of them (concurrently) gathering the results.
//
// Returns an error only if the prefix expansion fails. Errors from individual
// operations are returned through []pinInfo, use hasErrors to check them.
func performBatchOperation(ctx context.Context, op batchOperation) ([]pinInfo, error) {
	op.client.BeginBatch(ctx)
	defer op.client.EndBatch(ctx)

	pkgs := op.packages
	if len(pkgs) == 0 {
		var err error
		pkgs, err = expandPkgDir(ctx, op.client, op.packagePrefix)
		if err != nil {
			return nil, err
		}
	}
	return callConcurrently(pkgs, func(pkg string) pinInfo {
		pin, err := op.callback(pkg)
		if err != nil {
			return pinInfo{
				Pkg:          pkg,
				Error:        err.Error(),
				ErrorCode:    cipderr.ToCode(err),
				ErrorDetails: cipderr.ToDetails(err),
				err:          err,
			}
		}
		return pinInfo{Pkg: pkg, Pin: &pin}
	}), nil
}

func callConcurrently(pkgs []string, callback func(pkg string) pinInfo) []pinInfo {
	// Push index through channel to make results ordered as 'pkgs'.
	ch := make(chan struct {
		int
		pinInfo
	})
	for idx, pkg := range pkgs {
		go func(idx int, pkg string) {
			ch <- struct {
				int
				pinInfo
			}{idx, callback(pkg)}
		}(idx, pkg)
	}
	pins := make([]pinInfo, len(pkgs))
	for range pkgs {
		res := <-ch
		pins[res.int] = res.pinInfo
	}
	return pins
}

func printPinsAndError(pinMap map[string][]pinInfo) {
	for subdir, pins := range pinMap {
		hasPins := false
		hasErrors := false
		for _, p := range pins {
			if p.Error != "" {
				hasErrors = true
			} else if p.Pin != nil {
				hasPins = true
			}
		}
		subdirString := ""
		if (hasPins || hasErrors) && (len(pinMap) > 1 || subdir != "") {
			// only print this if it's not the root subdir, or there's more than one
			// subdir in pinMap.
			subdirString = fmt.Sprintf(" (subdir %q)", subdir)
		}
		if hasPins {
			fmt.Printf("Packages%s:\n", subdirString)
			for _, p := range pins {
				if p.Error != "" || p.Pin == nil {
					continue
				}
				plat := ""
				if p.Platform != "" {
					plat = fmt.Sprintf(" (for %s)", p.Platform)
				}
				tracking := ""
				if p.Tracking != "" {
					tracking = fmt.Sprintf(" (tracking %q)", p.Tracking)
				}
				fmt.Printf("  %s%s%s\n", p.Pin, plat, tracking)
			}
		}
		if hasErrors {
			fmt.Fprintf(os.Stderr, "Errors%s:\n", subdirString)
			for _, p := range pins {
				if p.Error != "" {
					fmt.Fprintf(os.Stderr, "  %s: %s.\n", p.Pkg, p.Error)
				}
			}
		}
	}
}

func hasErrors(pins []pinInfo) bool {
	for _, p := range pins {
		if p.Error != "" {
			return true
		}
	}
	return false
}
