// Copyright 2019 The LUCI Authors.
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

package deployer

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
)

func TestMultipleDeployProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("Skipping, flaky on luci-go-continuous-win7-64: crbug.com/1077343")
	}

	const procs = 30

	tempDir, err := ioutil.TempDir("", "cipd_test")
	if err != nil {
		t.Fatalf("TempDir: %s", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	wg := sync.WaitGroup{}
	wg.Add(procs)
	for p := 0; p < procs; p++ {
		go func() {
			defer wg.Done()
			// Run a crashing process first, to emulate an unowned lock.
			_, err := runDeployerProc(p, tempDir, true)
			if err == nil {
				t.Errorf("Subprocess %d unexpectedly succeeded", p)
				return
			}
			// Now run a non-crashing deployer, it should succeed.
			out, err := runDeployerProc(p, tempDir, false)
			if err != nil {
				t.Errorf("Subprocess %d failed (%s):\n%s", p, err, out)
				return
			}
		}()
	}
	wg.Wait()
}

func runDeployerProc(idx int, tempDir string, crash bool) (out []byte, err error) {
	// See https://npf.io/2015/06/testing-exec-command/
	cmd := exec.Command(os.Args[0], "-test.run=TestDeployHelperProcess")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("CIPD_TEST_PROC_IDX=%d", idx),
		fmt.Sprintf("CIPD_TEST_TEMP_DIR=%s", tempDir),
		fmt.Sprintf("CIPD_TEST_CRASH=%v", crash),
	)
	return cmd.CombinedOutput()
}

func TestDeployHelperProcess(t *testing.T) {
	idxs := os.Getenv("CIPD_TEST_PROC_IDX")
	if idxs == "" {
		t.Skip("Skipping the helper test")
	}
	idx, err := strconv.ParseInt(idxs, 10, 64)
	if err != nil {
		t.Fatalf("Bad CIPD_TEST_PROC_IDX %q", idxs)
	}
	tempDir := os.Getenv("CIPD_TEST_TEMP_DIR")
	if tempDir == "" {
		t.Fatalf("Bad empty CIPD_TEST_TEMP_DIR")
	}
	crash := os.Getenv("CIPD_TEST_CRASH") == "true"

	// Do a hard crash right after acquiring the FS lock.
	if crash {
		origLockFS := lockFS
		lockFS = func(path string, waiter func() error) (func() error, error) {
			origLockFS(path, waiter)
			os.Exit(1)
			panic("unreachable")
		}
	}

	installMode := pkg.InstallModeSymlink
	if runtime.GOOS == "windows" {
		installMode = pkg.InstallModeCopy
	}

	// Verbose logging to stderr.
	ctx := gologger.StdConfig.Use(context.Background())
	ctx = logging.SetLevel(ctx, logging.Debug)

	// Do multiple rounds to increase the chance of collision.
	for r := 0; r < 20; r++ {
		instID := (int(idx) + r) % 5

		logging.Infof(ctx, "------------------------------------------------------")
		logging.Infof(ctx, "Round %d", r)
		logging.Infof(ctx, "------------------------------------------------------")

		inst := makeTestInstance(fmt.Sprintf("pkg/%d", instID), []fs.File{
			fs.NewTestFile(fmt.Sprintf("private_%d", instID), "data", fs.TestFileOpts{}),
			fs.NewTestFile("path/shared", "data", fs.TestFileOpts{}),
		}, installMode)
		inst.instanceID = fmt.Sprintf("-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwF%02d", instID)

		d := New(tempDir)
		maxThreads := 16
		_, err = d.DeployInstance(ctx, "", inst, "", maxThreads)
		if err != nil {
			t.Fatalf("DeployInstance failed on round %d: %s", r, err)
		}
	}
}
