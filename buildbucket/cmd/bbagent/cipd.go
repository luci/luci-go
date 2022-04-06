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

	bbpb "go.chromium.org/luci/buildbucket/proto"
	cipdVersion "go.chromium.org/luci/cipd/version"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
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

// installCipdPackages installs cipd packages defined in build.Infra.Buildbucket.Agent.Input
// and build exe. It also prepends desired value to $PATH env var.
//
// This will update the following fields in build:
//   * Infra.Buidlbucket.Agent.Output.ResolvedData
//   * Infra.Buildbucket.Agent.Purposes
//
// Note:
//   1. It assumes `cipd` client tool binary is already in path.
//   2. Hack: it includes bbagent version in the ensure file if it's called from
//   a cipd installed bbagent.
func installCipdPackages(ctx context.Context, build *bbpb.Build, workDir string) error {
	logging.Infof(ctx, "Installing cipd packages into %s", workDir)
	inputData := build.Infra.Buildbucket.Agent.Input.Data

	ensureFileBuilder := strings.Builder{}
	ensureFileBuilder.WriteString(ensureFileHeader)
	extraPathEnv := stringset.Set{}

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
		extraPathEnv.AddAll(pkgs.OnPath)
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

	// Prepend to $PATH
	var extraAbsPaths []string
	for _, p := range extraPathEnv.ToSortedSlice() {
		extraAbsPaths = append(extraAbsPaths, filepath.Join(workDir, p))
	}
	original := os.Getenv("PATH")
	if err := os.Setenv("PATH", strings.Join(append(extraAbsPaths, original), string(os.PathListSeparator))); err != nil {
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
