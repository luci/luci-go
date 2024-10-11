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

	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/swarming/client/swarming"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestReproduceParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdReproduce,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		assert.Loosely(t, code, should.Equal(1))
		assert.Loosely(t, stderr, should.ContainSubstring(errLike))
	}

	ftt.Run(`Requires task ID.`, t, func(t *ftt.Test) {
		expectErr(nil, "expecting exactly 1 argument")
	})

	ftt.Run(`Accepts only one task ID.`, t, func(t *ftt.Test) {
		expectErr([]string{"aaaa", "bbbb"}, "expecting exactly 1 argument")
	})
}

func TestPrepareTaskRequestEnvironment(t *testing.T) {
	t.Parallel()

	ftt.Run(`Make sure we can create the correct Cmd from a fetched task's properties.`, t, func(t *ftt.Test) {
		c := reproduceImpl{}

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

		// So that this test works on swarming!
		ctxBaseEnvMap.Remove(swarming.ServerEnvVar)
		ctxBaseEnvMap.Remove(swarming.TaskIDEnvVar)

		// Set env values to be removed or replaced in prepareTaskRequestEnvironment().
		ctxBaseEnvMap.Set("removeKey", "removeValue")
		ctxBaseEnvMap.Set("replaceKey", "oldValue")
		ctxBaseEnvMap.Set("noChangeKey", "noChangeValue")
		ctxBaseEnvMap.Set("PATH", "existingPathValue")
		ctx = ctxBaseEnvMap.SetInCtx(ctx)

		expectedEnvMap := ctxBaseEnvMap.Clone()

		var fetchedCASFiles bool

		properties := &swarmingv2.TaskProperties{
			Command:     []string{"rbd", "stream", "-test-id-prefix", "--isolated-output=${ISOLATED_OUTDIR}/chicken-output.json"},
			RelativeCwd: relativeCwd,
			Env: []*swarmingv2.StringPair{
				{
					Key:   "key",
					Value: "value1",
				},
				{
					Key:   "replaceKey",
					Value: "value2",
				},
				{
					Key:   "removeKey",
					Value: "",
				},
			},
			EnvPrefixes: []*swarmingv2.StringListPair{
				{
					Key:   "PATH",
					Value: []string{".task_template_packages", ".task_template_packages/zoo"},
				},
				{
					Key:   "CHICKENS",
					Value: []string{"egg", "rooster"},
				},
			},
			CasInputRoot: &swarmingv2.CASReference{
				CasInstance: "CAS-instance",
			},
			CipdInput: &swarmingv2.CipdInput{
				Packages: []*swarmingv2.CipdPackage{
					{
						PackageName: "infra/tools/luci-auth/${platform}",
						Path:        ".task_template_packages",
						Version:     "git_revision:41a7e9bcbf18718dcda83dd5c6188cfc44271e70",
					},
					{
						PackageName: "infra/tools/luci/logdog/butler/${platform}",
						Path:        ".",
						Version:     "git_revision:e1abc57be62d198b5c2f487bfb2fa2d2eb0e867c",
					},
					{
						PackageName: "infra/tools/luci/logchicken/${platform}",
						Path:        "",
						Version:     "git_revision:e1abc57be62d198b5c2f487bfb2fa2d2eb0e867d",
					},
				},
			},
		}

		service := &swarmingtest.Client{
			FilesFromCASMock: func(_ context.Context, _ string, _ *swarmingv2.CASReference) ([]string, error) {
				fetchedCASFiles = true
				return []string{}, nil
			},
		}

		cmd, err := c.prepareTaskRequestEnvironment(ctx, properties, service, &testAuthFlags{})
		assert.Loosely(t, err, should.BeNil)

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

		assert.Loosely(t, cmd.Path, should.Equal(expected.Path))
		assert.Loosely(t, cmd.Args, should.Resemble(expected.Args))
		assert.Loosely(t, cmd.Env, should.Resemble(expected.Env))
		assert.Loosely(t, cmd.Dir, should.Equal(expected.Dir))

		assert.Loosely(t, fetchedCASFiles, should.BeTrue)

		assert.Loosely(t, cipdSlicesByPath, should.Resemble(map[string]ensure.PackageSlice{
			"": {
				ensure.PackageDef{
					PackageTemplate:   "infra/tools/luci/logdog/butler/${platform}",
					UnresolvedVersion: "git_revision:e1abc57be62d198b5c2f487bfb2fa2d2eb0e867c",
				},
				ensure.PackageDef{
					PackageTemplate:   "infra/tools/luci/logchicken/${platform}",
					UnresolvedVersion: "git_revision:e1abc57be62d198b5c2f487bfb2fa2d2eb0e867d",
				},
			},
			".task_template_packages": {
				ensure.PackageDef{
					PackageTemplate:   "infra/tools/luci-auth/${platform}",
					UnresolvedVersion: "git_revision:41a7e9bcbf18718dcda83dd5c6188cfc44271e70",
				}},
		}))
	})
}

func TestReproduceTaskRequestCommand(t *testing.T) {
	t.Parallel()

	ftt.Run(`Make sure we can execute commands.`, t, func(t *ftt.Test) {
		c := reproduceImpl{}

		ctx := context.Background()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "cmd", "/c", "echo", "chicken")
		} else {
			cmd = exec.CommandContext(ctx, "echo", "chicken")
		}

		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		err := c.executeTaskRequestCommand(ctx, &swarmingv2.TaskRequestResponse{}, cmd, &testAuthFlags{})
		assert.Loosely(t, err, should.BeNil)
		if runtime.GOOS == "windows" {
			assert.Loosely(t, stdout.String(), should.Equal("chicken\r\n"))
		} else {
			assert.Loosely(t, stdout.String(), should.Equal("chicken\n"))
		}
	})
}
