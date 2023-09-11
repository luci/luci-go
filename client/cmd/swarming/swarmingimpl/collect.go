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

package swarmingimpl

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/maruel/subcommands"
	"golang.org/x/sync/semaphore"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/clipb"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/swarming"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
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

var taskIDRe = regexp.MustCompile("^[a-f0-9]+$")

// weightedSemaphore allows mocking semaphore.Weighted in tests.
type weightedSemaphore interface {
	Acquire(context.Context, int64) error
	TryAcquire(int64) bool
	Release(int64)
}

// taskResult is a consolidation of the results of packaging up swarming
// task results from collect.
type taskResult struct {
	// taskID is the ID of the swarming task for which this results were retrieved.
	taskID string

	// result is the raw result structure returned by a swarming RPC call.
	// result may be nil if err is non-nil.
	result *swarmingv2.TaskResultResponse

	// output is the console output produced by the swarming task.
	// output will only be populated if requested.
	output string

	// outputs is a list of file outputs from a task, downloaded from an isolate server.
	// outputs will only be populated if requested.
	outputs []string

	// err is set if an operational error occurred while doing RPCs to gather the
	// task result, which includes errors received from the server.
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

// CmdCollect returns an object for the `collect` subcommand.
func CmdCollect(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "collect -S <server> -requests-json <path> [<task ID> <task ID> ...])",
		ShortDesc: "waits on a set of Swarming tasks",
		LongDesc:  "Waits on a set of Swarming tasks given either as task IDs or via a file produced by \"trigger\" subcommand.",
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &collectImpl{}, base.Features{
				MinArgs:         0,
				MaxArgs:         base.Unlimited,
				MeasureDuration: true,
				OutputJSON: base.OutputJSON{
					Enabled:             true,
					DeprecatedAliasFlag: "task-summary-json",
					Usage:               "A file to write a summary of task results as json.",
					DefaultToStdout:     false,
				},
			})
		},
	}
}

type collectImpl struct {
	taskIDs           []string
	wait              bool
	timeout           time.Duration
	taskSummaryPython bool
	taskOutput        taskOutputOption
	outputDir         string
	eager             bool
	perf              bool
	jsonInput         string
	casAddr           string
}

func (cmd *collectImpl) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.wait, "wait", true, "Wait task completion.")
	fs.DurationVar(&cmd.timeout, "timeout", 0, "Timeout to wait for result. Set to 0 for no timeout.")

	//TODO(tikuta): Remove this flag once crbug.com/894045 is fixed.
	fs.BoolVar(&cmd.taskSummaryPython, "task-summary-python", false, "Generate python client compatible task summary json.")

	fs.BoolVar(&cmd.eager, "eager", false, "Return after first task completion.")
	fs.BoolVar(&cmd.perf, "perf", false, "Includes performance statistics.")
	fs.Var(&cmd.taskOutput, "task-output-stdout", "Where to output each task's console output (stderr/stdout). (none|json|console|all)")
	fs.StringVar(&cmd.outputDir, "output-dir", "", "Where to download isolated output to.")
	fs.StringVar(&cmd.jsonInput, "requests-json", "", "Load the task IDs from a .json file as saved by \"trigger -json-output\".")
	fs.StringVar(&cmd.casAddr, "cas-addr", casclient.AddrProd, "CAS service address.")
}

func (cmd *collectImpl) ParseInputs(args []string, env subcommands.Env) error {
	if cmd.timeout < 0 {
		return errors.Reason("negative timeout is not allowed").Err()
	}
	if !cmd.wait && cmd.timeout > 0 {
		return errors.Reason("Do not specify -timeout with -wait=false.").Err()
	}

	// Collect all task IDs to wait on.
	cmd.taskIDs = args
	if cmd.jsonInput != "" {
		data, err := os.ReadFile(cmd.jsonInput)
		if err != nil {
			return errors.Annotate(err, "reading json input").Err()
		}
		var tasks clipb.SpawnTasksOutput
		if err := protojson.Unmarshal(data, &tasks); err != nil {
			return errors.Annotate(err, "unmarshalling json input").Err()
		}
		for _, task := range tasks.Tasks {
			cmd.taskIDs = append(cmd.taskIDs, task.TaskId)
		}
	}

	// Verify they all look like Swarming task IDs.
	//
	// TODO(vadimsh): Extract and reuse in other subcommands.
	for _, taskID := range cmd.taskIDs {
		if !taskIDRe.MatchString(taskID) {
			return errors.Reason("task ID %q must be hex ([a-f0-9])", taskID).Err()
		}
	}
	if len(cmd.taskIDs) == 0 {
		return errors.Reason("must specify at least one task id, either directly or through -requests-json").Err()
	}

	return nil
}

func (cmd *collectImpl) fetchTaskResults(ctx context.Context, taskID string, service swarming.Swarming, downloadSem weightedSemaphore, auth base.AuthFlags) taskResult {
	defer logging.Debugf(ctx, "Finished fetching task result: %s", taskID)

	errorResult := func(stage string, err error) taskResult {
		return taskResult{taskID: taskID, err: errors.Annotate(err, "when fetching %s", stage).Err()}
	}

	var output string    // task stdout as is
	var outputs []string // names of output files downloaded from CAS

	// Fetch the result details.
	logging.Debugf(ctx, "Fetching task result: %s", taskID)
	result, err := service.TaskResult(ctx, taskID, cmd.perf)
	if err != nil {
		return errorResult("task result", err)
	}

	// Signal that we want to start downloading outputs. We'll only proceed
	// to download them if another task has not already finished and triggered
	// an eager return.
	//
	// TODO(vadimsh): Semaphore is confusing and unnecessary here, this can be
	// done via context cancellation.
	if !downloadSem.TryAcquire(1) {
		return errorResult("task output", errors.New("canceled by first task"))
	}
	defer downloadSem.Release(1)

	// TODO(vadimsh): Fetch output and output files in parallel.

	// If we got the result details, try to fetch stdout if the user asked for it.
	//
	// TODO(vadimsh): If cmd.wait is true, fetch this only if task has completed.
	// If the task is still running, we'll make another call later anyway.
	if cmd.taskOutput != taskOutputNone {
		logging.Debugf(ctx, "Fetching task output: %s", taskID)
		taskOutput, err := service.TaskOutput(ctx, taskID)
		if err != nil {
			return errorResult("task output", errors.New("canceled by first task"))
		}
		output = string(taskOutput.Output)
	}

	// Download the result files if available and if we have a place to put it.
	//
	// TODO(vadimsh): If cmd.wait is true, fetch this only if task has completed.
	// If the task is still running, we'll make another call later anyway.
	if cmd.outputDir != "" {
		logging.Debugf(ctx, "Fetching task outputs: %s", taskID)
		outdir, err := prepareOutputDir(cmd.outputDir, taskID)
		if err != nil {
			return errorResult("output files", err)
		}
		if result.CasOutputRoot != nil {
			// TODO(vadimsh): Reuse CAS client instead of creating it all the time.
			cascli, err := auth.NewRBEClient(ctx, cmd.casAddr, result.CasOutputRoot.CasInstance)
			if err != nil {
				return errorResult("output files", err)
			}
			outputs, err = service.FilesFromCAS(ctx, outdir, cascli, &swarmingv2.CASReference{
				CasInstance: result.CasOutputRoot.CasInstance,
				Digest: &swarmingv2.Digest{
					Hash:      result.CasOutputRoot.Digest.Hash,
					SizeBytes: result.CasOutputRoot.Digest.SizeBytes,
				},
			})
			if err != nil {
				return errorResult("output files", err)
			}
		}
	}

	return taskResult{
		taskID:  taskID,
		result:  result,
		output:  output,
		outputs: outputs,
	}
}

func prepareOutputDir(outputDir, taskID string) (string, error) {
	// Create a task-id-based subdirectory to house the outputs.
	dir := filepath.Join(filepath.Clean(outputDir), taskID)
	// The call can theoretically be retried. In this case the directory will
	// already exist and may contain partial results. Take no chance and restart
	// from scratch.
	if err := os.RemoveAll(dir); err != nil {
		return "", errors.Annotate(err, "failed to remove directory: %s", dir).Err()
	}
	if err := os.MkdirAll(dir, 0777); err != nil {
		return "", errors.Annotate(err, "failed to create directory: %s", dir).Err()
	}
	return dir, nil
}

func (cmd *collectImpl) pollForTaskResult(ctx context.Context, taskID string, service swarming.Swarming, downloadSem weightedSemaphore, auth base.AuthFlags) taskResult {
	startedTime := clock.Now(ctx)
	for {
		result := cmd.fetchTaskResults(ctx, taskID, service, downloadSem, auth)
		if result.err != nil {
			// If we received an error from fetchTaskResults, it either hit a fatal
			// failure, or it hit too many transient failures.
			return result
		}

		// Stop if the task is no longer pending or running.
		if !TaskIsAlive(result.result.State) {
			logging.Debugf(ctx, "Task completed: %s", taskID)
			return result
		}
		if !cmd.wait {
			logging.Debugf(ctx, "Task fetched: %s", taskID)
			return result
		}

		currentTime := clock.Now(ctx)

		// Start with a 1 second delay and for each 30 seconds of waiting
		// add another second until hitting a 15 second ceiling.
		delay := time.Second + (currentTime.Sub(startedTime) / 30)
		if delay >= 15*time.Second {
			delay = 15 * time.Second
		}

		logging.Debugf(ctx, "Waiting %s for task: %s", delay.Round(time.Millisecond), taskID)
		timerResult := <-clock.After(ctx, delay)

		// timerResult should have an error if the context's deadline was exceeded,
		// or if the context was cancelled.
		if timerResult.Err != nil {
			if result.err == nil {
				result.err = timerResult.Err
			}
			return result
		}
	}
}

// summarizeResultsPython generates a summary compatible with python's client.
func summarizeResultsPython(results []taskResult) (any, error) {
	shards := make([]map[string]any, len(results))

	for i, result := range results {
		// Convert TaskResultResponse proto to a free-form map[string]any.
		buf, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(result.result)
		if err != nil {
			return nil, err
		}
		var jsonResult map[string]any
		if err := json.Unmarshal(buf, &jsonResult); err != nil {
			return nil, err
		}
		// Inject `output` as an extra field not present in the original proto.
		if len(jsonResult) > 0 {
			jsonResult["output"] = result.output
		} else {
			jsonResult = nil
		}
		shards[i] = jsonResult
	}

	return base.LegacyJSON(map[string]any{"shards": shards}), nil
}

// summarizeResults generates a summary of the task results.
func (cmd *collectImpl) summarizeResults(results []taskResult) (any, error) {
	summary := map[string]*clipb.ResultSummaryEntry{}
	for _, result := range results {
		entry := &clipb.ResultSummaryEntry{}
		if result.err != nil {
			entry.Error = result.err.Error()
		}
		if result.result != nil {
			if cmd.taskOutput.includesJSON() {
				entry.Output = result.output
			}
			entry.Outputs = result.outputs
			entry.Results = result.result
		}
		summary[result.taskID] = entry
	}
	return summary, nil
}

func (cmd *collectImpl) pollForTasks(ctx context.Context, taskIDs []string, service swarming.Swarming, downloadSem weightedSemaphore, auth base.AuthFlags) []taskResult {
	if len(taskIDs) == 0 {
		return nil
	}

	var cancel context.CancelFunc
	if cmd.timeout > 0 {
		ctx, cancel = clock.WithTimeout(ctx, cmd.timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	// Aggregate results by polling and fetching across multiple goroutines.
	results := make([]taskResult, len(taskIDs))
	var wg sync.WaitGroup
	wg.Add(len(taskIDs))
	taskFinished := make(chan int, len(taskIDs))
	for i := range taskIDs {
		go func(i int) {
			defer func() {
				taskFinished <- i
				wg.Done()
			}()
			results[i] = cmd.pollForTaskResult(ctx, taskIDs[i], service, downloadSem, auth)
		}(i)
	}

	if cmd.eager {
		go func() {
			<-taskFinished
			// After the first task finishes, block any new tasks from starting
			// to download outputs, but let any in-progress downloads complete.
			downloadSem.Acquire(ctx, int64(len(taskIDs)))
			cancel()
		}()
	}

	wg.Wait()

	return results
}

func (cmd *collectImpl) Execute(ctx context.Context, svc swarming.Swarming, extra base.Extra) (any, error) {
	downloadSem := semaphore.NewWeighted(int64(len(cmd.taskIDs)))
	results := cmd.pollForTasks(ctx, cmd.taskIDs, svc, downloadSem, extra.AuthFlags)

	for _, result := range results {
		if cmd.taskOutput.includesConsole() || result.err != nil {
			result.Print(os.Stdout)
		}
	}

	// Don't bother assembling the summary if we aren't going to store it.
	if extra.OutputJSON == "" {
		return nil, nil
	}

	// TODO(crbug.com/894045): Python-compatible summary is actually the most
	// commonly used now (used by recipes).
	if cmd.taskSummaryPython {
		return summarizeResultsPython(results)
	}
	return cmd.summarizeResults(results)
}
