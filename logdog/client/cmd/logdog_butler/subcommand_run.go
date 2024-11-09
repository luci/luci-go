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

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/nestedflagset"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/exitcode"

	"go.chromium.org/luci/logdog/client/bootstrapResult"
	"go.chromium.org/luci/logdog/client/butler"
	"go.chromium.org/luci/logdog/client/butler/streamserver"
)

var subcommandRun = &subcommands.Command{
	UsageLine: "run",
	ShortDesc: "Bootstraps an application, passing its output through the Butler.",
	LongDesc:  "Bootstraps an application to stream through the Butler.",
	CommandRun: func() subcommands.CommandRun {
		cmd := &runCommandRun{}

		cmd.Flags.StringVar(&cmd.resultPath, "result-path", "",
			"If supplied, a JSON file describing the bootstrap result will be written here if the bootstrapped process "+
				"is successfully executed.")
		cmd.Flags.StringVar(&cmd.jsonArgsPath, "json-args-path", "",
			"If specified, this is a JSON file containing the full command to run as an "+
				"array of strings.")
		cmd.Flags.StringVar(&cmd.chdir, "chdir", "",
			"If specified, switch to this directory prior to running the command.")
		cmd.Flags.BoolVar(&cmd.attach, "attach", true,
			"If true, attaches the bootstrapped process's STDOUT and STDERR streams.")
		cmd.Flags.BoolVar(&cmd.stdin, "forward-stdin", false,
			"If true, forward STDIN to the bootstrapped process.")

		// "stdout" command-line option.
		stdoutFlag := new(nestedflagset.FlagSet)
		cmd.stdout.addFlags(&stdoutFlag.F)
		cmd.Flags.Var(stdoutFlag, "stdout", "STDOUT stream parameters.")

		// "stderr" command-line option.
		stderrFlag := new(nestedflagset.FlagSet)
		cmd.stderr.addFlags(&stderrFlag.F)
		cmd.Flags.Var(stderrFlag, "stderr", "STDERR stream parameters.")

		return cmd
	},
}

type runCommandRun struct {
	subcommands.CommandRunBase

	// If not empty, write bootstrap result JSON here.
	resultPath string

	// jsonArgsPath, if not empty, is the path to a JSON file containing an array
	// of strings, each of which is a command-line argument to the bootstrapped
	// command.
	jsonArgsPath string

	// chdir, if not empty, is the directory to switch to before running the
	// bootstrapped command.
	chdir string

	// attach, if true, automatically attaches the subprocess's STDOUT and STDERR
	// streams to the Butler.
	attach bool
	stdin  bool
	stdout streamConfig // Stream configuration for STDOUT.
	stderr streamConfig // Stream configuration for STDERR.
}

func (cmd *runCommandRun) Run(app subcommands.Application, args []string, _ subcommands.Env) int {
	a := app.(*application)

	if cmd.jsonArgsPath != "" {
		if len(args) > 0 {
			log.Errorf(a, "Cannot supply both JSON and command-line arguments.")
			return configErrorReturnCode
		}

		err := error(nil)
		args, err = cmd.loadJSONArgs()
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"path":       cmd.jsonArgsPath,
			}.Errorf(a, "Failed to load JSON arguments.")
			return configErrorReturnCode
		}

		log.Fields{
			"args": args,
			"path": cmd.jsonArgsPath,
		}.Debugf(a, "Loaded arguments from JSON file.")
	}

	if len(args) == 0 {
		log.Errorf(a, "A command must be specified.")
		return configErrorReturnCode
	}

	// Resolve
	streamServer, err := streamserver.New(a, "")
	if err != nil {
		log.WithError(err).Errorf(a, "Invalid stream server URI.")
		return configErrorReturnCode
	}

	// Get the actual path to the command
	commandPath, err := exec.LookPath(args[0])
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"command":    args[0],
		}.Errorf(a, "Failed to identify command path.")
		return runtimeErrorReturnCode
	}
	args = args[1:]

	// Verify our current working directory. "cwd" is only used for validation and
	// logging.
	cwd := cmd.chdir
	if cwd != "" {
		fi, err := os.Stat(cwd)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"path":       cwd,
			}.Errorf(a, "Failed to stat `chdir` directory.")
			return runtimeErrorReturnCode
		}

		if !fi.IsDir() {
			log.Fields{
				"path": cwd,
			}.Errorf(a, "Target `chdir` path is not a directory.")
			return runtimeErrorReturnCode
		}
	} else {
		// Just for output.
		cwd, _ = os.Getwd()
	}

	// Get our output.
	of, err := a.getOutputFactory()
	if err != nil {
		log.WithError(err).Errorf(a, "Failed to get output factory instance.")
		return runtimeErrorReturnCode
	}
	output, err := of.configOutput(a)
	if err != nil {
		log.WithError(err).Errorf(a, "Failed to create output instance.")
		return runtimeErrorReturnCode
	}
	defer output.Close()

	// Update our environment for the child process to inherit
	bsEnv := output.URLConstructionEnv()

	// Configure stream server
	streamServerOwned := true
	log.Fields{
		"url": streamServer.Address(),
	}.Infof(a, "Creating stream server.")
	if err := streamServer.Listen(); err != nil {
		log.WithError(err).Errorf(a, "Failed to connect to stream server.")
		return runtimeErrorReturnCode
	}
	log.Infof(a, "listening on %q", streamServer.Address())

	defer func() {
		if streamServerOwned {
			streamServer.Close()
		}
	}()

	bsEnv.StreamServerURI = streamServer.Address()

	// Build our command environment.
	env := environ.System()
	bsEnv.Augment(env)

	// Construct and execute the command.
	procCtx, procCancelFunc := context.WithCancel(a)
	defer procCancelFunc()

	proc := exec.CommandContext(procCtx, commandPath, args...)
	proc.Dir = cmd.chdir
	proc.Env = env.Sorted()
	if cmd.stdin {
		proc.Stdin = os.Stdin
	}

	// Attach STDOUT / STDERR pipes if configured to do so.
	//
	// This will happen in one of two ways:
	// - If no stream name is provided, we will not create a Butler stream for
	//   this output; instead, we will connect them directly to the bootstrapped
	//   process.
	// - If a stream name is provided, we will create a Butler stream for this
	//   output, and tee as necessary.
	//
	// When connecting the stream to the Butler, we track them with a
	// sync.WaitGroup because we have to wait for them to close before reaping the
	// process (see exec.Cmd's StdoutPipe method).
	//
	// In this case, we will hand them to the Butler, which will Close then when
	// it counters io.EOF.
	var (
		stdout, stderr io.ReadCloser
		streamWG       sync.WaitGroup
	)
	if cmd.attach {
		// STDOUT
		if cmd.stdout.Name == "" {
			// Forward directly.
			proc.Stdout = os.Stdout
		} else {
			// Connect to Butler.
			stdout, err = proc.StdoutPipe()
			if err != nil {
				log.WithError(err).Errorf(a, "Failed to get STDOUT pipe.")
				return runtimeErrorReturnCode
			}
			stdout = &callbackReadCloser{stdout, streamWG.Done}
			streamWG.Add(1)
		}

		// STDERR
		if cmd.stderr.Name == "" {
			// Forward directly.
			proc.Stderr = os.Stderr
		} else {
			// Connect to Butler.
			stderr, err = proc.StderrPipe()
			if err != nil {
				log.WithError(err).Errorf(a, "Failed to get STDERR pipe.")
				return runtimeErrorReturnCode
			}
			stderr = &callbackReadCloser{stderr, streamWG.Done}
			streamWG.Add(1)
		}
	}

	if log.IsLogging(a, log.Debug) {
		log.Fields{
			"commandPath": commandPath,
			"cwd":         cwd,
			"args":        args,
		}.Debugf(a, "Executing application.")
		for _, entry := range proc.Env {
			log.Debugf(a, "Environment variable: %s", entry)
		}
	}

	// Attach and run our Butler instance.
	var (
		executed   = false
		returnCode = runtimeErrorReturnCode
	)

	err = a.runWithButler(output, func(b *butler.Butler) error {
		// We want to kill our process early if our Butler run finishes.
		defer procCancelFunc()

		// If we have configured a stream server, add it.
		if streamServer != nil {
			b.AddStreamServer(streamServer)
			streamServerOwned = false
		}

		// Add our pipes as direct streams, if configured.
		if stdout != nil {
			if err := b.AddStream(stdout, cmd.stdout.properties()); err != nil {
				return errors.Annotate(err, "failed to attach STDOUT pipe stream").Err()
			}
		}
		if stderr != nil {
			if err := b.AddStream(stderr, cmd.stderr.properties()); err != nil {
				return errors.Annotate(err, "failed to attach STDERR pipe stream").Err()
			}
		}

		// Execute the command. The bootstrapped application will begin executing
		// in the background.
		if err := proc.Start(); err != nil {
			return errors.Annotate(err, "failed to start bootstrapped process").Err()
		}

		// Wait for the process's streams to finish. We must do this before Wait()
		// on the process itself.
		streamWG.Wait()

		// Reap the process.
		err := proc.Wait()
		if rc, ok := exitcode.Get(err); ok {
			if rc != 0 {
				log.Fields{
					"returnCode": rc,
				}.Errorf(a, "Command completed with non-zero return code.")
			}

			returnCode = rc
			executed = true
		} else {
			log.WithError(err).Errorf(a, "Command failed.")
		}

		// Wait for our Butler to finish.
		b.Activate()
		if err := b.Wait(); err != nil {
			if err == context.Canceled {
				return err
			}
			return errors.Annotate(err, "failed to Wait() for Butler").Err()
		}

		return nil
	})
	if err != nil {
		logAnnotatedErr(a, err, "Error running bootstrapped Butler process:")
	}

	if !executed {
		return runtimeErrorReturnCode
	}

	// Output our bootstrap result.
	br := bootstrapResult.Result{
		ReturnCode: returnCode,
		Command:    append([]string{commandPath}, args...),
	}
	if err := cmd.maybeWriteResult(a, &br); err != nil {
		logAnnotatedErr(a, err, "Failed to write bootstrap result:")
	}

	return returnCode
}

func (cmd *runCommandRun) maybeWriteResult(ctx context.Context, r *bootstrapResult.Result) error {
	if cmd.resultPath == "" {
		return nil
	}

	log.Fields{
		"path": cmd.resultPath,
	}.Debugf(ctx, "Writing bootstrap result.")
	if err := r.WriteJSON(cmd.resultPath); err != nil {
		return errors.Annotate(err, "failed to write JSON file: %s", cmd.resultPath).Err()
	}
	return nil
}

func (cmd *runCommandRun) loadJSONArgs() ([]string, error) {
	fd, err := os.Open(cmd.jsonArgsPath)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	dec := json.NewDecoder(fd)
	args := []string(nil)
	if err := dec.Decode(&args); err != nil {
		return nil, err
	}
	return args, nil
}

// callbackReadCloser invokes a callback method when closed.
type callbackReadCloser struct {
	io.ReadCloser
	callback func()
}

func (c *callbackReadCloser) Close() error {
	defer c.callback()
	return c.ReadCloser.Close()
}
