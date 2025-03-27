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

package base

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/lucicfg"
	"go.chromium.org/luci/lucicfg/pkg"
)

// GenerateConfigs executes the Starlark script and assembles final values for
// meta config.
//
// It is a common part of subcommands that generate configs.
//
// 'meta' is initial Meta config with default parameters, it will be mutated
// in-place to contain the final parameters (based on lucicfg.config(...) calls
// in Starlark and the config populated via CLI flags, passed as 'flags').
// 'flags' are also mutated in-place to rebase ConfigDir onto cwd.
//
// 'vars' are a collection of k=v pairs passed via CLI flags as `-var k=v`. They
// are used to pre-set lucicfg.var(..., exposed_as=<k>) variables.
//
// 'repoOverrides' are a collection of k=v pairs passed via CLI flags as
// `-repo-overrides k=v`. They are used to setup local overrides of remote
// dependencies
func GenerateConfigs(ctx context.Context, inputFile string, meta, flags *lucicfg.Meta, vars map[string]string, repoOverrides map[string]string) (*lucicfg.State, error) {
	abs, err := filepath.Abs(inputFile)
	if err != nil {
		return nil, err
	}

	// Make sure the input file exists, to make the error message in this case be
	// more humane. lucicfg.Generate will formulate this error as "no such module"
	// which looks confusing.
	//
	// Also check that the script starts with "#!..." line, indicating it is
	// executable. This gives a hint in case lucicfg is mistakenly invoked with
	// some library script. Executing such scripts directly usually causes very
	// confusing errors.
	switch f, err := os.Open(abs); {
	case os.IsNotExist(err):
		return nil, fmt.Errorf("no such file: %s", inputFile)
	case err != nil:
		return nil, err
	default:
		yes, err := startsWithShebang(f)
		f.Close()
		switch {
		case err != nil:
			return nil, err
		case !yes:
			fmt.Fprintf(os.Stderr,
				`================================= WARNING =================================
Body of the script %s doesn't start with "#!".

It is likely not a correct entry point script and lucicfg execution will fail
with cryptic errors or unexpected results. Many configs consist of more than
one *.star file, but there's usually only one entry point script that should
be passed to lucicfg.

If it is the correct script, make sure it starts with the following line to
indicate it is executable (and remove this warning):

    #!/usr/bin/env lucicfg

You may also optionally set +x flag on it, but this is not required.
===========================================================================

`, filepath.Base(abs))
		}
	}

	logging.Infof(ctx, "Generating configs using %s...", lucicfg.UserAgent)

	// Convert stringy repoOverrides into a structured form.
	var overrides []*pkg.RepoOverride
	for _, repo := range slices.Sorted(maps.Keys(repoOverrides)) {
		override, err := pkg.RepoOverrideFromSpec(repo, repoOverrides[repo])
		if err != nil {
			return nil, NewCLIError("bad -repo-override %s=%s: %s", repo, repoOverrides[repo], err)
		}
		overrides = append(overrides, override)
	}

	// TODO: Use Cache to construct a pkg.RepoManager that knows how to fetch
	// remote repos into that cache.
	cache := lucicfg.OpenCache(ctx)
	if _, err := cache.Subdir("remote"); err != nil {
		return nil, err
	}

	// Load the main package with dependencies from disk.
	entry, err := pkg.EntryOnDisk(ctx, abs, &pkg.ErroringRepoManager{
		Error: errors.New("remote packages aren't implemented yet"),
	}, overrides)
	if err != nil {
		return nil, err
	}

	// Generate everything, storing the result in memory.
	state, err := lucicfg.Generate(ctx, lucicfg.Inputs{
		Entry: entry,
		Meta:  meta,
		Vars:  vars,
	})
	if err != nil {
		return nil, err
	}

	// Config dir in the default meta, and if set from Starlark, is relative to
	// the main package root. It is relative to cwd ONLY when explicitly provided
	// via -config-dir CLI flag. Note that ".." is allowed.
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	meta.RebaseConfigDir(entry.Local.DiskPath)
	state.Meta.RebaseConfigDir(entry.Local.DiskPath)
	flags.RebaseConfigDir(cwd)

	// Figure out the final meta config: values set via starlark override
	// defaults, and values passed explicitly via CLI flags override what is
	// in starlark.
	meta.PopulateFromTouchedIn(&state.Meta)
	meta.PopulateFromTouchedIn(flags)
	meta.Log(ctx)

	// Discard changes to the non-tracked files by loading their original bodies
	// (if any) from disk. We replace them to make sure the output is still
	// validated as a whole, it is just only partially generated in this case.
	if len(meta.TrackedFiles) != 0 {
		if err := state.Output.DiscardChangesToUntracked(ctx, meta.TrackedFiles, meta.ConfigDir); err != nil {
			return nil, err
		}
	}

	return state, nil
}

func startsWithShebang(r io.Reader) (bool, error) {
	buf := make([]byte, 2)
	switch _, err := io.ReadFull(r, buf); {
	case err == io.EOF || err == io.ErrUnexpectedEOF:
		return false, nil // the file is smaller than 2 bytes
	case err != nil:
		return false, err
	default:
		return bytes.Equal(buf, []byte("#!")), nil
	}
}
