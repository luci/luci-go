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
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/maruel/subcommands"

	config "go.chromium.org/luci/common/api/luci_config/config/v1"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/lucicfg/cli/base"
)

// TODO(vadimsh): If the config set is not provided, try to guess it from the
// git repo and location of files within the repo (by comparing them to output
// of GetConfigSets() listing). Present the guess to the end user, so they can
// confirm it a put it into the flag/config.

// Cmd is 'validate' subcommand.
func Cmd(params base.Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "validate -config-set CONFIG_SET [CONFIG_DIR]",
		ShortDesc: "sends files under CONFIG_DIR (or CWD if not set) to LUCI Config service for validation",
		CommandRun: func() subcommands.CommandRun {
			vr := &validateRun{}
			vr.Init(params)
			vr.AddValidationFlags()
			return vr
		},
	}
}

type validateRun struct {
	base.Subcommand
}

type validateResult struct {
	Messages []*config.ComponentsConfigEndpointValidationMessage `json:"messages"`
}

func (vr *validateRun) checkArgs(args []string) bool {
	switch {
	case !vr.CheckArgs(args, 0, 1):
		return false
	case vr.Meta.ConfigServiceHost == "":
		vr.ReportMissingFlag("-config-service-host")
	case vr.Meta.ConfigSet == "":
		// -config-set flag is required for now. Later, when `validate` learns to
		// generate configs first, it may be set on Starlark side.
		vr.ReportMissingFlag("-config-set")
	default:
		return true // all good
	}
	return false // something is not good
}

func (vr *validateRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !vr.checkArgs(args) {
		return 1
	}

	// Grab ConfigDir from first arg, defaulting to cwd.
	if len(args) == 1 {
		vr.Meta.ConfigDir = args[0]
	} else {
		configDir, err := os.Getwd()
		if err != nil {
			return vr.Done(nil, err)
		}
		vr.Meta.ConfigDir = configDir
	}

	ctx := cli.GetContext(a, vr, env)

	// Construct the service outside run to improve testability.
	svc, err := vr.ConfigService(ctx)
	if err != nil {
		return vr.Done(nil, err)
	}

	return vr.Done(vr.run(ctx, svc))
}

// TODO(vadimsh): Move this into configset.go, so it can be reused from commands
// that generate configs.

// run recursively searches vr.Meta.ConfigDir for config files, calls
// svc.ValidateConfig() on them and aggregates the results.
func (vr *validateRun) run(ctx context.Context, svc *config.Service) (*validateResult, error) {
	validateRequest, err := vr.constructRequest()
	if err != nil {
		return nil, err
	}
	resp, err := svc.ValidateConfig(validateRequest).Context(ctx).Do()
	if err != nil {
		return &validateResult{}, fmt.Errorf("error validating configs: %v", err)
	}
	return processResponse(ctx, resp, vr.Meta.FailOnWarnings)
}

// processResponses produces a validateResult that contains all the messages
// from resp.
//
// Returns an error if any files were invalid or failed to be checked.
func processResponse(ctx context.Context, resp *config.LuciConfigValidateConfigResponseMessage, failOnWarnings bool) (*validateResult, error) {
	fail := false
	for _, message := range resp.Messages {
		lvl := logging.Info
		if message.Severity == "WARNING" {
			lvl = logging.Warning
			if failOnWarnings {
				fail = true
			}
		} else if message.Severity == "ERROR" || message.Severity == "CRITICAL" {
			lvl = logging.Error
			fail = true
		}
		logging.Logf(ctx, lvl, "%s: %s", message.Path, message.Text)
	}
	if fail {
		return &validateResult{resp.Messages}, fmt.Errorf("some files were invalid")
	}
	return &validateResult{resp.Messages}, nil
}

// constructRequest searches the vr.Meta.ConfigDir for config files and
// constructs a LuciConfigValidateConfigRequestMessage.
func (vr *validateRun) constructRequest() (*config.LuciConfigValidateConfigRequestMessage, error) {
	var configFiles []*config.LuciConfigValidateConfigRequestMessageFile
	err := filepath.Walk(vr.Meta.ConfigDir,
		func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() { // Walk will handle recursion
				return nil
			}
			content, err := ioutil.ReadFile(p)
			if err != nil {
				return err
			}
			relPath, err := filepath.Rel(vr.Meta.ConfigDir, p)
			if err != nil {
				return err
			}
			configFiles = append(configFiles, &config.LuciConfigValidateConfigRequestMessageFile{
				Content: base64.StdEncoding.EncodeToString(content),
				Path:    filepath.ToSlash(relPath),
			})
			return nil
		})
	if err != nil {
		return nil, err
	}

	return &config.LuciConfigValidateConfigRequestMessage{
		ConfigSet: vr.Meta.ConfigSet,
		Files:     configFiles,
	}, nil
}
