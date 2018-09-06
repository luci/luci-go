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

package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/maruel/subcommands"

	"golang.org/x/net/context"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
)

func cmdSpawnTasks(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "spawn-tasks <options>",
		ShortDesc: "Spawns a set of Swarming tasks",
		LongDesc:  "Spawns a set of Swarming tasks given a JSON file.",
		CommandRun: func() subcommands.CommandRun {
			r := &spawnTasksRun{}
			r.Init(defaultAuthOpts)
			return r
		},
	}
}

type spawnTasksRun struct {
	commonFlags
	jsonInput  string
	jsonOutput string
}

func (c *spawnTasksRun) Init(defaultAuthOpts auth.Options) {
	c.commonFlags.Init(defaultAuthOpts)
	c.Flags.StringVar(&c.jsonInput, "json-input", "", "(required) Read Swarming task requests from this file.")
	c.Flags.StringVar(&c.jsonOutput, "json-output", "", "Write details about the triggered task(s) to this file as json.")
}

func (c *spawnTasksRun) Parse(args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if c.jsonInput == "" {
		return errors.Reason("input JSON file is required").Err()
	}
	return nil
}

func (c *spawnTasksRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if err := c.Parse(args); err != nil {
		printError(a, err)
		return 1
	}
	cl, err := c.defaultFlags.StartTracing()
	if err != nil {
		printError(a, err)
		return 1
	}
	defer cl.Close()

	if err := c.main(a, args, env); err != nil {
		printError(a, err)
		return 1
	}
	return 0
}

func (c *spawnTasksRun) main(a subcommands.Application, args []string, env subcommands.Env) error {
	start := time.Now()

	tasksFile, err := os.Open(c.jsonInput)
	if err != nil {
		return errors.Annotate(err, "failed to open tasks file").Err()
	}
	defer tasksFile.Close()
	requests, err := processTasksStream(tasksFile)
	if err != nil {
		return err
	}
	service, err := c.createSwarmingClient()
	if err != nil {
		return err
	}
	results, merr := createNewTasks(context.Background(), service, requests)

	var output io.Writer
	if c.jsonOutput != "" {
		file, err := os.Create(c.jsonOutput)
		if err != nil {
			return err
		}
		defer file.Close()
		output = file
	} else {
		output = os.Stdout
	}

	data := triggerResults{Tasks: results}
	b, err := json.MarshalIndent(&data, "", "  ")
	if err != nil {
		return errors.Annotate(err, "marshalling trigger result").Err()
	}

	if _, err = output.Write(b); err != nil {
		return errors.Annotate(err, "writing json output").Err()
	}

	log.Printf("Duration: %s\n", units.Round(time.Since(start), time.Millisecond))
	return merr
}

type tasksInput struct {
	Requests []*swarming.SwarmingRpcsNewTaskRequest `json:"requests"`
}

func processTasksStream(tasks io.Reader) ([]*swarming.SwarmingRpcsNewTaskRequest, error) {
	dec := json.NewDecoder(tasks)
	dec.DisallowUnknownFields()

	requests := tasksInput{}
	if err := dec.Decode(&requests); err != nil {
		return nil, errors.Annotate(err, "decoding tasks file").Err()
	}

	// Populate the tasks with information about the current envirornment
	// if they're not already set.
	currentUser := os.Getenv("USER")
	parentTaskID := os.Getenv("SWARMING_TASK_ID")
	for _, request := range requests.Requests {
		if request.User == "" {
			request.User = currentUser
		}
		if request.ParentTaskId == "" {
			request.ParentTaskId = parentTaskID
		}
	}

	return requests.Requests, nil
}

func createNewTasks(c context.Context, service swarmingService, requests []*swarming.SwarmingRpcsNewTaskRequest) ([]*swarming.SwarmingRpcsTaskRequestMetadata, error) {
	var mu sync.Mutex
	results := make([]*swarming.SwarmingRpcsTaskRequestMetadata, 0, len(requests))
	err := parallel.WorkPool(8, func(gen chan<- func() error) {
		for _, request := range requests {
			request := request
			gen <- func() error {
				result, err := service.NewTask(c, request)
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
