// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package python

import (
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/system/exitcode"

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

	// testRunner is the runner function to use to run an exec.Cmd. This can be
	// swapped out for testing.
	testRunner runnerFunc
}

// Command returns a configurable command structure bound to this Interpreter.
func (i *Interpreter) Command() *Command {
	return &Command{
		Python:     i.Python,
		testRunner: i.testRunner,
	}
}

// GetVersion runs the specified Python interpreter with the "--version"
// flag and maps it to a known specification verison.
func (i *Interpreter) GetVersion(c context.Context) (v Version, err error) {
	if i.Python == "" {
		err = errors.New("missing Python interpreter")
		return
	}

	i.cachedVersionMu.Lock()
	defer i.cachedVersionMu.Unlock()

	// Check again, under write-lock.
	if i.cachedVersion != nil {
		v = *i.cachedVersion
		return
	}

	// We use CombinedOutput here becuase Python2 writes the version to STDERR,
	// while Python3+ writes it to STDOUT.
	cmd := exec.CommandContext(c, i.Python, "--version")

	rf := i.testRunner
	if rf == nil {
		rf = defaultRunnerFunc
	}

	out, err := rf(cmd, true)
	if err != nil {
		err = errors.Annotate(err).Reason("failed to get Python version").Err()
		return
	}
	if v, err = parseVersionOutput(strings.TrimSpace(out)); err != nil {
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

// Command can run Python commands using an Interpreter.
//
// It is created using an Interpreter's Command method.
type Command struct {
	// Python is the path to the Python interpreter to use. It is automatically
	// populated from an Interpreter when created through an Interpreter's Command
	// method.
	Python string

	// WorkDir is the working directory to use when running the interpreter. If
	// empty, the current working directory will be used.
	WorkDir string

	// ConnectSTDIN will cause this process' STDIN to be passed through to the
	// Python subprocess. Otherwise, the Python subprocess will receive a closed
	// STDIN.
	ConnectSTDIN bool

	// Isolated means that the Python invocation should include flags to isolate
	// it from local system modification.
	//
	// This removes environmental factors such as:
	// - The user's "site.py".
	// - The current PYTHONPATH environment variable.
	// - Compiled ".pyc/.pyo" files.
	Isolated bool

	// Env, if not nil, is the environment to supply.
	Env []string

	// testRunner is the runner function to use to run an exec.Cmd. This can be
	// swapped out for testing.
	testRunner runnerFunc
}

// Run runs the configured Command with the supplied arguments.
//
// Run returns wrapped errors. Use errors.Unwrap to get the main cause, if
// needed. If an error occurs during setup or invocation, it will be returned
// directly. If the interpreter runs and returns zero, nil will be returned. If
// the interpreter runs and returns non-zero, an Error instance will be returned
// containing that return code.
func (ic *Command) Run(c context.Context, args ...string) error {
	if ic.Python == "" {
		return errors.New("a Python interpreter must be supplied")
	}

	if ic.Isolated {
		args = append([]string{
			"-B", // Don't compile "pyo" binaries.
			"-E", // Don't use PYTHON* enviornment variables.
			"-s", // Don't use user 'site.py'.
		}, args...)
	}

	cmd := exec.CommandContext(c, ic.Python, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if ic.ConnectSTDIN {
		cmd.Stdin = os.Stdin
	}
	cmd.Dir = ic.WorkDir
	cmd.Env = ic.Env

	if logging.IsLogging(c, logging.Debug) {
		logging.Debugf(c, "Running Python command (cwd=%s): %s",
			cmd.Dir, strings.Join(cmd.Args, " "))
	}

	// Allow testing to supply an alternative runner function.
	rf := ic.testRunner
	if rf == nil {
		rf = defaultRunnerFunc
	}

	if _, err := rf(cmd, false); err != nil {
		// If the process failed because of a non-zero return value, return that
		// as our error.
		if rc, has := exitcode.Get(err); has {
			return errors.Annotate(Error(rc)).Reason("Python bootstrap returned non-zero").Err()
		}

		return errors.Annotate(err).Reason("failed to run Python command").
			D("python", ic.Python).
			D("args", args).
			Err()
	}
	return nil
}

func defaultRunnerFunc(cmd *exec.Cmd, capture bool) (string, error) {
	// If we're capturing output, combine STDOUT and STDERR (see GetVersion
	// for details).
	if capture {
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", errors.Annotate(err).Err()
		}
		return string(out), nil
	}

	// Non-capturing run.
	if err := cmd.Run(); err != nil {
		return "", errors.Annotate(err).Err()
	}
	return "", nil
}
