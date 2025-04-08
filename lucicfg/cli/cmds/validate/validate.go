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

// Package validate implements 'validate' subcommand.
package validate

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/lucicfg"
	"go.chromium.org/luci/lucicfg/buildifier"
	"go.chromium.org/luci/lucicfg/cli/base"
	"go.chromium.org/luci/lucicfg/fileset"
	"go.chromium.org/luci/lucicfg/pkg"
)

// Cmd is 'validate' subcommand.
func Cmd(params base.Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "validate [CONFIG_DIR|SCRIPT]",
		ShortDesc: "sends configuration files to LUCI Config service for validation",
		LongDesc: `Sends configuration files to LUCI Config service for validation.

If the first positional argument is a directory, takes all files there and
sends them to LUCI Config service, to be validated as a single config set. The
name of the config set (e.g. "projects/foo") MUST be provided via -config-set
flag, it is required in this mode.

If the first positional argument is a Starlark file, it is interpreted (as with
'generate' subcommand) and the resulting generated configs are compared to
what's already on disk in -config-dir directory.

By default uses semantic comparison (i.e. config files on disk are deserialized
and compared to the generated files as objects). This is useful to ignore
insignificant formatting changes that may appear due to differences between
lucicfg versions. If -strict is used, compares files as byte blobs. In this case
'validate' detects no changes if and only if 'generate' produces no diff.

If configs on disk are different from the generated ones, the subcommand exits
with non-zero exit code. Otherwise the configs are sent to LUCI Config service
for validation. Partitioning into config sets is specified in the Starlark code
in this case, -config-set flag is rejected if given.

When interpreting Starlark script, flags like -config-dir and -fail-on-warnings
work as overrides for values declared in the script via lucicfg.config(...)
statement. See its doc for more details.
		`,
		CommandRun: func() subcommands.CommandRun {
			vr := &validateRun{}
			vr.Init(params)
			vr.AddGeneratorFlags()
			vr.Flags.StringVar(&vr.configSet, "config-set", "",
				"Name of the config set to validate against when validating existing *.cfg configs.")
			vr.Flags.BoolVar(&vr.strict, "strict", false,
				"Use byte-by-byte comparison instead of comparing configs as proto messages.")
			return vr
		},
	}
}

type validateRun struct {
	base.Subcommand

	configSet string // used only when validating existing *.cfg
	strict    bool   // -strict flag
}

type validateResult struct {
	// Meta is the final meta parameters used by the generator.
	Meta *lucicfg.Meta `json:"meta,omitempty"`
	// LinterFindings is linter findings (if enabled).
	LinterFindings []*buildifier.Finding `json:"linter_findings,omitempty"`
	// Validation is per config set validation results.
	Validation []*lucicfg.ValidationResult `json:"validation"`

	// Stale is a list of config files on disk that are out-of-date compared to
	// what is produced by the starlark script.
	//
	// When non-empty, means invocation of "lucicfg generate ..." will either
	// update or delete all these files.
	Stale []string `json:"stale,omitempty"`

	// LockfileState is what state the PACKAGE.lock is in.
	//
	// "CHANGED", "UNCHANGED" or "" (if the lockfile was completely ignored e.g.
	// because "-repo-override" was used or this is a legacy package without
	// a lockfile).
	LockfileState string `json:"lockfile_state,omitempty"`
}

func (vr *validateRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !vr.CheckArgs(args, 1, 1) {
		return 1
	}

	// The first argument is either a directory with a config set to validate,
	// or a entry point *.star file.
	target := args[0]

	// If 'target' is a directory, it is a directory with generated files we
	// need to validate. If it is a file, it is *.star file to use to generate
	// configs in memory and compare them to whatever is on disk.
	ctx := cli.GetContext(a, vr, env)
	switch fi, err := os.Stat(target); {
	case os.IsNotExist(err):
		return vr.Done(nil, fmt.Errorf("no such file: %s", target))
	case err != nil:
		return vr.Done(nil, err)
	case fi.Mode().IsDir():
		return vr.Done(vr.validateExisting(ctx, target))
	default:
		return vr.Done(vr.validateGenerated(ctx, target))
	}
}

// validateExisting validates an existing config set on disk, whatever it may
// be.
//
// Verifies -config-set flag was used, since it is the only way to provide the
// name of the config set to verify against in this case.
//
// Also verifies -config-dir is NOT used, since it is redundant: the directory
// is passed through the positional arguments.
func (vr *validateRun) validateExisting(ctx context.Context, dir string) (*validateResult, error) {
	switch {
	case vr.configSet == "":
		return nil, base.MissingFlagError("-config-set")
	case vr.Meta.ConfigServiceHost == "":
		return nil, base.MissingFlagError("-config-service-host")
	case vr.Meta.WasTouched("config_dir"):
		return nil, base.NewCLIError("-config-dir shouldn't be used, the directory was already given as positional argument")
	}
	configSet, err := lucicfg.ReadConfigSet(dir, vr.configSet)
	if err != nil {
		return nil, err
	}
	_, res, err := base.Validate(ctx, base.ValidateParams{
		Output:        configSet.AsOutput("."),
		Meta:          vr.Meta,
		ConfigService: vr.ConfigService,
	})
	return &validateResult{Validation: res}, err
}

// validateGenerated executes Starlark script, compares the result to whatever
// is on disk (failing the validation if there's a difference), and then sends
// the output to LUCI Config service for validation.
func (vr *validateRun) validateGenerated(ctx context.Context, path string) (*validateResult, error) {
	// -config-set flag must not be used in this mode, config sets are defined
	// on Starlark level.
	if vr.configSet != "" {
		return nil, base.NewCLIError("-config-set can't be used when validating Starlark-based configs")
	}

	meta := vr.DefaultMeta()
	gen, err := base.GenerateConfigs(ctx, path, &meta, &vr.Meta, vr.Vars, vr.RepoOverrides)
	if err != nil {
		return nil, err
	}

	state := gen.State
	output := state.Output

	result := &validateResult{Meta: &meta}

	if meta.ConfigDir != "-" {
		// Find files that are present on disk, but no longer in the output.
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
				result.Stale = append(result.Stale, f)
			}
		}

		// Find files that are newer in the output or do not exist on disk. Do
		// semantic comparison for protos, unless -strict is set.
		cmp, err := output.Compare(meta.ConfigDir, !vr.strict)
		if err != nil {
			return result, err
		}
		for name, res := range cmp {
			if res == lucicfg.Different {
				result.Stale = append(result.Stale, name)
			}
		}
		sort.Strings(result.Stale)

		// Ask the user to regenerate files if they are different.
		if len(result.Stale) != 0 {
			return result, fmt.Errorf(
				"the following files need to be regenerated: %s.\n"+
					"  Run `lucicfg generate %q` to update them.",
				strings.Join(result.Stale, ", "), path)
		}

		// We want to make sure the *exact* files we have on disk pass the server
		// validation (even if they are, perhaps, semantically identical to files in
		// 'output', as we have just checked). Replace the generated output with
		// what's on disk.
		if err := output.Read(meta.ConfigDir); err != nil {
			return result, err
		}
	}

	// Make sure the lockfile on disk is up-to-date.
	switch {
	case pkg.IsLockfileOverridden(gen.Lockfile):
		logging.Warningf(ctx, "Leaving %s untouched since this run uses -repo-override", pkg.LockfileName)
	case gen.Package.Definition.Name == pkg.LegacyPackageNamePlaceholder:
		// This is a legacy package without PACKAGE.star. Silently do nothing.
	default:
		fresh, err := pkg.CheckLockfileStaleness(gen.Package.DiskPath, gen.Lockfile, !vr.strict)
		switch {
		case err != nil:
			return result, err
		case fresh:
			result.LockfileState = "UNCHANGED"
		default:
			result.LockfileState = "CHANGED"
			return result, fmt.Errorf(
				"the PACKAGE.lock file on disk is stale.\n"+
					"  Run `lucicfg generate %q` to update it.", path)
		}
	}

	// Apply local linters and validate outputs via LUCI Config RPC. This silently
	// skips configs not belonging to any config sets.
	result.LinterFindings, result.Validation, err = base.Validate(ctx, base.ValidateParams{
		Loader:        state.Inputs.Entry.Main,
		Source:        state.Visited,
		Output:        output,
		Meta:          meta,
		Root:          state.Inputs.Entry.Local.DiskPath,
		Formatter:     state.Inputs.Entry.Local.Formatter,
		ConfigService: vr.ConfigService,
	})
	return result, err
}
