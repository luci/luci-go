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

package host

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
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

func setPathEnv(workDir string, extraRelPaths []string) error {
	if len(extraRelPaths) == 0 {
		return nil
	}
	var extraAbsPaths []string
	for _, p := range extraRelPaths {
		extraAbsPaths = append(extraAbsPaths, filepath.Join(workDir, p))
	}
	original := os.Getenv("PATH")
	return os.Setenv("PATH", strings.Join(append(extraAbsPaths, original), string(os.PathListSeparator)))
}

func prependPath(bld *bbpb.Build, workDir string) error {
	extraPathEnv := stringset.Set{}
	for _, ref := range bld.Infra.Buildbucket.Agent.Input.Data {
		extraPathEnv.AddAll(ref.OnPath)
	}
	return setPathEnv(workDir, extraPathEnv.ToSortedSlice())
}

// getCipdClientWithRetry attempts to download the cipd client using http.Get with a retry strategy.
func getCipdClientWithRetry(ctx context.Context, cipdURL string) (resp *http.Response, err error) {
	doGetCipd := func() error {
		resp, err = http.Get(cipdURL)
		if err != nil {
			return transient.Tag.Apply(err)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		return transient.Tag.Apply(errors.New(fmt.Sprintf("HTTP request failed with status code: %d", resp.StatusCode)))
	}
	// Configure the retry
	err = retry.Retry(ctx, transient.Only(func() retry.Iterator {
		// Configure the retry strategy
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   500 * time.Millisecond, // initial delay time
				Retries: 5,                      // number of retries
			},
			Multiplier: 2,               // backoff multiplier
			MaxDelay:   5 * time.Second, // maximum delay time
		}
	}), doGetCipd, nil)
	return
}

// installCipd installs the cipd client provided to us for the build. It will return
// the path on disk of the binary so that installCipdPackages can use the downloaded cipd client.
func installCipd(ctx context.Context, build *bbpb.Build, workDir, cacheBase, platform string) error {
	var cipdFile, cipdDir, cipdServer, cipdVersion string
	var onPath []string
	// We "loop" through this because it is a map, however, there should only be one entry in this map.
	for relativePath, cipdSource := range build.Infra.Buildbucket.Agent.Input.CipdSource {
		// The binary itself will be located at "workdir/cipd/cipd"
		// where "workdir/cipd" is the directory.
		cipdDir = filepath.Join(workDir, relativePath)
		cipdFile = filepath.Join(cipdDir, "cipd")
		cipdServer = cipdSource.GetCipd().Server
		cipdVersion = cipdSource.GetCipd().Specs[0].Version
		onPath = cipdSource.OnPath
		break
	}
	if cipdVersion == "" {
		return nil
	}

	cipdClientCache := build.Infra.Buildbucket.Agent.CipdClientCache

	needtoDownload := true
	if cipdClientCache != nil {
		// Use cipd client cache.
		cipdCacheDirRel := filepath.Join(cacheBase, cipdClientCache.Path) // cache/cipd_client
		cipdCacheDir := filepath.Join(workDir, cipdCacheDirRel)           // b/s/w/ir/cache/cipd_client
		cipdCachePath := filepath.Join(cipdCacheDir, "cipd")              // b/s/w/ir/cache/cipd_client/cipd
		cipdCacheOnPath := []string{cipdCacheDirRel, filepath.Join(cipdCacheDirRel, "bin")}

		_, err := os.Stat(cipdCachePath)
		switch {
		case err == nil:
			// cache hit, use the client directly.
			needtoDownload = false
			onPath = cipdCacheOnPath
		case os.IsNotExist(err):
			// cache miss, download and install cipd client in cipdCacheDir.
			cipdDir = cipdCacheDir
			cipdFile = cipdCachePath
			onPath = cipdCacheOnPath
		default:
			logging.Infof(ctx, "failed to get cipd client from cache: %s.", err)
			// Forget about cache, download and install cipd client directly.
		}
	}

	if needtoDownload {
		if err := downloadCipd(ctx, cipdServer, platform, cipdVersion, cipdDir, cipdFile); err != nil {
			return err
		}
	}

	// Append cipd path to $PATH.
	return setPathEnv(workDir, onPath)
}

func downloadCipd(ctx context.Context, cipdServer, platform, cipdVersion, cipdDir, cipdFile string) error {
	// Pull cipd binary
	cipdURL := fmt.Sprintf("https://%s/client?platform=%s&version=%s", cipdServer, platform, cipdVersion)
	logging.Infof(ctx, "Install CIPD client from URL: %s into %s", cipdURL, cipdDir)
	resp, err := getCipdClientWithRetry(ctx, cipdURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Make the directory to save cipd client into if it does not exist.
	err = os.MkdirAll(cipdDir, os.ModePerm)
	if err != nil {
		return err
	}
	// Create file to store cipd binary in
	out, err := os.Create(cipdFile)
	if err != nil {
		return err
	}
	defer out.Close()
	// Write binary to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}
	// Give the binary executable permission.
	err = os.Chmod(cipdFile, 0700)
	if err != nil {
		return err
	}
	return nil
}

// installCipdPackages installs cipd packages defined in build.Infra.Buildbucket.Agent.Input
// and build exe.
//
// This will update the following fields in build:
//   - Infra.Buidlbucket.Agent.Output.ResolvedData
//   - Infra.Buildbucket.Agent.Purposes
//
// Note:
//  1. It will use the `cipd` client tool binary path that is in path.
//  2. Hack: it includes bbagent version in the ensure file if it's called from
//     a cipd installed bbagent.
func installCipdPackages(ctx context.Context, build *bbpb.Build, workDir, cacheBase string) error {
	logging.Infof(ctx, "Installing cipd packages into %s", workDir)
	inputData := build.Infra.Buildbucket.Agent.Input.Data

	ensureFileBuilder := strings.Builder{}
	ensureFileBuilder.WriteString(ensureFileHeader)

	payloadPath := kitchenCheckout
	for dir, purpose := range build.Infra.Buildbucket.Agent.GetPurposes() {
		if purpose == bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD {
			payloadPath = dir
		}
	}
	payloadPathInAgentInput := false
	for dir, pkgs := range inputData {
		if pkgs.GetCipd() == nil {
			continue
		}
		if dir == payloadPath {
			payloadPathInAgentInput = true
		}
		fmt.Fprintf(&ensureFileBuilder, "@Subdir %s\n", dir)
		for _, spec := range pkgs.GetCipd().Specs {
			fmt.Fprintf(&ensureFileBuilder, "%s %s\n", spec.Package, spec.Version)
		}
	}
	if !payloadPathInAgentInput && build.Exe != nil {
		fmt.Fprintf(&ensureFileBuilder, "@Subdir %s\n", payloadPath)
		fmt.Fprintf(&ensureFileBuilder, "%s %s\n", build.Exe.CipdPackage, build.Exe.CipdVersion)
		if build.Infra.Buildbucket.Agent.Purposes == nil {
			build.Infra.Buildbucket.Agent.Purposes = make(map[string]bbpb.BuildInfra_Buildbucket_Agent_Purpose, 1)
		}
		build.Infra.Buildbucket.Agent.Purposes[payloadPath] = bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD
	}

	// TODO(crbug.com/1297809): Remove this redundant log once this feature development is done.
	logging.Infof(ctx, "===ensure file===\n%s\n=========", ensureFileBuilder.String())

	// Find cipd packages cache and set $CIPD_CACHE_DIR.
	cache := build.Infra.Buildbucket.Agent.CipdPackagesCache
	if cache != nil {
		cacheDir := filepath.Join(cacheBase, cache.Path)
		logging.Infof(ctx, "Setting $CIPD_CACHE_DIR to %q", cacheDir)
		if err := os.Setenv("CIPD_CACHE_DIR", cacheDir); err != nil {
			return err
		}
	}

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
	jsonResults, err := io.ReadAll(resultsFile)
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
