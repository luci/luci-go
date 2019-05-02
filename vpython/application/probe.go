// Copyright 2017 The LUCI Authors.
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

package application

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.chromium.org/luci/vpython/python"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/common/system/prober"
)

const (
	// CheckWrapperENV is an environment variable that is used to detect if a
	// given binary is a "vpython" wrapper instance. The "vpython" Main function
	// will immediately exit with a non-zero return code if this environment
	// variable is set.
	//
	// See checkWrapper for more details.
	checkWrapperENV = "VPYTHON_CHECK_WRAPPER"

	// checkWrapperSentinel is text that will be output by a "vpython" instance if
	// checkWrapperENV is set. If an application exits with a non-zero error code
	// and outputs this text to STDOUT, then it is considered a "vpython" wrapper.
	checkWrapperSentinel = checkWrapperENV + ": vpython wrapper check, exiting with non-zero return code"
)

func wrapperCheck(env environ.Env) bool {
	if env.GetEmpty(checkWrapperENV) != "" {
		fmt.Fprintln(os.Stdout, checkWrapperSentinel)
		return true
	}
	return false
}

type lookPath struct {
	// probeBase is a base prober whose Self and SelfStat fields will be used
	// during a lookPathImpl call. This ensures that we only have to call
	// ResolveSelf once for any given application.
	probeBase prober.Probe

	filter python.LookPathFilter

	env        environ.Env
	lastPath   string
	lastOutput string
}

func (lp *lookPath) look(c context.Context, target string, filter python.LookPathFilter) (*python.LookPathResult, error) {
	// Construct our probe, derived it from our "probeBase" and tailored to the
	// specific "target" we're looking for.
	probe := lp.probeBase
	probe.Target = target
	probe.CheckWrapper = lp.checkWrapper

	lp.filter = filter

	var result python.LookPathResult
	var err error
	if result.Path, err = probe.Locate(c, "", lp.env.Clone()); err != nil {
		return nil, err
	}

	// During probing, we try and capture the output of platform.python_version().
	// If we do, and if we confirm that the "path" returned by the probe matches
	// "lastPath", return this version along with the resolution.
	if result.Path == lp.lastPath {
		var err error
		if result.Version, err = python.ParseVersion(lp.lastOutput); err == nil {
			logging.Debugf(c, "Detected Python version %q from probe candidate [%s]", result.Version, result.Path)
		} else {
			logging.Debugf(c, "Failed to parse version from probe candidate [%s]: %s", result.Path, err)
		}
	}

	return &result, nil
}

// checkWrapper is a prober.CheckWrapperFunc that detects if a given path is a
// "vpython" instance.
//
// It does this by running it "path -c ...." with CheckWrapperENV set. If
// the target is a "vpython" wrapper, it will immediately exit with a non-zero
// value (see Main). If it is not a valid Python program, its behavior is
// undefined. If it is a valid Python instance, it will exit with a Python
// version string.
//
// As a side effect, the text output of the last version probe will be stored
// in lp.lastOutput. The "look" function can pass this value on to
// the LookPathResult.
func (lp *lookPath) checkWrapper(c context.Context, path string, env environ.Env) (isWrapper bool, err error) {
	env.Set(checkWrapperENV, "1")

	// We use 'sys.write' instead of 'print' so this will work for both py2 and
	// py3.
	cmd := exec.CommandContext(c, path, "-c",
		"import platform, sys; sys.stdout.write(platform.python_version())")
	cmd.Env = env.Sorted()

	output, err := cmd.CombinedOutput()
	rc, ok := exitcode.Get(err)
	if !ok {
		err = errors.Annotate(err, "failed to check if %q is a wrapper", path).Err()
		return
	}

	// Retain the last output, so our "look" can reference it to extract the
	// version string.
	lp.lastPath = path
	lp.lastOutput = string(output)

	if rc != 0 {
		// The target exited with a non-zero error code. It is a wrapper if it
		// printed the sentinel text.
		isWrapper = strings.TrimSpace(lp.lastOutput) == checkWrapperSentinel
		if isWrapper {
			// The target was successfully identified as a wrapper. Clear "err", which
			// is non-nil b/c of the non-zero exit code, to indicate a successful
			// wrapper check.
			err = nil
			return
		}

		// The target returned non-zero, but didn't identify as a wrapper. It is
		// likely something that happens to be named the same thing as the target,
		// which is an error.
		err = errors.Annotate(err, "wrapper check returned non-zero").Err()
	}

	// If this isn't a wrapper, check if it meets our criteria
	err = lp.filter(c, &python.Interpreter{Python: path})

	return
}
