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

// Package generate implements 'generate' subcommand.
package generate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/lucicfg"
	"go.chromium.org/luci/lucicfg/buildifier"
	"go.chromium.org/luci/lucicfg/cli/base"
	"go.chromium.org/luci/lucicfg/fileset"
	"go.chromium.org/luci/lucicfg/pkg"
)

// Cmd is 'generate' subcommand.
func Cmd(params base.Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "generate SCRIPT",
		ShortDesc: "interprets a high-level config, generating *.cfg files",
		LongDesc: `Interprets a high-level config, generating *.cfg files.

Writes generated configs to the directory given via -config-dir or via
lucicfg.config(config_dir=...) statement in the script. If it is '-', just
prints them to stdout.

If -validate is given, sends the generated config to LUCI Config service for
validation. This can also be done separately via 'validate' subcommand.

If the generation stage fails, doesn't overwrite any files on disk. If the
generation succeeds, but the validation fails, the new generated files are kept
on disk, so they can be manually examined for reasons they are invalid.
`,
		CommandRun: func() subcommands.CommandRun {
			gr := &generateRun{}
			gr.Init(params)
			gr.AddGeneratorFlags()
			gr.Flags.BoolVar(&gr.force, "force", false, "Rewrite existing output files on disk even if they are semantically equal to generated ones")
			gr.Flags.BoolVar(&gr.validate, "validate", false, "Validate the generate configs by sending them to LUCI Config")
			gr.Flags.StringVar(&gr.emitToStdout, "emit-to-stdout", "",
				"When set to a path, keep generated configs in memory (don't touch disk) and just emit this single config file to stdout")
			return gr
		},
	}
}

type generateRun struct {
	base.Subcommand

	force        bool
	validate     bool
	emitToStdout string
}

type generateResult struct {
	// Meta is the final meta parameters used by the generator.
	Meta *lucicfg.Meta `json:"meta,omitempty"`
	// LinterFindings is linter findings (if enabled).
	LinterFindings []*buildifier.Finding `json:"linter_findings,omitempty"`
	// Validation is per config set validation results (if -validate was used).
	Validation []*lucicfg.ValidationResult `json:"validation,omitempty"`

	// Changed is a list of config files that have changed or been created.
	Changed []string `json:"changed,omitempty"`
	// Unchanged is a list of config files that haven't changed.
	Unchanged []string `json:"unchanged,omitempty"`
	// Deleted is a list of config files deleted from disk due to staleness.
	Deleted []string `json:"deleted,omitempty"`

	// LockfileState is what happened to the PACKAGE.lock.
	//
	// "CHANGED", "UNCHANGED" or "" (if the lockfile was completely ignored e.g.
	// because "-repo-override" was used or this is a legacy package without
	// a lockfile).
	LockfileState string `json:"lockfile_state,omitempty"`
}

func (gr *generateRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !gr.CheckArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, gr, env)
	return gr.Done(gr.run(ctx, args[0]))
}

func (gr *generateRun) run(ctx context.Context, inputFile string) (*generateResult, error) {
	meta := gr.DefaultMeta()
	gen, err := base.GenerateConfigs(ctx, inputFile, &meta, &gr.Meta, gr.Vars, gr.RepoOverrides, gr.RepoOptions)
	if err != nil {
		return nil, err
	}

	state := gen.State
	output := state.Output

	result := &generateResult{Meta: &meta}

	switch {
	case gr.emitToStdout != "":
		// When using -emit-to-stdout, just print the requested file to stdout and
		// do not touch configs on disk. This also overrides `config_dir = "-"`,
		// since we don't want to print two different sources to stdout.
		datum := output.Data[gr.emitToStdout]
		if datum == nil {
			return nil, fmt.Errorf("-emit-to-stdout: no such generated file %q", gr.emitToStdout)
		}
		blob, err := datum.Bytes()
		if err != nil {
			return nil, err
		}
		if _, err := os.Stdout.Write(blob); err != nil {
			return nil, fmt.Errorf("when writing to stdout: %s", err)
		}

	case meta.ConfigDir == "-":
		// Note: the result of this output is generally not parsable and should not
		// be used in any scripting.
		output.DebugDump()

	default:
		// Get rid of stale output in ConfigDir by deleting tracked files that are
		// no longer in the output. Note that if TrackedFiles is empty (default),
		// nothing is deleted, it is the responsibility of lucicfg users to make
		// sure there's no stale output in this case.
		trackedSet, err := fileset.New(meta.TrackedFiles)
		if err != nil {
			return result, err
		}
		tracked, err := fileset.ScanDirectory(meta.ConfigDir, trackedSet)
		if err != nil {
			return result, err
		}
		for _, f := range tracked {
			if _, present := output.Data[f]; !present {
				result.Deleted = append(result.Deleted, f)
				logging.Warningf(ctx, "Deleting tracked file no longer present in the output: %q", f)
				if err := os.Remove(filepath.Join(meta.ConfigDir, filepath.FromSlash(f))); err != nil {
					return result, err
				}
			}
		}
		// Write the new output there.
		result.Changed, result.Unchanged, err = output.Write(meta.ConfigDir, gr.force)
		if err != nil {
			return result, err
		}
		// Update the lockfile if it has changed.
		switch {
		case pkg.IsLockfileOverridden(gen.Lockfile):
			logging.Warningf(ctx, "Leaving %s untouched since this run uses -repo-override", pkg.LockfileName)
		case gen.Package.Definition.Name == pkg.LegacyPackageNamePlaceholder:
			// This is a legacy package without PACKAGE.star. Silently do nothing.
		default:
			fresh, err := pkg.CheckLockfileStaleness(gen.Package.DiskPath, gen.Lockfile, !gr.force)
			switch {
			case err != nil:
				return result, err
			case fresh:
				result.LockfileState = "UNCHANGED"
			default:
				result.LockfileState = "CHANGED"
				if err := pkg.WriteLockfile(gen.Package.DiskPath, gen.Lockfile); err != nil {
					return result, err
				}
			}
		}
	}

	// Optionally validate via RPC and apply linters. This is slow, thus off by
	// default.
	if gr.validate {
		var source []string
		source, err = state.SourcesToLint()
		if err != nil {
			return result, err
		}
		result.LinterFindings, result.Validation, err = base.Validate(ctx, base.ValidateParams{
			Loader:        state.Inputs.Entry.Main,
			Source:        source,
			Output:        output,
			Meta:          meta,
			Root:          state.Inputs.Entry.Local.DiskPath,
			Formatter:     state.Inputs.Entry.Local.Formatter,
			ConfigService: gr.ConfigService,
		})
	}
	return result, err
}
