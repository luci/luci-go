// Copyright 2022 The LUCI Authors.
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/cipd/client/cipd/platform"
	cipdVersion "go.chromium.org/luci/cipd/version"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

const (
	ensureFileHeader = "$ServiceURL https://chrome-infra-packages.appspot.com/\n$ParanoidMode CheckPresence\n"
	kitchenCheckout  = "kitchen-checkout"
)

// resultsFilePath is the path to the generated file from cipd ensure command.
// Placing it here to allow to replace it during testing.
var resultsFilePath = filepath.Join(os.TempDir(), "cipd_ensure_results.json")

// execCommandContext to allow to replace it during testing.
var execCommandContext = exec.CommandContext

type cipdPkg struct {
	Package    string `json:"package"`
	InstanceID string `json:"instance_id"`
}

// cipdOut corresponds to the structure of the generated result json file from cipd ensure command.
type cipdOut struct {
	Result map[string][]*cipdPkg `json:"result"`
}

func prependPath(bld *bbpb.Build, workDir string) error {
	extraPathEnv := stringset.Set{}
	for _, ref := range bld.Infra.Buildbucket.Agent.Input.Data {
		extraPathEnv.AddAll(ref.OnPath)
	}

	var extraAbsPaths []string
	for _, p := range extraPathEnv.ToSortedSlice() {
		extraAbsPaths = append(extraAbsPaths, filepath.Join(workDir, p))
	}
	original := os.Getenv("PATH")
	if err := os.Setenv("PATH", strings.Join(append(extraAbsPaths, original), string(os.PathListSeparator))); err != nil {
		return err
	}
	return nil
}

// downloadCipdPackages wraps installCipdPackages with logic to update the build
func downloadCipdPackages(ctx context.Context, cwd string, c clientInput) int {
	// Most likely happens in `led get-build` process where it creates from an old build
	// before new Agent field was there. This new feature shouldn't work for those builds.
	if c.input.Build.Infra.Buildbucket.Agent == nil {
		checkReport(ctx, c, errors.New("Cannot enable downloading cipd pkgs feature; Build Agent field is not set"))
	}

	agent := c.input.Build.Infra.Buildbucket.Agent
	agent.Output = &bbpb.BuildInfra_Buildbucket_Agent_Output{
		Status: bbpb.Status_STARTED,
	}
	updateReq := &bbpb.UpdateBuildRequest{
		Build: &bbpb.Build{
			Id: c.input.Build.Id,
			Infra: &bbpb.BuildInfra{
				Buildbucket: &bbpb.BuildInfra_Buildbucket{
					Agent: agent,
				},
			},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"build.infra.buildbucket.agent.output"}},
		Mask:       readMask,
	}
	bldStartCipd, err := c.bbclient.UpdateBuild(ctx, updateReq)
	if err != nil {
		// Carry on and bear the non-fatal update failure.
		logging.Warningf(ctx, "Failed to report build agent STARTED status: %s", err)
	}
	// The build has been canceled, bail out early.
	if bldStartCipd.CancelTime != nil {
		return cancelBuild(ctx, c.bbclient, bldStartCipd)
	}

	agent.Output.AgentPlatform = platform.CurrentPlatform()

	// Encapsulate all the installation logic with a defer to set the
	// TotalDuration. As we add more installation logic (e.g. RBE-CAS),
	// TotalDuration should continue to surround that logic.
	err = func() error {
		start := clock.Now(ctx)
		defer func() {
			agent.Output.TotalDuration = &durationpb.Duration{
				Seconds: int64(clock.Since(ctx, start).Round(time.Second).Seconds()),
			}
		}()
		if err := prependPath(c.input.Build, cwd); err != nil {
			return err
		}
		return installCipdPackages(ctx, c.input.Build, cwd)
	}()

	if err != nil {
		logging.Errorf(ctx, "Failure in installing cipd packages: %s", err)
		agent.Output.Status = bbpb.Status_FAILURE
		agent.Output.SummaryHtml = err.Error()
		updateReq.Build.Status = bbpb.Status_INFRA_FAILURE
		updateReq.Build.SummaryMarkdown = "Failed to install cipd packages for this build"
		updateReq.UpdateMask.Paths = append(updateReq.UpdateMask.Paths, "build.status", "build.summary_markdown")
	} else {
		agent.Output.Status = bbpb.Status_SUCCESS
		if c.input.Build.Exe != nil {
			updateReq.UpdateMask.Paths = append(updateReq.UpdateMask.Paths, "build.infra.buildbucket.agent.purposes")
		}
	}

	bldCompleteCipd, bbErr := c.bbclient.UpdateBuild(ctx, updateReq)
	if bbErr != nil {
		logging.Warningf(ctx, "Failed to report build agent output status: %s", bbErr)
	}
	if err != nil {
		os.Exit(-1)
	}
	// The build has been canceled, bail out early.
	if bldCompleteCipd.CancelTime != nil {
		return cancelBuild(ctx, c.bbclient, bldCompleteCipd)
	}
	return 0
}

// installCipdPackages installs cipd packages defined in build.Infra.Buildbucket.Agent.Input
// and build exe.
//
// This will update the following fields in build:
//   - Infra.Buidlbucket.Agent.Output.ResolvedData
//   - Infra.Buildbucket.Agent.Purposes
//
// Note:
//  1. It assumes `cipd` client tool binary is already in path.
//  2. Hack: it includes bbagent version in the ensure file if it's called from
//     a cipd installed bbagent.
func installCipdPackages(ctx context.Context, build *bbpb.Build, workDir string) error {
	logging.Infof(ctx, "Installing cipd packages into %s", workDir)
	inputData := build.Infra.Buildbucket.Agent.Input.Data

	ensureFileBuilder := strings.Builder{}
	ensureFileBuilder.WriteString(ensureFileHeader)

	// TODO(crbug.com/1297809): Remove it once we decide to have a subfolder for
	// non-bbagent packages in the post-migration stage.
	switch ver, err := cipdVersion.GetStartupVersion(); {
	case err != nil:
		// If the binary is not installed via CIPD, err == nil && ver.InstanceID == "".
		return errors.Annotate(err, "Failed to get the current executable startup version").Err()
	default:
		fmt.Fprintf(&ensureFileBuilder, "%s %s\n", ver.PackageName, ver.InstanceID)
	}

	for dir, pkgs := range inputData {
		if pkgs.GetCipd() == nil {
			continue
		}
		fmt.Fprintf(&ensureFileBuilder, "@Subdir %s\n", dir)
		for _, spec := range pkgs.GetCipd().Specs {
			fmt.Fprintf(&ensureFileBuilder, "%s %s\n", spec.Package, spec.Version)
		}
	}
	if build.Exe != nil {
		fmt.Fprintf(&ensureFileBuilder, "@Subdir %s\n", kitchenCheckout)
		fmt.Fprintf(&ensureFileBuilder, "%s %s\n", build.Exe.CipdPackage, build.Exe.CipdVersion)
		if build.Infra.Buildbucket.Agent.Purposes == nil {
			build.Infra.Buildbucket.Agent.Purposes = make(map[string]bbpb.BuildInfra_Buildbucket_Agent_Purpose, 1)
		}
		build.Infra.Buildbucket.Agent.Purposes[kitchenCheckout] = bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD
	}

	// TODO(crbug.com/1297809): Remove this redundant log once this feature development is done.
	logging.Infof(ctx, "===ensure file===\n%s\n=========", ensureFileBuilder.String())

	// Install packages
	cmd := execCommandContext(ctx, "cipd", "ensure", "-root", workDir, "-ensure-file", "-", "-json-output", resultsFilePath)
	logging.Infof(ctx, "Running command: %s", cmd.String())
	cmd.Stdin = strings.NewReader(ensureFileBuilder.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.Annotate(err, "Failed to run cipd ensure command").Err()
	}

	resultsFile, err := os.Open(resultsFilePath)
	if err != nil {
		return err
	}
	defer resultsFile.Close()
	cipdOutputs := cipdOut{}
	jsonResults, err := ioutil.ReadAll(resultsFile)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(jsonResults, &cipdOutputs); err != nil {
		return err
	}

	resolved := make(map[string]*bbpb.ResolvedDataRef, len(inputData)+1)
	build.Infra.Buildbucket.Agent.Output.ResolvedData = resolved
	for p, pkgs := range cipdOutputs.Result {
		resolvedPkgs := make([]*bbpb.ResolvedDataRef_CIPD_PkgSpec, 0, len(pkgs))
		for _, pkg := range pkgs {
			resolvedPkgs = append(resolvedPkgs, &bbpb.ResolvedDataRef_CIPD_PkgSpec{
				Package: pkg.Package,
				Version: pkg.InstanceID,
			})
		}
		resolved[p] = &bbpb.ResolvedDataRef{
			DataType: &bbpb.ResolvedDataRef_Cipd{Cipd: &bbpb.ResolvedDataRef_CIPD{
				Specs: resolvedPkgs,
			}},
		}
	}

	return nil
}
