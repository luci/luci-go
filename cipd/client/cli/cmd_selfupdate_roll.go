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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/digests"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

const digestsSfx = ".digests"

////////////////////////////////////////////////////////////////////////////////
// 'selfupdate-roll' subcommand.

func cmdSelfUpdateRoll(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "selfupdate-roll -version-file <path> (-version <version> | -check)",
		ShortDesc: "generates or checks the client version and *.digests files",
		LongDesc: "Generates or checks the client version and *.digests files.\n\n" +
			"When -version is specified, takes its value as CIPD client version, " +
			"resolves it into a list of hashes of the client binary at this version " +
			"for all known platforms, and (on success) puts the version into a file " +
			"specified by -version-file (referred to as <version-file> below), and " +
			"all hashes into <version-file>.digests file. They are later used by " +
			"'selfupdate -version-file <version-file>' to verify validity of the " +
			"fetched binary.\n\n" +
			"If -version is not specified, reads it from <version-file> and generates " +
			"<version-file>.digests file based on it.\n\n" +
			"When using -check, just verifies hashes in the <version-file>.digests " +
			"file match the version recorded in the <version-file>.",
		CommandRun: func() subcommands.CommandRun {
			c := &selfupdateRollRun{}

			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "", "Version of the client to roll to.")
			c.Flags.StringVar(&c.versionFile, "version-file", "<version-file>",
				"Indicates the path to a file with the version (<version-file> itself) and "+
					"the path to the file with pinned hashes of the CIPD binary (<version-file>.digests file).")
			c.Flags.BoolVar(&c.check, "check", false, "If set, checks that the file with "+
				"pinned hashes of the CIPD binary (<version-file>.digests file) is up-to-date.")
			return c
		},
	}
}

type selfupdateRollRun struct {
	cipdSubcommand
	clientOptions

	version     string
	versionFile string
	check       bool
}

func (c *selfupdateRollRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}

	ctx := cli.GetContext(a, c, env)
	client, err := c.clientOptions.makeCIPDClient(ctx, "", nil)
	if err != nil {
		return c.done(nil, err)
	}
	defer client.Close(ctx)

	if c.check {
		if c.version != "" {
			return c.done(nil, makeCLIError("-version should not be used in -check mode"))
		}
		version, err := loadClientVersion(c.versionFile)
		if err != nil {
			return c.done(nil, err)
		}
		return c.doneWithPins(checkClientDigests(ctx, client, c.versionFile+digestsSfx, version))
	}

	// Grab the version from the command line and fallback to the -version-file
	// otherwise. The fallback is useful when we just want to regenerate *.digests
	// without touching the version file itself.
	version := c.version
	if version == "" {
		var err error
		version, err = loadClientVersion(c.versionFile)
		if err != nil {
			return c.done(nil, err)
		}
	}

	// It really makes sense to pin only tags. Warn about that. Still proceed,
	// maybe users are using refs and do not move them by convention.
	switch {
	case common.ValidateInstanceID(version, common.AnyHash) == nil:
		return c.done(nil,
			cipderr.BadArgument.Apply(errors.New("expecting a version identifier that can be "+
				"resolved for all per-platform CIPD client packages, not a concrete instance ID")))
	case common.ValidateInstanceTag(version) != nil:
		fmt.Printf(
			"WARNING! Version %q is not a tag. The hash pinning in *.digests file is "+
				"only useful for unmovable version identifiers. Proceeding, assuming "+
				"the immutability of %q is maintained manually. If it moves, selfupdate "+
				"will break due to *.digests file no longer matching the packages!\n\n",
			version, version)
	}

	pins, err := generateClientDigests(ctx, client, c.versionFile+digestsSfx, version)
	if err != nil {
		return c.doneWithPins(pins, err)
	}

	if c.version != "" {
		if err := os.WriteFile(c.versionFile, []byte(c.version+"\n"), 0666); err != nil {
			return c.done(nil, cipderr.IO.Apply(errors.Fmt("writing the client version file: %w", err)))
		}
	}

	return c.doneWithPins(pins, nil)
}

////////////////////////////////////////////////////////////////////////////////

func generateClientDigests(ctx context.Context, client cipd.Client, path, version string) ([]pinInfo, error) {
	digests, pins, err := assembleClientDigests(ctx, client, version)
	if err != nil {
		return pins, err
	}

	buf := bytes.Buffer{}
	versionFileName := strings.TrimSuffix(filepath.Base(path), digestsSfx)
	if err := digests.Serialize(&buf, version, versionFileName); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0666); err != nil {
		return nil, cipderr.IO.Apply(errors.Fmt("writing client digests file: %w", err))
	}

	fmt.Printf("The pinned client hashes have been written to %s.\n\n", filepath.Base(path))
	return pins, nil
}

func checkClientDigests(ctx context.Context, client cipd.Client, path, version string) ([]pinInfo, error) {
	existing, err := loadClientDigests(path)
	if err != nil {
		return nil, err
	}
	digests, pins, err := assembleClientDigests(ctx, client, version)
	if err != nil {
		return pins, err
	}
	if !digests.Equal(existing) {
		base := filepath.Base(path)
		return nil,

			cipderr.Stale.Apply(errors.Fmt("the file with pinned client hashes (%s) is stale, "+
				"use 'cipd selfupdate-roll -version-file %s' to update it",
				base, strings.TrimSuffix(base, digestsSfx)))
	}
	fmt.Printf("The file with pinned client hashes (%s) is up-to-date.\n\n", filepath.Base(path))
	return pins, nil
}

// assembleClientDigests produces the digests file by making backend RPCs.
func assembleClientDigests(ctx context.Context, c cipd.Client, version string) (*digests.ClientDigestsFile, []pinInfo, error) {
	if !strings.HasSuffix(cipd.ClientPackage, "/${platform}") {
		panic(fmt.Sprintf("client package template (%q) is expected to end with '${platform}'", cipd.ClientPackage))
	}

	out := &digests.ClientDigestsFile{}
	mu := sync.Mutex{}

	// Ask the backend to give us hashes of the client binary for all platforms.
	pins, err := performBatchOperation(ctx, batchOperation{
		client:        c,
		packagePrefix: strings.TrimSuffix(cipd.ClientPackage, "${platform}"),
		callback: func(pkg string) (common.Pin, error) {
			pin, err := c.ResolveVersion(ctx, pkg, version)
			if err != nil {
				return common.Pin{}, err
			}
			desc, err := c.DescribeClient(ctx, pin)
			if err != nil {
				return common.Pin{}, err
			}
			mu.Lock()
			defer mu.Unlock()
			plat := pkg[strings.LastIndex(pkg, "/")+1:]
			if err := out.AddClientRef(plat, desc.Digest); err != nil {
				return common.Pin{}, err
			}
			return pin, nil
		},
	})
	switch {
	case err != nil:
		return nil, pins, err
	case hasErrors(pins):
		return nil, pins, cipderr.RPC.Apply(errors.New("failed to obtain the client binary digest for all platforms"))
	}

	out.Sort()
	return out, pins, nil
}
