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

package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sync"

	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/maruel/subcommands"
	config "go.chromium.org/luci/common/api/luci_config/config/v1"

	"go.chromium.org/luci/common/cli"
)

// TODO(vadimsh): If the config set is not provided, try to guess it from the
// git repo and location of files within the repo (by comparing them to output
// of GetConfigSets() listing). Present the guess to the end user, so they can
// confirm it a put it into the flag/config.

func cmdValidate(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "validate [CONFIG_DIR]",
		ShortDesc: "sends files under CONFIG_DIR (or CWD if not set) to LUCI Config service for validation",
		CommandRun: func() subcommands.CommandRun {
			c := &validateRun{}
			c.init(params, true)
			c.Flags.StringVar(&c.configSet, "config-set", "<name>", "Name of the config set to validate against.")
			return c
		},
	}
}

type validateRun struct {
	subcommand
	configSet string
	configDir string
}

type individualResult struct {
	Path          string `json:"path"`
	ErrorSeverity string `json:"error_severity"`
	ErrorMessage  string `json:"error_text"`
}

type validateResult struct {
	IndividualResults []individualResult `json:"individual_results"`
}

func (c *validateRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 1) {
		return 1
	}
	if len(args) == 1 {
		c.configDir = args[0]
	} else {
		configDir, err := os.Getwd()
		if err != nil {
			c.printError(err)
			return 1
		}
		c.configDir = configDir
	}
	ctx := cli.GetContext(a, c, env)
	// Construct the service outside run to improve testability.
	svc, err := c.configService(ctx)
	if err != nil {
		return c.done(nil, err)
	}
	return c.done(c.run(ctx, svc))
}

// responseWithPath bundles a response message and / or error with the file path that
// was used in the validation request.
type responseWithPath struct {
	ResponseMessage *config.LuciConfigValidateConfigResponseMessage
	Err             error
	Path            *string
}

// pushResponses calls svc.ValidateConfig() for each element of configFiles, pushes the responses
// into respChan and then closes respChan.
func (c *validateRun) pushResponses(ctx context.Context, svc *config.Service, configFiles []*config.LuciConfigValidateConfigRequestMessageFile, respChan chan responseWithPath) {
	// We want to associate each error with the config file it refers to.
	// The server's response doesn't allow any way to do this when validating
	// multiple files in one call, so we do a separate ValidateConfig() call per file.
	var wg sync.WaitGroup
	for i := range configFiles {
		wg.Add(1)
		// Force i to be taken by value instead of reference, otherwise this is racy.
		go func(i int) {
			defer wg.Done()
			validateRequest := config.LuciConfigValidateConfigRequestMessage{}
			validateRequest.ConfigSet = c.configSet
			validateRequest.Files = []*config.LuciConfigValidateConfigRequestMessageFile{configFiles[i]}
			resp, err := svc.ValidateConfig(&validateRequest).Context(ctx).Do()
			respChan <- responseWithPath{resp, err, &configFiles[i].Path}
		}(i)
	}
	go func() {
		wg.Wait()
		close(respChan)
	}()
}

// processResponses iterates over respChan until its closed and aggregates those
// responses into the validateResult. Returns an error if any files were invalid or failed
// to be checked.
func processResponses(respChan chan responseWithPath) (*validateResult, error) {
	allSuccess := true
	var res validateResult
	for resp := range respChan {
		if resp.Err != nil {
			log.Printf("Error validating %s: %v\n", *resp.Path, resp.Err)
			allSuccess = false
			res.IndividualResults = append(res.IndividualResults,
				individualResult{*resp.Path, "TOOL", fmt.Sprintf("Call to ValidateConfig() failed with: %v", resp.Err)})
			continue
		}
		messagesLen := len(resp.ResponseMessage.Messages)
		if messagesLen == 1 {
			allSuccess = false
			message := resp.ResponseMessage.Messages[0]
			log.Printf("%s: %s: %s\n", *resp.Path, message.Severity, message.Text)
			res.IndividualResults = append(res.IndividualResults,
				individualResult{*resp.Path, message.Severity, message.Text})
		} else if messagesLen > 1 {
			allSuccess = false
			errMsg := fmt.Sprintf("expected only 1 message in the response, got %d", messagesLen)
			log.Printf("Error validating %s: %s\n", *resp.Path, errMsg)
			res.IndividualResults = append(res.IndividualResults,
				individualResult{*resp.Path, "TOOL", fmt.Sprintf(errMsg)})
		}
	}
	var err error = nil
	if !allSuccess {
		err = fmt.Errorf("Some files were invalid or could not be checked")
	}
	return &res, err
}

// run recursively searches c.configDir for config files, calls svc.ValidateConfig() on them
// and aggregates the results.
func (c *validateRun) run(ctx context.Context, svc *config.Service) (*validateResult, error) {
	var configFiles []*config.LuciConfigValidateConfigRequestMessageFile
	err := filepath.Walk(c.configDir,
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
			relPath, err := filepath.Rel(c.configDir, p)
			if err != nil {
				return err
			}
			configFile := config.LuciConfigValidateConfigRequestMessageFile{
				Content: base64.StdEncoding.EncodeToString(content),
				Path:    relPath,
			}
			configFiles = append(configFiles, &configFile)
			return nil
		})
	if err != nil {
		return nil, err
	}

	respChan := make(chan responseWithPath)
	go c.pushResponses(ctx, svc, configFiles, respChan)
	return processResponses(respChan)
}
