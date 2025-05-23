// Copyright 2018 The LUCI Authors.
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

package swarmingimpl

import (
	"context"
	"flag"
	"io"
	"net/url"
	"os"
	"sync"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/clipb"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/output"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/swarming/client/swarming"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

// CmdSpawnTasks returns an object for the `spawn-tasks` subcommand.
func CmdSpawnTasks(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "spawn-tasks -S <server> -json-input <path>",
		ShortDesc: "spawns a set of Swarming tasks defined in a JSON file",
		LongDesc:  "Spawns a set of Swarming tasks defined in a JSON file.",
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &spawnTasksImpl{}, base.Features{
				MinArgs:         0,
				MaxArgs:         0,
				MeasureDuration: true,
				OutputJSON: base.OutputJSON{
					Enabled:         true,
					DefaultToStdout: true,
				},
			})
		},
	}
}

type spawnTasksImpl struct {
	jsonInput        string
	cancelExtraTasks bool

	requests []*swarmingv2.NewTaskRequest
}

func (cmd *spawnTasksImpl) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&cmd.jsonInput, "json-input", "", "(required) Read Swarming task requests from this file.")
	// TODO(https://crbug.com/997221): Remove this option.
	fs.BoolVar(&cmd.cancelExtraTasks, "cancel-extra-tasks", false, "Legacy option that does absolutely nothing.")
}

func (cmd *spawnTasksImpl) ParseInputs(ctx context.Context, args []string, env subcommands.Env, extra base.Extra) error {
	if cmd.jsonInput == "" {
		return errors.Reason("input JSON file is required, pass it via -json-input").Err()
	}

	tasksFile, err := os.Open(cmd.jsonInput)
	if err != nil {
		return errors.Annotate(err, "failed to open -json-input tasks file").Err()
	}
	defer tasksFile.Close()

	cmd.requests, err = processTasksStream(ctx, tasksFile, env, extra.ServerURL)
	return err
}

func (cmd *spawnTasksImpl) Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra base.Extra) error {
	results, err := createNewTasks(ctx, svc, cmd.requests)
	if err != nil {
		return err
	}
	return output.Proto(sink, &clipb.SpawnTasksOutput{Tasks: results})
}

// usingDefaultServer returns true if serverURL is the same server as
// swarming.ServerEnvVar.
func usingDefaultServer(env subcommands.Env, serverURL *url.URL) (bool, error) {
	serverEnvVar := env[swarming.ServerEnvVar]
	if !serverEnvVar.Exists {
		return false, nil
	}

	envServerURL, err := lhttp.ParseHostURL(serverEnvVar.Value)
	if err != nil {
		return false, errors.Annotate(err, "parsing %s env var", swarming.ServerEnvVar).Err()
	}

	return envServerURL.String() == serverURL.String(), nil
}

func processTasksStream(ctx context.Context, tasks io.Reader, env subcommands.Env, serverURL *url.URL) ([]*swarmingv2.NewTaskRequest, error) {
	blob, err := io.ReadAll(tasks)
	if err != nil {
		return nil, errors.Annotate(err, "reading tasks file").Err()
	}
	var requests clipb.SpawnTasksInput
	if err := protojson.Unmarshal(blob, &requests); err != nil {
		return nil, errors.Annotate(err, "decoding tasks file").Err()
	}

	// usingSameServer is true when running on Swarming and creating tasks for
	// the current server.
	usingSameServer, err := usingDefaultServer(env, serverURL)
	if err != nil {
		return nil, err
	}

	// When sending a request to a different Swarming server, skip setting
	// ParentTaskId, as the request will fail if ParentTaskId cannot be found on
	// the server.
	warnLogged := false
	currentTaskID := env[swarming.TaskIDEnvVar].Value

	// Populate the tasks with information about the current environment
	// if they're not already set.
	for _, ntr := range requests.Requests {
		if ntr.User == "" {
			ntr.User = env[swarming.UserEnvVar].Value
		}
		if ntr.ParentTaskId == "" && currentTaskID != "" {
			if usingSameServer {
				ntr.ParentTaskId = currentTaskID
			} else if !warnLogged {
				logging.Warningf(ctx, "Request is using %s instead of this task's server, not setting parent task ID in requests", serverURL)
				warnLogged = true
			}
		}
	}
	return requests.Requests, nil
}

func createNewTasks(ctx context.Context, service swarming.Client, requests []*swarmingv2.NewTaskRequest) ([]*swarmingv2.TaskRequestMetadataResponse, error) {
	var mu sync.Mutex
	results := make([]*swarmingv2.TaskRequestMetadataResponse, 0, len(requests))
	err := parallel.WorkPool(8, func(gen chan<- func() error) {
		for _, request := range requests {
			gen <- func() error {
				result, err := service.NewTask(ctx, request)
				if err != nil {
					return err
				}
				mu.Lock()
				defer mu.Unlock()
				results = append(results, result)
				return nil
			}
		}
	})
	return results, err
}
