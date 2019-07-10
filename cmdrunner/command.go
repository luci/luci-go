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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/exec2"
)

const (
	executableSuffixParameter = "${EXECUTABLE_SUFFIX}"
	isolatedOutdirParameter   = "${ISOLATED_OUTDIR}"
	swarmingBotFileParameter  = "${SWARMING_BOT_FILE}"
)

// ErrHardTimeout is error for timeout from Run command.
var ErrHardTimeout = errors.Reason("timeout happens").Err()

// replaceParameters replaces parameter tokens with appropriate values in a
// string.
func replaceParameters(ctx context.Context, arg, outDir, botFile string) (string, error) {

	if runtime.GOOS == "windows" {
		arg = strings.Replace(arg, executableSuffixParameter, ".exe", -1)
	} else {
		arg = strings.Replace(arg, executableSuffixParameter, "", -1)
	}
	replaceSlash := false

	if strings.Contains(arg, isolatedOutdirParameter) {
		if outDir == "" {
			return "", errors.Reason("output directory is requested in command or env var, but not provided; please specify one").Err()
		}
		arg = strings.Replace(arg, isolatedOutdirParameter, outDir, -1)
		replaceSlash = true
	}

	if strings.Contains(arg, swarmingBotFileParameter) {
		if botFile != "" {
			arg = strings.Replace(arg, swarmingBotFileParameter, botFile, -1)
			replaceSlash = true
		} else {
			logging.Warningf(ctx, "swarmingBotFileParameter found in command or env var, but no bot_file specified. Leaving parameter unchanged.")
		}
	}

	if replaceSlash {
		arg = strings.Replace(arg, "/", string(filepath.Separator), -1)
	}

	return arg, nil
}

// processCommand replaces parameters in a command line.
func processCommand(ctx context.Context, command []string, outDir, botFile string) ([]string, error) {
	newCommand := make([]string, 0, len(command))
	for _, arg := range command {
		newArg, err := replaceParameters(ctx, arg, outDir, botFile)
		if err != nil {
			return nil, fmt.Errorf("failed to replace parameter %s: %v", arg, err)
		}
		newCommand = append(newCommand, newArg)
	}
	return newCommand, nil
}

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
		p, err := replaceParameters(ctx, v, outDir, botFile)
		if err != nil {
			return fmt.Errorf("failed to call replaceParameters for %s: %v", v, err)
		}
		out.Set(k, p)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if cipdInfo != nil {
		binDir := filepath.Dir(cipdInfo.binaryPath)
		out.Set("PATH", binDir+string(filepath.ListSeparator)+out.GetEmpty("PATH"))
		out.Set("CIPD_CACHE_DIR", cipdInfo.cacheDir)
	}

	for key, paths := range envPrefixes {
		newPaths := make([]string, 0, len(paths))
		for _, p := range paths {
			newPaths = append(newPaths, filepath.Clean(filepath.Join(runDir, p)))
		}
		if cur, ok := out.Get(key); ok {
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
		if _, ok := environ.System().Get("SWARMING_TASK_ID"); ok {
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
