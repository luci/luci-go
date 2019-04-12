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
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/lucicfg"

	"go.chromium.org/luci/lucicfg/cli/base"
)

// TODO(vadimsh): If the config set is not provided, try to guess it from the
// git repo and location of files within the repo (by comparing them to output
// of GetConfigSets() listing). Present the guess to the end user, so they can
// confirm it a put it into the flag/config.

// Cmd is 'validate' subcommand.
func Cmd(params base.Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "validate [CONFIG_DIR|SCRIPT]",
		ShortDesc: "sends configuration files to LUCI Config service for validation",
		LongDesc: `Sends configuration files to LUCI Config service for validation.

If the first positional argument is a directory, takes all files there and
sends them to LUCI Config service, to be validated as a single config set. The
name of the config set (e.g. "projects/foo") should be provided via -config-set
flag, it is required in this mode.

If the first positional argument is a Starlark file, it is interpreted (same as
with 'generate' subcommand) and the resulting generated configs are compared
to what's already on disk in -config-dir directory. If they are different, the
subcommand exits with non-zero exit code (indicating files on disk are stale).
Otherwise the configs are sent to LUCI Config service for validation, as above.

Passing no positional arguments at all is equivalent to passing the current
working directory, i.e. all files there will be send to LUCI Config for
validation.

When interpreting Starlark script, flags like -config-dir and -config-set work
as overrides for values declared in the script itself via lucicfg.config(...)
statement. See its doc for more details.
		`,
		CommandRun: func() subcommands.CommandRun {
			vr := &validateRun{}
			vr.Init(params)
			vr.AddMetaFlags()
			return vr
		},
	}
}

type validateRun struct {
	base.Subcommand
}

type validateResult struct {
	*lucicfg.ValidationResult

	// Stale is a list of config files on disk that are out-of-date compared to
	// what is produced by the starlark script.
	//
	// When non-empty, means invocation of "lucicfg generate ..." will either
	// update or delete all these files.
	Stale []string `json:"stale,omitempty"`
}

func (vr *validateRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !vr.CheckArgs(args, 0, 1) {
		return 1
	}

	// The first argument is either a directory with a config set to validate,
	// or a entry point *.star file. Default is to validate the config set in
	// the cwd.
	target := ""
	if len(args) == 1 {
		target = args[0]
	} else {
		var err error
		if target, err = os.Getwd(); err != nil {
			return vr.Done(nil, err)
		}
	}

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
	case vr.Meta.ConfigServiceHost == "":
		return nil, base.MissingFlagError("-config-service-host")
	case vr.Meta.ConfigSet == "":
		return nil, base.MissingFlagError("-config-set")
	case vr.Meta.WasTouched("config_dir"):
		return nil, base.NewCLIError("-config-dir shouldn't be used, the directory was already given as positional argument")
	}
	configSet, err := lucicfg.ReadConfigSet(dir)
	if err != nil {
		return nil, err
	}
	res, err := base.ValidateConfigs(ctx, configSet, &vr.Meta, vr.ConfigService)
	return &validateResult{ValidationResult: res}, err
}

// validateGenerated executes Starlark script, compares the result to whatever
// is on disk (failing the validation if there's a difference), and then sends
// the output to LUCI Config service for validation.
func (vr *validateRun) validateGenerated(ctx context.Context, path string) (*validateResult, error) {
	meta := vr.DefaultMeta()
	output, err := base.GenerateConfigs(ctx, path, &meta, &vr.Meta)
	if err != nil {
		return nil, err
	}

	result := &validateResult{}

	if meta.ConfigDir != "-" {
		// Find files that are present on disk, but no longer in the output.
		tracked, err := lucicfg.FindTrackedFiles(meta.ConfigDir, meta.TrackedFiles)
		if err != nil {
			return result, err
		}
		for _, f := range tracked {
			if _, present := output.Data[f]; !present {
				result.Stale = append(result.Stale, f)
			}
		}
		// Find files that are newer in the output or do not exist on disk.
		changed, _ := output.Compare(meta.ConfigDir)
		result.Stale = append(result.Stale, changed...)
	}

	if len(result.Stale) != 0 {
		return result, fmt.Errorf(
			"following files on disk are out-of-date: %s.\n"+
				"  Run `lucicfg generate %q` to update them",
			strings.Join(result.Stale, ", "), path)
	}

	// If config set is not set, warn, but carry on. It is OK to use 'validate'
	// to check for diff in configs that do not belong to any config set.
	switch {
	case meta.ConfigServiceHost == "":
		logging.Warningf(ctx, "Config service host is not set, skipping validation against LUCI Config service")
	case meta.ConfigSet == "":
		logging.Warningf(ctx, "Config set name is not set, skipping validation against LUCI Config service")
	default:
		// TODO(vadimsh): Split into multiple config sets and validate them in
		// parallel.
		result.ValidationResult, err = base.ValidateConfigs(ctx, output.AsConfigSet(), &meta, vr.ConfigService)
	}
	return result, err
}
