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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/kr/pretty"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cmdrunner"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/signals"
)

// CmdReproduce returns an object fo the `reproduce` subcommand.
func CmdReproduce(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "reproduce <task ID>",
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

	c.Flags.StringVar(&c.work, "work", "work", "Directory to map the task input files into.")
	// TODO(crbug.com/1027071): support cache and outtput directories.
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
	ctx, cancel := context.WithCancel(c.defaultFlags.MakeLoggingContext(os.Stderr))
	signals.HandleInterrupt(cancel)
	service, err := c.createSwarmingClient(ctx)
	if err != nil {
		return err
	}

	taskID := args[0]
	taskRequest, err := service.GetTaskRequest(ctx, taskID)
	if err != nil {
		return err
	}
	properties := taskRequest.Properties

	// TODO: remove
	logging.Debugf(ctx, "TaskRequest.Properties: %v", properties)
	pretty.Println(properties)
	logging.Debugf(ctx, "args: %v", args)

	// support relative cwd in task request
	workdir := c.work
	if properties.RelativeCwd != "" {
		workdir = path.Join(workdir, properties.RelativeCwd)
	}
	if err := prepareDir(workdir); err != nil {
		return err
	}

	cmdEnv := environ.New(os.Environ())

	// Set environment variables in task request
	cmdEnv.Set("SWARMING_BOT_ID", "reproduce")
	cmdEnv.Set("SWARMING_TASK_ID", "reproduce")
	for _, env := range properties.Env {
		key := env.Key
		if env.Value == "" {
			cmdEnv.Remove(key)
		} else {
			cmdEnv.Set(env.Key, env.Value)
		}
	}

	// Set env prefix in task request
	for _, prefix := range properties.EnvPrefixes {
		paths := make([]string, 0, len(prefix.Value)+1)
		for _, value := range prefix.Value {
			paths = append(paths, path.Join(workdir, value))
		}

		key := prefix.Key
		cur, ok := cmdEnv.Get(key)
		if ok {
			paths = append(paths, cur)
		}
		cmdEnv.Set(key, strings.Join(paths, string(os.PathListSeparator)))
	}

	if properties.InputsRef != nil && properties.CasInputRoot != nil {
		return errors.Reason("Fetched TaskRequest has files from Isolate and RBE-CAS").Err()
	}

	// support isolated input in task request
	if properties.InputsRef != nil {
		if _, err := service.GetFilesFromIsolate(ctx, workdir, properties.InputsRef); err != nil {
			return err
		}
	}

	// support rbe-cas input in task request
	if properties.CasInputRoot != nil {
		cascli, err := c.authFlags.NewCASClient(ctx, properties.CasInputRoot.CasInstance)
		if err != nil {
			return err
		}
		if _, err := service.GetFilesFromCAS(ctx, workdir, cascli, properties.CasInputRoot); err != nil {
			return err
		}

	}

	// support cipd package download in task request
	ci := properties.CipdInput
	if ci != nil {
		slicesByPath := map[string]ensure.PackageSlice{}
		for _, pkg := range ci.Packages {
			path := pkg.Path
			if path == "." {
				path = ""
			}
			if _, ok := slicesByPath[pkg.Path]; !ok {
				slicesByPath[path] = make(ensure.PackageSlice, 0, len(ci.Packages))
			}
			packageDef := ensure.PackageDef{
				UnresolvedVersion: pkg.Version, PackageTemplate: pkg.PackageName}
			slicesByPath[path] = append(slicesByPath[path], packageDef)
		}
		opts := cipd.ClientOptions{
			Root:       workdir,
			ServiceURL: "https://chrome-infra-packages.appspot.com",
		}
		client, err := cipd.NewClient(opts)
		if err != nil {
			return err
		}
		ef := &ensure.File{
			ServiceURL:       "https://chrome-infra-packages.appspot.com",
			PackagesBySubdir: slicesByPath,
		}
		resolver := cipd.Resolver{Client: client}
		resolved, err := resolver.Resolve(ctx, ef, template.DefaultExpander())
		if err != nil {
			return err
		}
		if _, err = client.EnsurePackages(ctx, resolved.PackagesBySubdir, resolved.ParanoidMode, 1, false); err != nil {
			return err
		}

	}

	// execute Command in task request
	prenewCommand, err := cmdrunner.ProcessCommand(ctx, properties.Command, workdir, "")
	if err != nil {
		return err
	}
	newCommand := make([]string, len(prenewCommand)+3)
	copy(newCommand, prenewCommand)
	copy(newCommand[5:], newCommand[2:])
	newCommand[2] = "-new"
	newCommand[3] = "-realm"
	newCommand[4] = "chromium-m89:ci"
	cmd := exec.Command(newCommand[0], newCommand[1:]...)
	for _, c := range newCommand {
		fmt.Printf("%s \n", c)
	}
	taskEnv := make([]string, len(cmdEnv))
	for _, v := range cmdEnv {
		taskEnv = append(taskEnv, v)
	}
	cmd.Env = taskEnv
	cmd.Dir = workdir
	fmt.Printf("%s \n", taskEnv)
	fmt.Printf("%s \n", workdir)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		fmt.Println(fmt.Sprint(err) + ": " + stderr.String())
	}
	fmt.Println("Result: " + out.String())
	return nil
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
