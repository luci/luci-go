// Copyright 2021 The LUCI Authors.
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

package lib

import (
	"context"
	"os"
	"os/exec"
	"path"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/signals"
)

// CmdReproduce returns an object fo the `reproduce` subcommand.
func CmdReproduce(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "reproduce -S <server> <task ID> ",
		ShortDesc: "reproduces a task locally",
		LongDesc:  "Fetches a TaskRequest and runs the same commands that were run on the bot.",
		CommandRun: func() subcommands.CommandRun {
			r := &reproduceRun{}
			r.init(authFlags)
			return r
		},
	}
}

type reproduceRun struct {
	commonFlags
	work string
}

func (c *reproduceRun) init(authFlags AuthFlags) {
	c.commonFlags.Init(authFlags)

	c.Flags.StringVar(&c.work, "work", "work", "Directory to map the task input files into and execute the task.")
	// TODO(crbug.com/1188473): support cache and output directories.
}

func (c *reproduceRun) parse(args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 1 {
		return errors.Reason("must specify exactly one task id.").Err()
	}
	return nil
}

func (c *reproduceRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if err := c.parse(args); err != nil {
		printError(a, err)
		return 1
	}
	if err := c.main(a, args, env); err != nil {
		printError(a, err)
		return 1
	}
	return 0
}

func (c *reproduceRun) main(a subcommands.Application, args []string, env subcommands.Env) error {
	ctx, cancel := context.WithCancel(c.defaultFlags.MakeLoggingContext(os.Stderr))
	signals.HandleInterrupt(cancel)

	service, err := c.createSwarmingClient(ctx)
	if err != nil {
		return err
	}

	// TODO(crbug.com/1188473): Create a pass an rbeclient.Client
	cmd, err := c.createTaskRequestCommand(ctx, args[0], service)
	if err != nil {
		return errors.Annotate(err, "failed to create command from task request").Err()
	}

	return c.executeTaskRequestCommand(cmd)
}

func (c *reproduceRun) executeTaskRequestCommand(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return errors.Annotate(err, "failed to start command: %v", cmd).Err()
	}
	if err := cmd.Wait(); err != nil {
		return errors.Annotate(err, "failed to complete command: %v", cmd).Err()
	}
	return nil
}

func (c *reproduceRun) createTaskRequestCommand(ctx context.Context, taskID string, service swarmingService) (*exec.Cmd, error) {
	tr, err := service.GetTaskRequest(ctx, taskID)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get task request: %s", taskID).Err()
	}
	// In practice, later slices are less likely to assume that there is a named cache
	// that is not available locally.
	properties := tr.TaskSlices[len(tr.TaskSlices)-1].Properties

	workdir := c.work
	if properties.RelativeCwd != "" {
		workdir = path.Join(workdir, properties.RelativeCwd)
	}
	if err := prepareDir(workdir); err != nil {
		return nil, err
	}

	// Set environment variables.
	cmdEnvMap := environ.FromCtx(ctx)
	for _, env := range properties.Env {
		if env.Value == "" {
			cmdEnvMap.Remove(env.Key)
		} else {
			cmdEnvMap.Set(env.Key, env.Value)
		}
	}

	// TODO(crbug.com/1188473): Set env prefix in task request
	// TODO(crbug.com/1188473): Support isolated input in task request.
	// TODO(crbug.com/1188473): Support RBE-CAS input in task request.
	// TODO(crbug.com/1188473): Support CIPD package download in task request

	cmd := exec.CommandContext(ctx, properties.Command[0], properties.Command[1:]...)
	cmd.Env = cmdEnvMap.Sorted()
	cmd.Dir = workdir
	return cmd, nil
}

func prepareDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return errors.Annotate(err, "failed to remove directory: %s", dir).Err()
	}
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return errors.Annotate(err, "failed to create directory: %s", dir).Err()
	}
	return nil
}
