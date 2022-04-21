// Copyright 2021 The LUCI Authors.
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

package swarmingimpl

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	rbeclient "github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/system/environ"
	. "go.chromium.org/luci/common/testing/assertions"
)

func init() {
	// So that this test works on swarming!
	os.Unsetenv(ServerEnvVar)
	os.Unsetenv(TaskIDEnvVar)
}

func TestReproduceParse_NoArgs(t *testing.T) {
	t.Parallel()
	Convey(`Make sure Parse works with no arguments.`, t, func() {
		c := reproduceRun{}
		c.init(&testAuthFlags{})

		err := c.parse([]string(nil))
		So(err, ShouldErrLike, "must provide -server")
	})
}

func TestReproduceParse_NoTaskID(t *testing.T) {
	t.Parallel()
	Convey(`Make sure Parse works with with no task ID.`, t, func() {
		c := reproduceRun{}
		c.init(&testAuthFlags{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})
		So(err, ShouldBeNil)

		err = c.parse([]string(nil))
		So(err, ShouldErrLike, "must specify exactly one task id.")
	})
}

func TestPrepareTaskRequestEnvironment(t *testing.T) {
	t.Parallel()
	Convey(`Make sure we can create the correct Cmd from a fetched task's properties.`, t, func() {
		c := reproduceRun{}
		c.init(&testAuthFlags{})
		var cipdSlicesByPath map[string]ensure.PackageSlice
		c.cipdDownloader = func(ctx context.Context, workdir string, slicesByPath map[string]ensure.PackageSlice) error {
			cipdSlicesByPath = slicesByPath
			return nil
		}
		// Use TempDir, which creates a temp directory, to return a unique directory name
		// that prepareTaskRequestEnvironment() will remove and recreate (via prepareDir()).
		c.work = t.TempDir()
		relativeCwd := "farm"

		c.out = t.TempDir()

		ctx := context.Background()

		ctxBaseEnvMap := environ.System()

		// Set env values to be removed or replaced in prepareTaskRequestEnvironment().
		ctxBaseEnvMap.Set("removeKey", "removeValue")
		ctxBaseEnvMap.Set("replaceKey", "oldValue")
		ctxBaseEnvMap.Set("noChangeKey", "noChangeValue")
		ctxBaseEnvMap.Set("PATH", "existingPathValue")
		ctx = ctxBaseEnvMap.SetInCtx(ctx)

		expectedEnvMap := ctxBaseEnvMap.Clone()

		var fetchedCASFiles bool

		properties := &swarming.SwarmingRpcsTaskProperties{
			Command:     []string{"rbd", "stream", "-test-id-prefix", "--isolated-output=${ISOLATED_OUTDIR}/chicken-output.json"},
			RelativeCwd: relativeCwd,
			Env: []*swarming.SwarmingRpcsStringPair{
				&swarming.SwarmingRpcsStringPair{
					Key:   "key",
					Value: "value1",
				},
				&swarming.SwarmingRpcsStringPair{
					Key:   "replaceKey",
					Value: "value2",
				},
				&swarming.SwarmingRpcsStringPair{
					Key:   "removeKey",
					Value: "",
				},
			},
			EnvPrefixes: []*swarming.SwarmingRpcsStringListPair{
				&swarming.SwarmingRpcsStringListPair{
					Key:   "PATH",
					Value: []string{".task_template_packages", ".task_template_packages/zoo"},
				},
				&swarming.SwarmingRpcsStringListPair{
					Key:   "CHICKENS",
					Value: []string{"egg", "rooster"},
				},
			},
			CasInputRoot: &swarming.SwarmingRpcsCASReference{
				CasInstance: "CAS-instance",
			},
			CipdInput: &swarming.SwarmingRpcsCipdInput{
				Packages: []*swarming.SwarmingRpcsCipdPackage{
					&swarming.SwarmingRpcsCipdPackage{
						PackageName: "infra/tools/luci-auth/${platform}",
						Path:        ".task_template_packages",
						Version:     "git_revision:41a7e9bcbf18718dcda83dd5c6188cfc44271e70",
					},
					&swarming.SwarmingRpcsCipdPackage{
						PackageName: "infra/tools/luci/logdog/butler/${platform}",
						Path:        ".",
						Version:     "git_revision:e1abc57be62d198b5c2f487bfb2fa2d2eb0e867c",
					},
					&swarming.SwarmingRpcsCipdPackage{
						PackageName: "infra/tools/luci/logchicken/${platform}",
						Path:        "",
						Version:     "git_revision:e1abc57be62d198b5c2f487bfb2fa2d2eb0e867d",
					},
				},
			},
		}

		service := &testService{
			filesFromCAS: func(_ context.Context, _ string, _ *rbeclient.Client, _ *swarming.SwarmingRpcsCASReference) ([]string, error) {
				fetchedCASFiles = true
				return []string{}, nil
			},
		}

		cmd, err := c.prepareTaskRequestEnvironment(ctx, properties, service)
		So(err, ShouldBeNil)

		expected := exec.CommandContext(ctx, "rbd", "stream", "-test-id-prefix",
			fmt.Sprintf("--isolated-output=%s", filepath.Join(c.out, "chicken-output.json")))
		expected.Dir = filepath.Join(c.work, relativeCwd)
		expected.Stdout = os.Stdout
		expected.Stderr = os.Stderr

		expectedEnvMap.Remove("removeKey")
		expectedEnvMap.Set("key", "value1")
		expectedEnvMap.Set("replaceKey", "value2")

		paths := []string{
			filepath.Join(c.work, ".task_template_packages"),
			filepath.Join(c.work, ".task_template_packages/zoo"),
			"existingPathValue"}
		expectedEnvMap.Set("PATH", strings.Join(paths, string(os.PathListSeparator)))
		chickens := []string{filepath.Join(c.work, "egg"), filepath.Join(c.work, "rooster")}
		expectedEnvMap.Set("CHICKENS", strings.Join(chickens, string(os.PathListSeparator)))

		expected.Env = expectedEnvMap.Sorted()
		So(cmd, ShouldResemble, expected)

		So(fetchedCASFiles, ShouldBeTrue)

		So(cipdSlicesByPath, ShouldResemble, map[string]ensure.PackageSlice{
			"": ensure.PackageSlice{
				ensure.PackageDef{
					PackageTemplate:   "infra/tools/luci/logdog/butler/${platform}",
					UnresolvedVersion: "git_revision:e1abc57be62d198b5c2f487bfb2fa2d2eb0e867c",
				},
				ensure.PackageDef{
					PackageTemplate:   "infra/tools/luci/logchicken/${platform}",
					UnresolvedVersion: "git_revision:e1abc57be62d198b5c2f487bfb2fa2d2eb0e867d",
				},
			},
			".task_template_packages": ensure.PackageSlice{
				ensure.PackageDef{
					PackageTemplate:   "infra/tools/luci-auth/${platform}",
					UnresolvedVersion: "git_revision:41a7e9bcbf18718dcda83dd5c6188cfc44271e70",
				}},
		})
	})
}

func TestReproduceTaskRequestCommand(t *testing.T) {
	t.Parallel()
	Convey(`Make sure we can execute commands.`, t, func() {
		c := reproduceRun{}
		c.init(&testAuthFlags{})
		ctx := context.Background()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "cmd", "/c", "echo", "chicken")
		} else {
			cmd = exec.CommandContext(ctx, "echo", "chicken")
		}

		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		err := c.executeTaskRequestCommand(ctx, &swarming.SwarmingRpcsTaskRequest{}, cmd)
		So(err, ShouldBeNil)
		if runtime.GOOS == "windows" {
			So(stdout.String(), ShouldEqual, "chicken\r\n")
		} else {
			So(stdout.String(), ShouldEqual, "chicken\n")
		}

	})
}
