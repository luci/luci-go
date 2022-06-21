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

package cmdrunner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl"
	clientswarming "go.chromium.org/luci/client/swarming"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/exec2"
	"go.chromium.org/luci/common/system/filesystem"
)

// ErrHardTimeout is error for timeout from Run command.
var ErrHardTimeout = errors.Reason("timeout happens").Err()

type cipdInfo struct {
	binaryPath string
	cacheDir   string
}

// environSystem is used for mocking in test.
var environSystem = environ.System

// getCommandEnv returns full OS environment to run a command in.
// Sets up TEMP, puts directory with cipd binary in front of PATH, exposes
// CIPD_CACHE_DIR env var, and installs all env_prefixes.
func getCommandEnv(ctx context.Context, tmpDir string, cipdInfo *cipdInfo, runDir string, env environ.Env, envPrefixes map[string][]string, outDir, botFile string) (environ.Env, error) {
	out := environSystem()

	err := env.Iter(func(k, v string) error {
		if v == "" {
			out.Remove(k)
			return nil
		}
		p, err := clientswarming.ReplaceCommandParameters(ctx, v, outDir, botFile)
		if err != nil {
			return fmt.Errorf("failed to call replaceParameters for %s: %v", v, err)
		}
		out.Set(k, p)
		return nil
	})
	if err != nil {
		return environ.Env{}, err
	}

	if cipdInfo != nil {
		binDir := filepath.Dir(cipdInfo.binaryPath)
		out.Set("PATH", binDir+string(filepath.ListSeparator)+out.Get("PATH"))
		out.Set("CIPD_CACHE_DIR", cipdInfo.cacheDir)
	}

	for key, paths := range envPrefixes {
		newPaths := make([]string, 0, len(paths))
		for _, p := range paths {
			newPaths = append(newPaths, filepath.Clean(filepath.Join(runDir, p)))
		}
		if cur, ok := out.Lookup(key); ok {
			newPaths = append(newPaths, cur)
		}
		out.Set(key, strings.Join(newPaths, string(filepath.ListSeparator)))
	}

	// * python respects $TMPDIR, $TEMP, and $TMP in this order, regardless of
	//   platform. So $TMPDIR must be set on all platforms.
	//   https://github.com/python/cpython/blob/2.7/Lib/tempfile.py#L155
	out.Set("TMPDIR", tmpDir)
	if runtime.GOOS == "windows" {
		// * chromium's base utils uses GetTempPath().
		//    https://cs.chromium.org/chromium/src/base/files/file_util_win.cc?q=GetTempPath
		// * Go uses GetTempPath().
		// * GetTempDir() uses %TMP%, then %TEMP%, then other stuff. So %TMP% must be
		//   set.
		//   https://docs.microsoft.com/en-us/windows/desktop/api/fileapi/nf-fileapi-gettemppathw
		out.Set("TMP", tmpDir)
		// https://blogs.msdn.microsoft.com/oldnewthing/20150417-00/?p=44213
		out.Set("TEMP", tmpDir)
	} else if runtime.GOOS == "darwin" {
		// * Chromium uses an hack on macOS before calling into
		//   NSTemporaryDirectory().
		//   https://cs.chromium.org/chromium/src/base/files/file_util_mac.mm?q=GetTempDir
		//   https://developer.apple.com/documentation/foundation/1409211-nstemporarydirectory
		out.Set("MAC_CHROMIUM_TMPDIR", tmpDir)
	} else {
		// TMPDIR is specified as the POSIX standard envvar for the temp directory.
		// http://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap08.html
		// * mktemp on linux respects $TMPDIR.
		// * Chromium respects $TMPDIR on linux.
		//   https://cs.chromium.org/chromium/src/base/files/file_util_posix.cc?q=GetTempDir
		// * Go uses $TMPDIR.
		//   https://go.googlesource.com/go/+/go1.10.3/src/os/file_unix.go#307
	}

	return out, nil
}

// Run runs the command.
func Run(ctx context.Context, command []string, cwd string, env environ.Env, hardTimeout time.Duration, gracePeriod time.Duration, lowerPriority bool, containment bool) (int, error) {
	logging.Infof(ctx, "runCommand(%s, %s, %s, %s, %s, %s, %s)", command, cwd, env, hardTimeout, gracePeriod, lowerPriority, containment)

	cmd := exec2.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = env.Sorted()
	cmd.Dir = cwd
	// TODO(tikuta): handle STOP_SIGNALS

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run command %s: %v\n", command, err)
		fmt.Fprint(os.Stderr, `<The executable does not exist or a dependent library is missing>
<Check for missing .so/.dll in the .isolate or GN file>
`)
		if _, ok := environ.System().Lookup(swarmingimpl.TaskIDEnvVar); ok {
			fmt.Fprint(os.Stderr, `<See the task's page for commands to help diagnose this issue by reproducing the task locally>
`)
		}
		return 1, errors.Annotate(err, "failed to start command").Err()
	}

	errCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if e, ok := err.(*exec.ExitError); ok && e.Exited() {
			// Ignore exited error.
			err = nil
		} else {
			cmd.Kill()
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return 1, err
		}
		return cmd.ProcessState.ExitCode(), nil
	case <-time.After(hardTimeout):
	}

	if err := cmd.Terminate(); err != nil {
		logging.Warningf(ctx, "failed to call Terminate: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			return 1, err
		}
		// Process exited fast enough after a nudge. Happy path.
		return cmd.ProcessState.ExitCode(), ErrHardTimeout
	case <-time.After(gracePeriod):
	}

	// Process didn't exit in time after a nudge, try to kill it.
	cmd.Kill()
	return 1, ErrHardTimeout
}

func linkOutputsToOutdir(runDir, outDir string, outputs []string) error {
	if err := filesystem.CreateDirectories(outDir, outputs); err != nil {
		return errors.Annotate(err, "failed to create directory").Err()
	}

	for _, output := range outputs {
		src := filepath.Join(runDir, output)
		dst := filepath.Join(outDir, output)
		if err := filesystem.HardlinkRecursively(src, dst); err != nil {
			return errors.Annotate(err, "failed to copy output from %s to %s", src, dst).Err()
		}
	}

	return nil
}

// UploadStats is stats of upload or uploadThenDelete.
type UploadStats struct {
	Duration time.Duration `json:"duration"`

	ItemsCold []byte `json:"items_cold"`
	ItemsHot  []byte `json:"items_hot"`
}

func upload(ctx context.Context, baseDir, outDir string) (*UploadStats, error) {
	start := time.Now()
	absOutDir := filepath.Join(baseDir, outDir)

	isEmpty, err := filesystem.IsEmptyDir(absOutDir)
	if err != nil {
		return nil, errors.Annotate(err, "failed to call IsEmptyDir(%s)", absOutDir).Err()
	}

	var stats UploadStats

	if !isEmpty {
		// TODO: implement CAS archive.
	}

	stats.Duration = time.Now().Sub(start)
	return &stats, nil
}

func uploadThenDelete(ctx context.Context, baseDir, outDir string) (stats *UploadStats, err error) {
	start := time.Now()

	defer func() {
		absOutDir := filepath.Join(baseDir, outDir)
		removeErr := filesystem.RemoveAll(absOutDir)
		if err == nil && removeErr != nil {
			err = errors.Annotate(removeErr, "failed to call RemoveAll(%s)", absOutDir).Err()
		}
		stats.Duration = time.Now().Sub(start)
	}()

	stats, err = upload(ctx, baseDir, outDir)
	if err != nil {
		return nil, errors.Annotate(err, "failed to call upload").Err()
	}

	return stats, nil
}
