// Copyright 2015 The LUCI Authors.
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

package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/golang/protobuf/proto"

	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/logdog/common/types"

	"go.chromium.org/luci/luciexe/legacy/annotee"
	"go.chromium.org/luci/luciexe/legacy/annotee/annotation"
)

// Executor bootstraps an application, running its output through a Processor.
type Executor struct {
	// Options are the set of Annotee options to use.
	Options annotee.Options

	// Stdin, if not nil, will be used as standard input for the bootstrapped
	// process.
	Stdin io.Reader

	// TeeStdout, if not nil, is a Writer where bootstrapped process standard
	// output will be tee'd.
	TeeStdout io.Writer
	// TeeStderr, if not nil, is a Writer where bootstrapped process standard
	// error will be tee'd.
	TeeStderr io.Writer

	executed   bool
	returnCode int

	// step is the serialized milo.Step protobuf taken from the end of the
	// Processor at execution finish.
	step []byte
}

// Run executes the bootstrapped process, blocking until it completes.
func (e *Executor) Run(ctx context.Context, command []string) error {
	// Clear any previous state.
	e.executed = false
	e.returnCode = 0
	e.step = nil

	if len(command) == 0 {
		return errors.New("no command")
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)

	// STDOUT
	stdoutRC, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create STDOUT pipe: %s", err)
	}
	defer stdoutRC.Close()
	stdout := e.configStream(stdoutRC, annotee.STDOUT, e.TeeStdout, true)

	stderrRC, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create STDERR pipe: %s", err)
	}
	defer stderrRC.Close()
	stderr := e.configStream(stderrRC, annotee.STDERR, e.TeeStderr, false)

	// Start our process.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start bootstrapped process: %s", err)
	}

	// Cleanup the process on exit, and record its status and return code.
	defer func() {
		if err := cmd.Wait(); err != nil {
			var ok bool
			if e.returnCode, ok = exitcode.Get(err); ok {
				e.executed = true
			} else {
				log.WithError(err).Errorf(ctx, "Failed to Wait() for bootstrapped process.")
			}
		} else {
			e.returnCode = 0
			e.executed = true
		}
	}()

	// Probe our execution information.
	options := e.Options
	if options.Execution == nil {
		options.Execution = annotation.ProbeExecution(command, nil, "")
	}

	// Configure our Processor.
	streams := []*annotee.Stream{
		stdout,
		stderr,
	}

	// Process the bootstrapped I/O. We explicitly defer a Finish here to ensure
	// that we clean up any internal streams if our Processor fails/panics.
	//
	// If we fail to process the I/O, terminate the bootstrapped process
	// immediately, since it may otherwise block forever on I/O.
	proc := annotee.New(ctx, options)
	defer proc.Finish()

	if err := proc.RunStreams(streams); err != nil {
		return fmt.Errorf("failed to process bootstrapped I/O: %v", err)
	}

	// Finish and record our annotation steps on completion.
	if e.step, err = proto.Marshal(proc.Finish().RootStep().Proto()); err != nil {
		log.WithError(err).Errorf(ctx, "Failed to Marshal final Step protobuf on completion.")
		return err
	}
	return nil
}

// Step returns the root Step protobuf from the latest run.
func (e *Executor) Step() []byte { return e.step }

// ReturnCode returns the executed process' return code.
//
// If the process hasn't completed its execution (see Executed), then this will
// return 0.
func (e *Executor) ReturnCode() int {
	return e.returnCode
}

// Executed returns true if the bootstrapped process' execution completed
// successfully. This is independent of the return value, and can be used to
// differentiate execution errors from process errors.
func (e *Executor) Executed() bool {
	return e.executed
}

func (e *Executor) configStream(r io.Reader, name types.StreamName, tee io.Writer, emitAll bool) *annotee.Stream {
	s := &annotee.Stream{
		Reader:      r,
		Name:        name,
		Tee:         tee,
		Alias:       "stdio",
		Annotate:    true,
		EmitAllLink: emitAll,
	}
	return s
}
