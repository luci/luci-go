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
			vr.Init(params, true)
			vr.Flags.StringVar(&vr.configSet, "config-set", "<name>", "Name of the config set to validate against.")
			return vr
		},
	}
}

type validateRun struct {
	base.Subcommand

	configSet string
	configDir string
}

type validateResult struct {
	ErrorMessages []*config.ComponentsConfigEndpointValidationMessage `json:"error_messages"`
}

func (vr *validateRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !vr.CheckArgs(args, 0, 1) {
		return 1
	}
	if len(args) == 1 {
		vr.configDir = args[0]
	} else {
		configDir, err := os.Getwd()
		if err != nil {
			return vr.Done(nil, err)
		}
		vr.configDir = configDir
	}
	ctx := cli.GetContext(a, vr, env)
	// Construct the service outside run to improve testability.
	svc, err := vr.ConfigService(ctx)
	if err != nil {
		return vr.Done(nil, err)
	}
	return vr.Done(vr.run(ctx, svc))
}

// run recursively searches vr.configDir for config files, calls svc.ValidateConfig() on them
// and aggregates the results.
func (vr *validateRun) run(ctx context.Context, svc *config.Service) (*validateResult, error) {
	validateRequest, err := vr.constructRequest()
	if err != nil {
		return nil, err
	}
	resp, err := svc.ValidateConfig(validateRequest).Context(ctx).Do()
	return processResponse(ctx, resp, err)
}

// processResponses produces a validateResult that contains all the messages from resp.
// Returns an error if any files were invalid or failed to be checked.
func processResponse(ctx context.Context, resp *config.LuciConfigValidateConfigResponseMessage, err error) (*validateResult, error) {
	if err != nil {
		return &validateResult{}, fmt.Errorf("error validating configs: %v", err)
	}
	if len(resp.Messages) == 0 {
		return &validateResult{}, nil
	}
	for _, message := range resp.Messages {
		lvl := logging.Info
		if message.Severity == "WARNING" {
			lvl = logging.Warning
		} else if message.Severity == "ERROR" || message.Severity == "CRITICAL" {
			lvl = logging.Error
		}
		logging.Logf(ctx, lvl, "%s: %s", message.Path, message.Text)
	}
	return &validateResult{resp.Messages}, fmt.Errorf("Some files were invalid")
}

// constructRequest searches the vr.configDir for config files and constructs a
// LuciConfigValidateConfigRequestMessage.
func (vr *validateRun) constructRequest() (*config.LuciConfigValidateConfigRequestMessage, error) {
	var configFiles []*config.LuciConfigValidateConfigRequestMessageFile
	err := filepath.Walk(vr.configDir,
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
			relPath, err := filepath.Rel(vr.configDir, p)
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
		ConfigSet: vr.configSet,
		Files:     configFiles,
	}, nil
}
