// Copyright 2020 The LUCI Authors.
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

// Package lib is used to implement the parent package main.
package lib

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/isolate"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/filesystem"
)

// CmdRun returns an object for the `run` subcommand.
func CmdRun() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "run <options> -- <extra args>",

		ShortDesc: "Runs the test executable in an isolated (temporary) directory",
		LongDesc: `All the dependencies are mapped into the temporary directory and the directory is cleaned up after the target exits. Argument processing stops at -- and these arguments are appended to the command line of the target to run.

For example, use: isolate run -isolated foo.isolate -- --gtest_filter=Foo.Bar`,
		CommandRun: func() subcommands.CommandRun {
			r := runRun{}
			r.commonFlags.Init()
			r.isolateFlags.Init(&r.Flags)
			return &r
		},
	}
}

type runRun struct {
	commonFlags
	isolateFlags
}

func (r *runRun) Parse(a subcommands.Application, args []string) error {
	if err := r.commonFlags.Parse(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := r.isolateFlags.Parse(cwd, RequireIsolateFile); err != nil {
		return err
	}

	return nil
}

func (r *runRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := r.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	cl, err := r.defaultFlags.StartTracing()
	if err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	defer cl.Close()
	if err := r.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}

func (r *runRun) main(a subcommands.Application, args []string) error {
	deps, rootDir, isol, err := isolate.ProcessIsolate(&r.ArchiveOptions)
	if err != nil {
		return errors.Annotate(err, "failed to process isolate").Err()
	}

	var cleanupErr error
	if err := (&filesystem.TempDir{
		Dir:    filepath.Dir(rootDir),
		Prefix: time.Now().Format("2006-01-02"),
		CleanupErrFunc: func(tdir string, err error) {
			cleanupErr = errors.Annotate(err, "failed to clean up %s", tdir).Err()
		},
	}).With(func(outDir string) error {
		if err := recreateTree(outDir, rootDir, deps); err != nil {
			return errors.Annotate(err, "failed to recreate tree").Err()
		}
		cmdAndArgs := append([]string{}, isol.Command...)
		cmdAndArgs = append(cmdAndArgs, args...)
		if len(cmdAndArgs) == 0 {
			return errors.Reason("command cannot be empty").Err()
		}
		cwd := filepath.Join(outDir, isol.RelativeCwd)
		// TODO: Ensure cwd exists + normalize cwd
		log.Printf("Running %v, cwd=%s\n", cmdAndArgs, cwd)
		cmd := exec.Command(cmdAndArgs[0], cmdAndArgs[1:]...)
		cmd.Dir = cwd
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return errors.Annotate(err, "failed to run: %v", cmdAndArgs).Err()
		}
		return nil
	}); err != nil {
		return err
	}
	return cleanupErr
}
