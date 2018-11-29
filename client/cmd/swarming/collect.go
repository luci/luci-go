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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/maruel/subcommands"
	"golang.org/x/net/http2"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

type taskOutputOption int64

const (
	taskOutputNone    taskOutputOption = 0
	taskOutputConsole taskOutputOption = 1 << 0
	taskOutputJSON    taskOutputOption = 1 << 1
	taskOutputAll     taskOutputOption = taskOutputConsole | taskOutputJSON
)

func (t *taskOutputOption) String() string {
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
	case "", "none":
		*t = taskOutputNone
	default:
		return errors.Reason("invalid task output option: %s", s).Err()
	}
	return nil
}

func (t *taskOutputOption) includesJSON() bool {
	return (*t & taskOutputJSON) != 0
}

func (t *taskOutputOption) includesConsole() bool {
	return (*t & taskOutputConsole) != 0
}

// taskResult is a consolidation of the results of packaging up swarming
// task results from collect.
type taskResult struct {
	// taskID is the ID of the swarming task for which this results were retrieved.
	taskID string

	// result is the raw result structure returned by a swarming RPC call.
	// result may be nil if err is non-nil.
	result *swarming.SwarmingRpcsTaskResult

	// output is the console output produced by the swarming task.
	// output will only be populated if requested.
	output string

	// outputs is a list of file outputs from a task, downloaded from an isolate server.
	// outputs will only be populated if requested.
	outputs []string

	// err is set if an operational error occurred while doing RPCs to gather the
	// task result, which includes errors recieved from the server.
	err error
}

func (t *taskResult) Print(w io.Writer) {
	if t.err != nil {
		fmt.Fprintf(w, "%s: %v\n", t.taskID, t.err)
	} else {
		fmt.Fprintf(w, "%s: exit %d\n", t.taskID, t.result.ExitCode)
		if t.output != "" {
			fmt.Fprintln(w, t.output)
		}
	}
}

func cmdCollect(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "collect <options> (-requests-json file | task_id...)",
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

	timeout           time.Duration
	taskSummaryJSON   string
	taskSummaryPython bool
	taskOutput        taskOutputOption
	outputDir         string
	perf              bool
	jsonInput         string
}

func (c *collectRun) Init(defaultAuthOpts auth.Options) {
	c.commonFlags.Init(defaultAuthOpts)

	c.Flags.DurationVar(&c.timeout, "timeout", 0, "Timeout to wait for result. Set to 0 for no timeout.")
	c.Flags.StringVar(&c.taskSummaryJSON, "task-summary-json", "", "Dump a summary of task results to a file as json.")

	//TODO(tikuta): Remove this flag once crbug.com/894045 is fixed.
	c.Flags.BoolVar(&c.taskSummaryPython, "task-summary-python", false, "Generate python client compatible task summary json.")

	c.Flags.BoolVar(&c.perf, "perf", false, "Includes performance statistics.")
	c.Flags.Var(&c.taskOutput, "task-output-stdout", "Where to output each task's console output (stderr/stdout). (none|json|console|all)")
	c.Flags.StringVar(&c.outputDir, "output-dir", "", "Where to download isolated output to.")
	c.Flags.StringVar(&c.jsonInput, "requests-json", "", "Load the task IDs from a .json file as saved by \"trigger -dump-json\"")
}

func (c *collectRun) Parse(args *[]string) error {
	var err error
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}

	// Validate timeout duration.
	if c.timeout < 0 {
		return errors.Reason("negative timeout is not allowed").Err()
	}

	// Validate arguments.
	if c.jsonInput != "" {
		data, err := ioutil.ReadFile(c.jsonInput)
		if err != nil {
			return errors.Annotate(err, "reading json input").Err()
		}
		input := triggerResults{}
		if err := json.Unmarshal(data, &input); err != nil {
			return errors.Annotate(err, "unmarshalling json input").Err()
		}
		// Modify args to contain all the task IDs.
		for _, task := range input.Tasks {
			*args = append(*args, task.TaskId)
		}
	}
	for _, arg := range *args {
		if !regexp.MustCompile("^[a-z0-9]+$").MatchString(arg) {
			return errors.Reason("task ID %q must contain only [a-z0-9]", arg).Err()
		}
	}
	if len(*args) == 0 {
		return errors.Reason("must specify at least one task id, either directly or through -json").Err()
	}
	return err
}

// concurentHTTP2 implements http.RoundTripper for highly concurrent http2
// requests using "https". When we need to do highly parallel http2 request,
// http2 request may be blocked by maxConcurrentStreams per http2.Transport. To
// bypass such limitation, conccurentHTTP2 can use several number of
// http2.Transport.
// TODO(tikuta): Remove this when crbug.com/825418 is fixed.
type concurentHTTP2 struct {
	transports []*http2.Transport
}

func (c concurentHTTP2) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "https" {
		// This is hack to prevent using http2 for localhost.
		return c.transports[rand.Intn(len(c.transports))].RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)

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

	if c.worker >= 10 {
		// TODO(tikuta): Tune this parameter (c.worker/10) if necessary.
		transports := make([]*http2.Transport, c.worker/10)
		for i := range transports {
			transports[i] = &http2.Transport{}
		}
		c.parsedAuthOpts.Transport = &concurentHTTP2{
			transports: transports,
		}
	}

	if err := c.main(a, args); err != nil {
		printError(a, err)
		return 1
	}
	return 0
}

func (c *collectRun) fetchTaskResults(ctx context.Context, taskID string, service swarmingService) taskResult {
	var result *swarming.SwarmingRpcsTaskResult
	var output string
	var outputs []string
	err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
		var err error

		// Fetch the result details.
		result, err = service.GetTaskResult(ctx, taskID, c.perf)
		if err != nil {
			return tagTransientGoogleAPIError(err)
		}

		// TODO(mknyszek): Fetch output and outputs in parallel.

		// If we got the result details, try to fetch stdout if the
		// user asked for it.
		if c.taskOutput != taskOutputNone {
			taskOutput, err := service.GetTaskOutput(ctx, taskID)
			if err != nil {
				return tagTransientGoogleAPIError(err)
			}
			output = taskOutput.Output
		}

		// Download the result isolated if available and if we have a place to put it.
		if c.outputDir != "" && result.OutputsRef != nil {
			outputs, err = service.GetTaskOutputs(ctx, taskID, c.outputDir, result.OutputsRef)
			if err != nil {
				return tagTransientGoogleAPIError(err)
			}
		}
		return nil
	}, func(err error, d time.Duration) {
		logging.WithError(err).Warningf(ctx, "Transient error while making request, retrying in %s...", d)
	})
	if err != nil {
		return taskResult{taskID: taskID, err: err}
	}

	return taskResult{
		taskID:  taskID,
		result:  result,
		output:  output,
		outputs: outputs,
	}
}

func (c *collectRun) pollForTaskResult(ctx context.Context, taskID string, service swarmingService, results chan<- taskResult) {
	var result taskResult
	startedTime := clock.Now(ctx)
	for {
		result = c.fetchTaskResults(ctx, taskID, service)
		if result.err != nil {
			// If we recieved an error from fetchTaskResults, it either hit a fatal
			// failure, or it hit too many transient failures.
			results <- result
			return
		}

		// Only stop if the swarming bot is "dead" (i.e. not running).
		state, err := parseTaskState(result.result.State)
		if err != nil {
			results <- taskResult{taskID: taskID, err: err}
			return
		}
		if !state.Alive() {
			results <- result
			return
		}

		currentTime := clock.Now(ctx)

		// Start with a 1 second delay and for each 30 seconds of waiting
		// add another second until hitting a 15 second ceiling.
		delay := time.Second + (currentTime.Sub(startedTime) / 30)
		if delay >= 15*time.Second {
			delay = 15 * time.Second
		}
		timerResult := <-clock.After(ctx, delay)

		// timerResult should have an error if the context's deadline was exceeded,
		// or if the context was cancelled.
		if timerResult.Err != nil {
			err := timerResult.Err
			if result.err != nil {
				result.err = errors.Annotate(result.err, "%v", timerResult.Err).Err()
			} else {
				result.err = err
			}
			results <- result
			return
		}
	}
}

// summarizeResultsPython generates summary JSON file compatible with python's
// swarming client.
func summarizeResultsPython(results []taskResult) ([]byte, error) {
	var shards []*swarming.SwarmingRpcsTaskResult

	for _, result := range results {
		shards = append(shards, result.result)
	}

	return json.MarshalIndent(map[string][]*swarming.SwarmingRpcsTaskResult{
		"shards": shards,
	}, "", "  ")
}

// summarizeResults generate a marshalled JSON summary of the task results.
func (c *collectRun) summarizeResults(results []taskResult) ([]byte, error) {
	if c.taskSummaryPython {
		return summarizeResultsPython(results)
	}

	jsonResults := map[string]interface{}{}
	for _, result := range results {
		jsonResult := map[string]interface{}{}
		if result.err != nil {
			jsonResult["error"] = result.err.Error()
		}
		if result.result != nil {
			jsonResult["results"] = *result.result
			if c.taskOutput.includesJSON() {
				jsonResult["output"] = result.output
			}
			jsonResult["outputs"] = result.outputs
		}
		jsonResults[result.taskID] = jsonResult
	}
	return json.MarshalIndent(jsonResults, "", "  ")
}

func (c *collectRun) main(a subcommands.Application, taskIDs []string) error {
	// Set up swarming service.
	service, err := c.createSwarmingClient()
	if err != nil {
		return err
	}

	// Prepare context.
	ctx := context.Background()
	if c.timeout > 0 {
		// TODO(mknyszek): Use cancel func to implement graceful exit on SIGINT.
		var cancel func()
		ctx, cancel = clock.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	// Aggregate results by polling and fetching across multiple goroutines.
	results := make([]taskResult, len(taskIDs))
	aggregator := make(chan taskResult, len(taskIDs))
	for _, id := range taskIDs {
		go c.pollForTaskResult(ctx, id, service, aggregator)
	}
	for i := 0; i < len(taskIDs); i++ {
		results[i] = <-aggregator
	}

	// Summarize and write summary json if applicable.
	if c.taskSummaryJSON != "" {
		jsonSummary, err := c.summarizeResults(results)
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(c.taskSummaryJSON, jsonSummary, 0664); err != nil {
			return err
		}
	}
	for _, result := range results {
		if c.taskOutput.includesConsole() || result.err != nil {
			result.Print(os.Stdout)
		}
	}
	return nil
}
