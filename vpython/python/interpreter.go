// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package python

import (
	"os/exec"
	"strings"
	"sync"

	"github.com/luci/luci-go/common/errors"

	"golang.org/x/net/context"
)

type runnerFunc func(cmd *exec.Cmd, capture bool) (string, error)

// Interpreter represents a system Python interpreter. It exposes the ability
// to use common functionality of that interpreter.
type Interpreter struct {
	// Python is the path to the system Python interpreter.
	Python string

	// cachedVersion is the cached Version for this interpreter. It is populated
	// on the first GetVersion call.
	cachedVersion   *Version
	cachedVersionMu sync.Mutex

	// testCommandHook, if not nil, is called on generated Command results prior
	// to returning them.
	testCommandHook func(*exec.Cmd)
}

// IsolatedCommand returns a configurable exec.Cmd structure bound to this
// Interpreter.
//
// The supplied arguments have several Python isolation flags prepended to them
// to remove environmental factors such as:
//	- The user's "site.py".
//	- The current PYTHONPATH environment variable.
//	- Compiled ".pyc/.pyo" files.
func (i *Interpreter) IsolatedCommand(c context.Context, args ...string) *exec.Cmd {
	// Isolate the supplied arguments.
	args = append([]string{
		"-B", // Don't compile "pyo" binaries.
		"-E", // Don't use PYTHON* environment variables.
		"-s", // Don't use user 'site.py'.
	}, args...)

	cmd := exec.CommandContext(c, i.Python, args...)
	if i.testCommandHook != nil {
		i.testCommandHook(cmd)
	}
	return cmd
}

// GetVersion runs the specified Python interpreter with the "--version"
// flag and maps it to a known specification verison.
func (i *Interpreter) GetVersion(c context.Context) (v Version, err error) {
	i.cachedVersionMu.Lock()
	defer i.cachedVersionMu.Unlock()

	// Check again, under write-lock.
	if i.cachedVersion != nil {
		v = *i.cachedVersion
		return
	}

	// We use CombinedOutput here becuase Python2 writes the version to STDERR,
	// while Python3+ writes it to STDOUT.
	cmd := i.IsolatedCommand(c, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = errors.Annotate(err).Err()
		return
	}

	if v, err = parseVersionOutput(strings.TrimSpace(string(out))); err != nil {
		err = errors.Annotate(err).Err()
		return
	}

	i.cachedVersion = &v
	return
}

func parseVersionOutput(output string) (Version, error) {
	// Expected output:
	// Python X.Y.Z
	parts := strings.SplitN(output, " ", 2)
	if len(parts) != 2 || parts[0] != "Python" {
		return Version{}, errors.Reason("unknown version output").
			D("output", output).
			Err()
	}

	v, err := ParseVersion(parts[1])
	if err != nil {
		err = errors.Annotate(err).Reason("failed to parse version from: %(value)q").
			D("value", parts[1]).
			Err()
		return v, err
	}
	return v, nil
}
