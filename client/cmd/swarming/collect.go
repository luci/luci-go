// Copyright 2016 The LUCI Authors.
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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
)

type TaskOutputOption int64

const (
	TaskOutputNone    = 0
	TaskOutputConsole = 1 << 0
	TaskOutputJSON    = 1 << 1
	TaskOutputAll     = TaskOutputConsole | TaskOutputJSON
)

func (t *TaskOutputOption) String() string {
	if t == nil {
		return "none"
	}
	switch *t {
	case TaskOutputJSON:
		return "json"
	case TaskOutputConsole:
		return "console"
	case TaskOutputAll:
		return "all"
	case TaskOutputNone:
		fallthrough
	default:
		return "none"
	}
}

func (t *TaskOutputOption) Set(s string) error {
	switch s {
	case "json":
		*t = TaskOutputJSON
	case "console":
		*t = TaskOutputConsole
	case "all":
		*t = TaskOutputAll
	case "":
		fallthrough
	case "none":
		*t = TaskOutputNone
	default:
		return fmt.Errorf("invalid task output option: %s", s)
	}
	return nil
}

func (t TaskOutputOption) IncludesJSON() bool {
	return (t & TaskOutputJSON) != 0
}

func (t TaskOutputOption) IncludesConsole() bool {
	return (t & TaskOutputConsole) != 0
}

type TaskResult struct {
	TaskID string
	Result *swarming.SwarmingRpcsTaskResult
	Output string
	Error  error
}

func NewTaskError(taskID string, err error) TaskResult {
	return TaskResult{
		TaskID: taskID,
		Error:  err,
	}
}

func (t *TaskResult) Print() {
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
	taskOutputDir   string
	taskOutput      TaskOutputOption
	perf            bool
	jsonInput       string
}

func (c *collectRun) Init(defaultAuthOpts auth.Options) {
	c.commonFlags.Init(defaultAuthOpts)

	c.Flags.DurationVar(&c.timeout, "timeout", 0, "Timeout to wait for result. Set to 0 for no timeout. No timeout by default.")
	c.Flags.StringVar(&c.taskSummaryJSON, "task-summary-json", "", "Dump a summary of task results to a file as json. Any output files emitted by the task can be collected by using --task-output-dir")
	c.Flags.StringVar(&c.taskOutputDir, "task-output-dir", "", "Directory to put task results into. When the task finishes, this directory contains per-shard directories with output files produced by shards.")
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
		input := &jsonDump{}
		if err := json.Unmarshal(data, input); err != nil {
			return fmt.Errorf("unmarshalling json input: %v", err)
		}
		if len(input.Tasks) == 0 {
			return errors.New("no tasks to collect in json input")
		}
		// Modify args to contain all the task IDs.
		for _, task := range input.Tasks {
			*args = append(*args, task.TaskID)
		}
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
	defer cl.Close()

	if err := c.main(a, args); err != nil {
		printError(a, err)
		return 1
	}
	return 0
}

func (c *collectRun) fetchTaskResults(taskID string) TaskResult {
	client, err := c.createAuthClient()
	if err != nil {
		return NewTaskError(taskID, err)
	}

	s, err := swarming.New(client)
	if err != nil {
		return NewTaskError(taskID, err)
	}
	s.BasePath = c.commonFlags.serverURL + swarmingAPISuffix

	// Fetch the result details.
	call := s.Task.Result(taskID).IncludePerformanceStats(c.perf)
	result, err := call.Do()
	if err != nil {
		return NewTaskError(taskID, err)
	}

	// If we got the result details, try to fetch stdout if the
	// user asked for it.
	var output string
	if c.taskOutput != TaskOutputNone {
		call := s.Task.Stdout(taskID)
		if taskOutput, err := call.Do(); err != nil {
			return NewTaskError(taskID, err)
		} else {
			output = taskOutput.Output
		}
	}

	// TODO: Fetch additional files from isolate server.

	return TaskResult{
		TaskID: taskID,
		Result: result,
		Output: output,
	}
}

func (c *collectRun) pollForTaskResult(taskID string, results chan<- TaskResult) {
	startedTime := time.Now()
	deadline := startedTime.Add(c.timeout)
	for {
		result := c.fetchTaskResults(taskID)
		if result.Error == nil {
			state, err := ParseTaskState(result.Result.State)
			if err != nil {
				results <- NewTaskError(taskID, err)
				return
			}
			if !state.Alive() {
				results <- result
				return
			}
		}
		currentTime := time.Now()
		if deadline.Before(currentTime) {
			err := fmt.Errorf("task timeout %s exceeded", c.timeout)
			results <- NewTaskError(taskID, err)
			return
		}

		// Start with a 1 second delay and for each 30 seconds of waiting
		// add another second until hitting a 15 second ceiling.
		delay := time.Second + (currentTime.Sub(startedTime) / 30)
		if delay >= 15*time.Second {
			delay = 15 * time.Second
		}
		if delay >= deadline.Sub(currentTime) {
			delay = deadline.Sub(currentTime)
		}
		time.Sleep(delay)
	}
}

// summarizeResults generate a marshalled JSON summary of the task results.
func (c *collectRun) summarizeResults(results []TaskResult) ([]byte, error) {
	jsonShardResults := []interface{}{}
	for _, result := range results {
		if result.Error != nil {
			jsonShardResults = append(jsonShardResults, result.Error)
		} else {
			jsonResult := map[string]interface{}{"results": *result.Result}
			if c.taskOutput.IncludesJSON() {
				jsonResult["output"] = result.Output
			}
			jsonShardResults = append(jsonShardResults, jsonResult)
		}
	}
	return json.MarshalIndent(map[string]interface{}{"shards": jsonShardResults}, "", "  ")
}

func (c *collectRun) main(a subcommands.Application, taskIDs []string) error {
	// Aggregate results by polling and fetching across multiple goroutines.
	results := make([]TaskResult, len(taskIDs))
	aggregator := make(chan TaskResult, len(taskIDs))
	for _, id := range taskIDs {
		go c.pollForTaskResult(id, aggregator)
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
	if len(c.taskOutputDir) > 0 {
		if _, err := os.Stat(c.taskOutputDir); os.IsNotExist(err) {
			os.MkdirAll(c.taskOutputDir, 0775)
		} else if err != nil {
			return err
		}
		summaryPath := filepath.Join(c.taskOutputDir, "summary.json")
		if err := ioutil.WriteFile(summaryPath, jsonSummary, 0664); err != nil {
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
