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

package lib

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/kr/pretty"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
)

// CmdReproduce returns an object fo the `reproduce` subcommand.
func CmdReproduce(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "reproduce <>", // TODO
		ShortDesc: "reproduces a task locally",
		LongDesc:  "Fetches a TaskRequest and runs the same commands that were run on the bot.",
		CommandRun: func() subcommands.CommandRun {
			r := &reproduceRun{}
			r.Init(authFlags)
			return r
		},
	}
}

type reproduceRun struct {
	commonFlags
	output string
	work   string
	cache  string
	keep   bool
}

func (c *reproduceRun) Init(authFlags AuthFlags) {
	c.commonFlags.Init(authFlags)

	c.Flags.StringVar(&c.output, "output", "out", "Directory that will have results stored into.")
	c.Flags.StringVar(&c.work, "work", "work", "Directory to map the task input files into.")
	c.Flags.StringVar(&c.cache, "cache", "cache", "Directory that contains the input cache.")
	c.Flags.BoolVar(&c.keep, "keep", false, "Keep the working directory after execution.")
	//c.Flags.BoolVar(&c.leak, "leak", false, "Do not delete the working directory after execution.")
}

func (c *reproduceRun) Parse(_ subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.Reason("must specify exactly one task id.").Err()
	}
	return nil
}

func (c *reproduceRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if err := c.Parse(a, args); err != nil {
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
	outdir, err := prepareDir(c.output)
	workdir, err := prepareDir(c.work)
	// TODO cache dir?

	ctx, cancel := context.WithCancel(c.defaultFlags.MakeLoggingContext(os.Stderr))
	signals.HandleInterrupt(cancel)
	service, err := c.createSwarmingClient(ctx)
	if err != nil {
		return err
	}

	taskID := args[0]
	result, err := service.GetTaskRequest(ctx, taskID)
	properties := result.Properties

	// TODO: remove
	fmt.Printf("%v", args)
	pretty.Println(properties)

	// support relative cwd in task request
	if properties.RelativeCwd != "" {
		workdir = path.Join(workdir, properties.RelativeCwd)
	}

	cmdEnv := environ.New(os.Environ())

	// environment variable in task request
	cmdEnv.Set("SWARMING_BOT_ID", "reproduce")
	cmdEnv.Set("SWARMING_TASK_ID", "reproduce")
	for e := range properties.Env {
		env := properties.Env[e]
		key := env.Key
		if env.Value == "" {
			cmdEnv.Remove(key)
		} else {
			cmdEnv.Set(env.Key, env.Value)
		}
	}

	// support env prefix in task request
	for p := range properties.EnvPrefixes {
		prefix := properties.EnvPrefixes[p]

		paths := make([]string, 0, len(prefix.Value)+1)
		for v := range prefix.Value {
			paths = append(paths, path.Join(workdir, prefix.Value[v]))
		}

		key := prefix.Key
		cur := cmdEnv.Get(key)
		if cur != "" {
			paths = append(paths, cur)
		}
		cmdEnv.Set(key, strings.Join(paths, string(os.PathListSeparator)))
	}

	// TODO(jojwang): raise error if both InputsRef and CasInputRoot are NOT nil?

	// support isolated input in task request
	// TODO(jojwang): double check GetTaskOutputs can be used for InputsRef
	if properties.InputsRef != nil {
		files, err := service.GetTaskOutputsFromIsolate(ctx, workdir, properties.InputsRef)
	}

	// support rbe-cas input in task request
	if properties.CasInputRoot != nil {
		cascli, err := c.authFlags.NewCASClient(ctx, properties.CasInputRoot.CasInstance)
		if err != nil {
			return err
		}
		files, nil := service.GetTaskOutputsFromCAS(ctx, workdir, cascli, properties.CasInputRoot)
	}

	// support cipd package download in task request
	// TODO: find cipd client to use
	ci := properties.CipdInput
	if ci != nil {

	}

	// execute Command in task request
	// TODO: finish
	cmd := exec.Command(properties.Command[0], properties.Command[1:]...)
	cmd.Env = cmdEnv
	cmd.Dir = workdir

	return nil
}

func prepareDir(dir string) (string, error) {
	if err := os.RemoveAll(dir); err != nil {
		return "", errors.Annotate(err, "failed to remove directory: %s", dir).Err()
	}
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return "", errors.Annotate(err, "failed to create directory: %s", dir).Err()
	}
	return dir, nil
}
