// Copyright 2023 The LUCI Authors.
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

package exec

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/exec/internal/execmockctx"
	"go.chromium.org/luci/common/system/environ"
)

var curExe, curExeErr = os.Executable()

// ErrUseConstructor is returned from methods of Cmd if the Cmd struct was
// created without using Command or CommandContext.
var ErrUseConstructor = errors.New("you must use Command or CommandContext to create a usable Cmd")

type mockPayload struct {
	// mocker is only set if mocking is enabled for this process.
	mocker execmockctx.CreateMockInvocation

	// We store the original user-provided `name` for this Cmd in case we need to
	// fall back to passthrough mode.
	originalName string

	// If mocked, this is a unique ID for this invocation. 0 means 'not mocked'.
	invocationID uint64

	// true iff this Cmd should be `chatty`
	chatty bool

	// buffering stdout for Output (and other modes), or stdout+stderr for CombinedOutput
	stdoutBuf *bytes.Buffer
	// buffering stderr for Output (and other modes).
	stderrBuf *bytes.Buffer
}

// Cmd behaves like an "os/exec".Cmd, except that it can be mocked using the
// "go.chromium.org/luci/common/exec/execmock" package.
//
// Must be created with Command or CommandContext.
//
// Mocking this Cmd allows the test program to substitute this Cmd invocation
// transparently with another, customized, subprocess.
type Cmd struct {
	// We embed the *Cmd without a field name so that users can drop in our struct
	// where they previously had an *"os/exec".Cmd with minimal code changes.
	*exec.Cmd

	// set to true in Command and CommandContext.
	safelyCreated bool

	mock      *mockPayload
	waitDefer func()
}

func multiWriter(a, b io.Writer) io.Writer {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return io.MultiWriter(a, b)
}

var chattyMu sync.Mutex

// chattySession implements a sprintf-like function to render a line which will
// go into an internal buffer, and a closer, which will write the whole buffer
// synchronously.
//
// This construction reduces the amount of mixed chatty output when running
// multiple mocked processes in parallel.
//
// Unfortunately, in chatty mode, there will be some synchronization introduced
// to the application which is not present in non-chatty mode, but this probably
// can't be helped much; Firing the buffered blocks off to a goroutine sort of
// works, but you need to introduce a way to close the chatty channel and wait
// for the goroutine to complete; otherwise you risk losing random chunks (or
// maybe all!) of the chatty output.
type chattySession struct {
	buf []string
}

func (c *chattySession) printlnf(msg string, args ...any) {
	c.buf = append(c.buf, fmt.Sprintf(msg, args...))
}

func (c *chattySession) dump() {
	toWrite := strings.Join(c.buf, "\n") + "\n"
	chattyMu.Lock()
	defer chattyMu.Unlock()
	os.Stderr.WriteString(toWrite)
}

// applyMock will look up a mock from the `mocker` and then adjust the state of
// the underlying Cmd to run in the mocked context.
//
// This includes adjusting the environment (to set the mock key) changing
// c.Path (to point at our own binary), setting c.stdxxxBuf if we're in chatty
// mode.
//
// It is possible for the exact mock result to be `passthrough` which means that
// `applyMock` will restore this Cmd to it's original state.
//
// applyMock will only ever do it's action to cover a single Start event; just
// like the underlying exec.Cmd object, this Cmd is not meant to be used
// multiple times (it will enter an inconsistent state).
func (c *Cmd) applyMock() (restore func(), mock *mockPayload, err error) {
	if !c.safelyCreated {
		c.Err = ErrUseConstructor
	}
	if c.Err != nil {
		return nil, nil, c.Err
	}
	if c.mock == nil {
		return func() {}, nil, nil
	}

	mock = c.mock
	invocation, err := mock.mocker(execmockctx.NewMockCriteria(c.Cmd), &c.Cmd.Process)
	c.mock = nil // we should never try to mock this Cmd again
	if err != nil {
		c.Err = err
		return nil, nil, c.Err
	}
	if invocation == nil {
		// passthrough mode; we have to un-mock and possibly do a LookPath.
		c.Path = mock.originalName
		if filepath.Base(c.Path) == c.Path {
			// NOTE: We don't want to use LookPath from this module's namespace to
			// prevent accidental overrides, so we use exec.LookPath explicitly.
			lp, err := exec.LookPath(c.Path)
			if lp != "" {
				c.Path = lp
			}
			if err != nil {
				c.Err = err
			}
		}
		return func() {}, nil, c.Err
	}

	mock.invocationID = invocation.ID

	oldPath := c.Path
	oldEnv := c.Env

	sysEnv := environ.System()

	if mock.chatty {
		// stdoutBuf or stderrBuf may already be set if StdxxxPipe have already been
		// called. If they have, then we don't want to touch them again, here, since
		// StdxxxPipe have already set them to be correctly read by Wait()'s chatty
		// printer.
		if mock.stdoutBuf == nil {
			mock.stdoutBuf = &bytes.Buffer{}
			c.Stdout = multiWriter(mock.stdoutBuf, c.Stdout)
		}
		if mock.stderrBuf == nil {
			mock.stderrBuf = &bytes.Buffer{}
			c.Stderr = multiWriter(mock.stderrBuf, c.Stderr)
		}

		var chat chattySession
		defer chat.dump()

		chat.printlnf("execmock: Start(invocation=%d): %q", invocation.ID, c.Args)
		if c.Dir != "" {
			chat.printlnf("  cwd: %s", c.Dir)
		}
		if c.Env != nil {
			diffEnv := sysEnv.Clone()
			cmdEnv := environ.New(c.Env)
			_ = cmdEnv.Iter(func(k, v string) error {
				sysVal, sysHas := diffEnv.Lookup(k)
				if sysHas {
					if sysVal != v {
						chat.printlnf("  env~ %s=%s", k, v)
					}
				} else {
					chat.printlnf("  env+ %s=%s", k, v)
				}
				diffEnv.Remove(k)
				return nil
			})
			// diffEnv now contains envvars which aren't in cmdEnv
			_ = diffEnv.Iter(func(k, v string) error {
				chat.printlnf("  env- %s", k)
				return nil
			})
		}
	}

	if curExeErr != nil {
		// should be impossible due to check in CommandContext... but double check
		// here just in case.
		panic(errors.Fmt("impossible: %w", curExeErr))
	}
	c.Path = curExe
	if c.Env == nil {
		c.Env = append(sysEnv.Sorted(), invocation.EnvVar)
	} else {
		c.Env = append(c.Env, invocation.EnvVar)
	}

	if mock.chatty {
		c.waitDefer = func() {
			// not entirely sure how ProcessState could be nil, but check it, just the
			// same.
			if c.ProcessState != nil {
				c.waitDefer = nil

				outbuf, errbuf := mock.stdoutBuf, mock.stderrBuf
				runnerPanic, runnerErr := invocation.GetErrorOutput()

				var chat chattySession
				defer chat.dump()

				chat.printlnf("execmock: Wait(invocation=%d): %v", mock.invocationID, c.ProcessState)
				if runnerErr != nil {
					chat.printlnf("  ERROR: %s", runnerErr)
				}
				if runnerPanic != "" {
					chat.printlnf("  PANIC:")
					for scn := bufio.NewScanner(strings.NewReader(runnerPanic)); scn.Scan(); {
						chat.printlnf("    !> %s", scn.Bytes())
					}
				}

				// we use bytes.NewReader because `buf` itself may still be held by
				// CombinedOutput. This won't actually do any additional allocations, but
				// it will prevent the buffer in `buf` from being consumed before
				// CombinedOutput can return it.
				if outbuf != nil {
					for scn := bufio.NewScanner(bytes.NewReader(outbuf.Bytes())); scn.Scan(); {
						chat.printlnf("  O> %s", scn.Bytes())
					}
				}
				if errbuf != nil {
					for scn := bufio.NewScanner(bytes.NewReader(errbuf.Bytes())); scn.Scan(); {
						chat.printlnf("  E> %s", scn.Bytes())
					}
				}
			}
		}
	}

	restore = func() {
		c.Path = oldPath
		c.Env = oldEnv
	}
	return
}

// CombinedOutput operates the same as "os/exec".Cmd.CombinedOutput.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	stdoutSet := c.Stdout != nil
	stderrSet := c.Stderr != nil

	restore, mock, err := c.applyMock()
	if err != nil {
		return nil, err
	}
	defer restore()

	if mock == nil || !mock.chatty {
		return c.Cmd.CombinedOutput()
	}

	// we can't use c.Cmd.CombinedOutput in chatty mode because they assert that
	// Stdout/Stderr aren't already set.
	//
	// However, in chatty mode we already have c.stdoutBuf which will collect
	// exactly what we want for CombinedOutput anyway.
	if stdoutSet {
		return nil, errors.New("execmock: Stdout already set")
	}
	if stderrSet {
		return nil, errors.New("execmock: Stderr already set")
	}

	// At this point applyMock has set up two buffers to capture stdout/stderr.
	// We want to combine them, so just remove stderrBuf and duplicate stdoutBuf.
	mock.stderrBuf = nil
	c.Stderr = mock.stdoutBuf

	// keep a ref to stdoutBuf so that the post-Wait chatty printer doesn't
	// replace it with nil.
	buf := mock.stdoutBuf
	err = c.Run()
	return buf.Bytes(), err
}

// Output operates the same as "os/exec".Cmd.Output.
func (c *Cmd) Output() ([]byte, error) {
	stdoutSet := c.Stdout != nil
	captureStderr := c.Cmd.Stderr == nil

	restore, mock, err := c.applyMock()
	if err != nil {
		return nil, err
	}
	defer restore()

	if mock == nil || !mock.chatty {
		return c.Cmd.Output()
	}

	// we can't use c.Cmd.Output in chatty mode because it asserts that Stdout is
	// not already set.
	if stdoutSet {
		return nil, errors.New("execmock: Stdout already set")
	}

	// keep refs to stdxxxBuf so we can use them after the post-Wait chatty
	// printer nil's them out.
	stdoutBuf := mock.stdoutBuf
	var stderrBuf *bytes.Buffer
	if captureStderr {
		stderrBuf = mock.stderrBuf
	}

	err = c.Run()
	if err != nil && captureStderr {
		if xerr, ok := err.(*exec.ExitError); ok {
			xerr.Stderr = stderrBuf.Bytes()
		}
	}

	return stdoutBuf.Bytes(), err
}

// Run operates the same as "os/exec".Cmd.Run.
func (c *Cmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

// Start operates the same as "os/exec".Cmd.Start.
func (c *Cmd) Start() error {
	restore, _, err := c.applyMock()
	if err != nil {
		return err
	}
	defer restore()

	return c.Cmd.Start()
}

// Wait operates the same as "os/exec".Cmd.Wait.
func (c *Cmd) Wait() error {
	if c.waitDefer != nil {
		defer c.waitDefer()
	}

	return c.Cmd.Wait()
}

type fakePipeReader struct {
	io.Reader
	io.Closer
}

// StderrPipe operates the same as "os/exec".Cmd.StderrPipe.
func (c *Cmd) StderrPipe() (io.ReadCloser, error) {
	reader, err := c.Cmd.StderrPipe()
	if err != nil || c.mock == nil || !c.mock.chatty {
		return reader, err
	}

	c.mock.stderrBuf = &bytes.Buffer{}
	return &fakePipeReader{io.TeeReader(reader, c.mock.stderrBuf), reader}, nil
}

// StdoutPipe operates the same as "os/exec".Cmd.StdoutPipe.
func (c *Cmd) StdoutPipe() (io.ReadCloser, error) {
	reader, err := c.Cmd.StdoutPipe()
	if err != nil || c.mock == nil || !c.mock.chatty {
		return reader, err
	}

	c.mock.stdoutBuf = &bytes.Buffer{}
	return &fakePipeReader{io.TeeReader(reader, c.mock.stdoutBuf), reader}, nil
}

// We don't need to emulate these; the native cmd impl should take care of it.
// func (c *Cmd) String() string
// func (c *Cmd) StdinPipe() (io.WriteCloser, error)

// mocked in tests
var getMockCreator = execmockctx.GetMockCreator

func commandImpl(ctx context.Context, name string, arg []string, mkFn func(ctx context.Context, name string, arg ...string) *exec.Cmd) *Cmd {
	ret := &Cmd{safelyCreated: true}
	mocker, chatty := getMockCreator(ctx)
	if mocker != nil {
		ret.mock = &mockPayload{mocker: mocker, chatty: chatty}
	} else {
		ret.Cmd = mkFn(ctx, name, arg...)
		return ret
	}

	ret.mock.originalName = name
	if curExeErr != nil {
		ret.Err = errors.Fmt("cannot resolve os.Executable for execmock: %w", curExeErr)
		return ret
	}
	// We use curExe as the target executable because exec.CommandContext does
	// an implicit LookPath when `name` is non-absolute; If this is some binary
	// like "git" then CommandContext would do a LookPath "for real" even during
	// tests.
	//
	// We generate Path and Args as if they were resolved though, so that
	// programs can print them without unexpected output.
	ret.Cmd = mkFn(ctx, curExe)
	if filepath.Base(name) != name {
		// The user gave a name which would not be resolved via PATH; restore it.
		ret.Cmd.Path = name
	} else {
		// The user gave a `name` with the intent of resolving it from PATH.
		//
		// In this branch we "did the LookPath", and Path is expected to be (some)
		// absolute path; We know this context contains test mocks, so make
		// something up here which will be obvious if printed.
		if runtime.GOOS == "windows" {
			ret.Cmd.Path = filepath.Join(`C:\execmock\PATH`, name)
		} else {
			ret.Cmd.Path = filepath.Join(`/execmock/PATH`, name)
		}
	}
	ret.Cmd.Args = append([]string{name}, arg...)
	return ret
}

// CommandContext behaves like "os/exec".CommandContext, except that it can be
// mocked via the "go.chromium.org/luci/common/exec/execmock" package.
func CommandContext(ctx context.Context, name string, arg ...string) *Cmd {
	return commandImpl(ctx, name, arg, exec.CommandContext)
}

// Command behaves like "os/exec".Command, except that it can be
// mocked via the "go.chromium.org/luci/common/exec/execmock" package.
//
// Note that although this takes a Context, it does not bind the lifetime of
// the returned Cmd to `ctx`.
func Command(ctx context.Context, name string, arg ...string) *Cmd {
	return commandImpl(ctx, name, arg, func(_ context.Context, name string, arg ...string) *exec.Cmd {
		return exec.Command(name, arg...)
	})
}
