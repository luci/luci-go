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

	"go.chromium.org/luci/lucicfg/cli/base"
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
			gr.AddMetaFlags()
			gr.Flags.BoolVar(&gr.validate, "validate", false, "Validate the generate configs by sending them to LUCI Config")
			return gr
		},
	}
}

type generateRun struct {
	base.Subcommand

	validate bool
}

type generateResult struct {
	// Digests is a map: config name -> SHA256 of the body. To spot differences.
	Digests map[string]string `json:"digests,omitempty"`
	// Meta is the final meta parameters used by the generator.
	Meta *lucicfg.Meta `json:"meta,omitempty"`
	// Validation contains validation results, if -validate was used.
	Validation *lucicfg.ValidationResult `json:"validation,omitempty"`

	// Changed is a list of config files that have changed or been created.
	Changed []string `json:"changed,omitempty"`
	// Unchanged is a list of config files that haven't changed.
	Unchanged []string `json:"unchanged,omitempty"`
	// Deleted is a list of config files deleted from disk due to staleness.
	Deleted []string `json:"deleted,omitempty"`
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
	output, err := base.GenerateConfigs(ctx, inputFile, &meta, &gr.Meta)
	if err != nil {
		return nil, err
	}

	result := &generateResult{
		Digests: output.Digests(),
		Meta:    &meta,
	}

	if meta.ConfigDir == "-" {
		output.DebugDump()
	} else {
		// Get rid of stale output in ConfigDir by deleting tracked files that are
		// no longer in the output. Note that if TrackedFiles is empty (default),
		// nothing is deleted, it is the responsibility of lucicfg users to make
		// sure there's no stale output in this case.
		tracked, err := lucicfg.FindTrackedFiles(meta.ConfigDir, meta.TrackedFiles)
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
		result.Changed, result.Unchanged, err = output.Write(meta.ConfigDir)
		if err != nil {
			return result, err
		}
	}

	// Optionally validate via RPC. This is slow, thus off by default.
	if gr.validate {
		switch {
		case meta.ConfigServiceHost == "":
			return result, fmt.Errorf("can't validate the config, lucicfg.config(config_service_host=...) is not set")
		case meta.ConfigSet == "":
			return result, fmt.Errorf("can't validate the config, lucicfg.config(config_set=...) is not set")
		}
		// TODO(vadimsh): Split into multiple config sets and validate them in
		// parallel.
		result.Validation, err = base.ValidateConfigs(ctx, output.AsConfigSet(), &meta, gr.ConfigService)
		if err != nil {
			return result, err
		}
	}

	return result, nil
}
