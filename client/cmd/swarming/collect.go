// Copyright 2017 The LUCI Authors.
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
	"errors"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/context"

	googleapi "google.golang.org/api/googleapi"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
)

type taskOutputOption int64

const (
	taskOutputNone taskOutputOption    = 0
	taskOutputConsole taskOutputOption = 1 << 0
	taskOutputJSON taskOutputOption    = 1 << 1
	taskOutputAll taskOutputOption     = taskOutputConsole | taskOutputJSON
)

func (t *taskOutputOption) String() string {
	if t == nil {
		return "none"
	}
	switch *t {
	case taskOutputJSON:
		return "json"
	case taskOutputConsole:
		return "console"
	case taskOutputAll:
		return "all"
	case taskOutputNone:
		fallthrough
	default:
		return "none"
	}
}

func (t *taskOutputOption) Set(s string) error {
	switch s {
	case "json":
		*t = taskOutputJSON
	case "console":
		*t = taskOutputConsole
	case "all":
		*t = taskOutputAll
	case "":
		fallthrough
	case "none":
		*t = taskOutputNone
	default:
		return fmt.Errorf("invalid task output option: %s", s)
	}
	return nil
}

func (t *taskOutputOption) IncludesJSON() bool {
	return (*t & taskOutputJSON) != 0
}

func (t *taskOutputOption) IncludesConsole() bool {
	return (*t & taskOutputConsole) != 0
}

type taskResult struct {
	TaskID string
	Result *swarming.SwarmingRpcsTaskResult
	Output string
	Error  error
}

func (t *taskResult) Print() {
	if t.Error != nil {
		fmt.Printf("%s: %v\n", t.TaskID, t.Error)
	} else {
		fmt.Printf("%s: exit %d\n", t.TaskID, t.Result.ExitCode)
		stdout := strings.TrimSpace(t.Output)
		if len(stdout) > 0 {
			lines := strings.Split(stdout, "\n")
			for _, line := range lines {
				fmt.Printf("    %s\n", line)
			}
		}
	}
}

func cmdCollect(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "collect <options> (--json file | task_id...)",
		ShortDesc: "Waits on a set of Swarming tasks",
		LongDesc:  "Waits on a set of Swarming tasks.",
		CommandRun: func() subcommands.CommandRun {
			r := &collectRun{}
			r.Init(defaultAuthOpts)
			return r
		},
	}
}

type collectRun struct {
	commonFlags

	timeout         time.Duration
	taskSummaryJSON string
	taskOutput      taskOutputOption
	perf            bool
	jsonInput       string
}

func (c *collectRun) Init(defaultAuthOpts auth.Options) {
	c.commonFlags.Init(defaultAuthOpts)

	c.Flags.DurationVar(&c.timeout, "timeout", 0, "Timeout to wait for result. Set to 0 for no timeout.")
	c.Flags.StringVar(&c.taskSummaryJSON, "task-summary-json", "", "Dump a summary of task results to a file as json.")
	c.Flags.BoolVar(&c.perf, "perf", false, "Includes performance statistics.")
	c.Flags.Var(&c.taskOutput, "task-output-stdout", "Where to output each task's console output (stderr/stdout). (none|json|console|all)")
	c.Flags.StringVar(&c.jsonInput, "json", "", "Load the task IDs from a .json file as saved by `trigger --dump-json`")
}

func (c *collectRun) Parse(args *[]string) error {
	var err error
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}

	// Validate timeout duration.
	if c.timeout < 0 {
		return fmt.Errorf("negative timeout is not allowed")
	}

	// Validate arguments.
	if c.jsonInput != "" {
		data, err := ioutil.ReadFile(c.jsonInput)
		if err != nil {
			return fmt.Errorf("reading json input: %v", err)
		}
		input := jsonDump{}
		if err := json.Unmarshal(data, &input); err != nil {
			return fmt.Errorf("unmarshalling json input: %v", err)
		}
		// Modify args to contain all the task IDs.
		*args = append(*args, input.TaskID)
	}
	for _, arg := range *args {
		if !regexp.MustCompile("^[a-z0-9]+$").MatchString(arg) {
			return errors.New("task ID %q must contain only [a-z0-9]")
		}
	}
	if len(*args) == 0 {
		return errors.New("must specify at least one task id, either directly or through -json")
	}
	return err
}

func (c *collectRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if err := c.Parse(&args); err != nil {
		printError(a, err)
		return 1
	}
	cl, err := c.defaultFlags.StartTracing()
	if err != nil {
		printError(a, err)
		return 1
	}
	if err = cl.Close(); err != nil {
		printError(a, err)
		return 1
	}
	if err := c.main(a, args); err != nil {
		printError(a, err)
		return 1
	}
	return 0
}

func (c *collectRun) fetchTaskResults(ctx context.Context, taskID string, service *swarming.Service) taskResult {
	// Fetch the result details.
	call := service.Task.Result(taskID).IncludePerformanceStats(c.perf)
	result, err := call.Context(ctx).Do()
	if err != nil {
		return taskResult{TaskID: taskID, Error: err}
	}

	// If we got the result details, try to fetch stdout if the
	// user asked for it.
	var output string
	if c.taskOutput != taskOutputNone {
		call := service.Task.Stdout(taskID)
		if taskOutput, err := call.Context(ctx).Do(); err != nil {
			return taskResult{TaskID: taskID, Error: err}
		} else {
			output = taskOutput.Output
		}
	}

	// TODO: Fetch additional files from isolate server.

	return taskResult{
		TaskID: taskID,
		Result: result,
		Output: output,
	}
}

func (c *collectRun) pollForTaskResult(ctx context.Context, taskID string, service *swarming.Service, results chan<- taskResult) {
	startedTime := clock.Now(ctx)
	for {
		result := c.fetchTaskResults(ctx, taskID, service)
		if result.Error != nil {
			if gapiErr, ok := result.Error.(*googleapi.Error); ok {
				// Error code of < 500 is a fatal error.
				if gapiErr.Code < 500 {
					results <- result
					return
				}
			}
		} else {
			// Only stop if the swarming bot is "dead" (i.e. not running).
			state, err := ParseTaskState(result.Result.State)
			if err != nil {
				results <- taskResult{TaskID: taskID, Error: err}
				return
			}
			if !state.Alive() {
				results <- result
				return
			}
		}

		currentTime := clock.Now(ctx)

		// Start with a 1 second delay and for each 30 seconds of waiting
		// add another second until hitting a 15 second ceiling.
		delay := time.Second + (currentTime.Sub(startedTime) / 30)
		if delay >= 15*time.Second {
			delay = 15 * time.Second
		}
		timerResult := <- clock.After(ctx, delay)

		// timerResult should have an error if the context's deadline was exceeded,
		// or if the context was cancelled.
		if timerResult.Err != nil {
			results <- taskResult{TaskID: taskID, Error: timerResult.Err}
			return
		}
	}
}

// summarizeResults generate a marshalled JSON summary of the task results.
func (c *collectRun) summarizeResults(results []taskResult) ([]byte, error) {
	jsonResults := []interface{}{}
	for _, result := range results {
		if result.Error != nil {
			jsonResults = append(jsonResults, result.Error)
		} else {
			jsonResult := map[string]interface{}{"results": *result.Result}
			if c.taskOutput.IncludesJSON() {
				jsonResult["output"] = result.Output
			}
			jsonResults = append(jsonResults, jsonResult)
		}
	}
	return json.MarshalIndent(map[string]interface{}{"tasks": jsonResults}, "", "  ")
}

func (c *collectRun) main(a subcommands.Application, taskIDs []string) error {
	// Set up swarming service.
	client, err := c.createAuthClient()
	if err != nil {
		return err
	}
	s, err := swarming.New(client)
	if err != nil {
		return err
	}
	s.BasePath = c.commonFlags.serverURL + swarmingAPISuffix

	// Prepare context.
	// TODO(mknyszek): Use cancel func to implement graceful exit on SIGINT.
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	// Aggregate results by polling and fetching across multiple goroutines.
	results := make([]taskResult, len(taskIDs))
	aggregator := make(chan taskResult, len(taskIDs))
	for _, id := range taskIDs {
		go c.pollForTaskResult(ctx, id, s, aggregator)
	}
	for i := 0; i < len(taskIDs); i++ {
		results[i] = <-aggregator
	}

	// Summarize results to JSON.
	jsonSummary, err := c.summarizeResults(results)
	if err != nil {
		return err
	}

	// Write any relevant files.
	if len(c.taskSummaryJSON) > 0 {
		if err := ioutil.WriteFile(c.taskSummaryJSON, jsonSummary, 0664); err != nil {
			return err
		}
	}
	if c.taskOutput.IncludesConsole() {
		for _, result := range results {
			result.Print()
		}
	}
	return nil
}
