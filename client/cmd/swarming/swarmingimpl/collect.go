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
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/clipb"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/output"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/swarming/client/swarming"
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

// SummaryLine is a short summary of task state for logs.
func (t *taskResult) SummaryLine() string {
	if t.err != nil {
		return fmt.Sprintf("%s: %s", t.taskID, t.err)
	}
	if t.result.State == swarmingv2.TaskState_COMPLETED {
		return fmt.Sprintf("%s: COMPLETED, exit code %d", t.taskID, t.result.ExitCode)
	}
	return fmt.Sprintf("%s: %s", t.taskID, t.result.State)
}

// CmdCollect returns an object for the `collect` subcommand.
func CmdCollect(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "collect -S <server> -requests-json <path> [<task ID> <task ID> ...])",
		ShortDesc: "waits on a set of Swarming tasks",
		LongDesc: `Waits on a set of Swarming tasks given either as task IDs or via a file produced by \"trigger\" subcommand.

Behavior depends on combination of -eager and -wait flags:
  * -wait is set and -eager is unset (default): wait for all tasks to complete
    and report their results.
  * -wait is set and -eager is set: wait for at least one task to complete and
    then report the current state of all tasks. It will be a mix of pending
    and completed tasks (with at least one, but perhaps more, task completed).
  * -wait is unset (regardless if -eager is set or not): report the current
    state of all tasks, don't wait. It will be a mix of pending and completed
    tasks with no other guarantees.

The JSON output will always have entries for all requested tasks. Each entry
contains the last known state of the task (in "results" field) if it was fetched
at least once and, possibly, an error message (in "error" field) if there was
an error fetching the state.

Note that "error" field reports only local errors. If a task itself failed
remotely, but this outcome was successfully fetched, then "error" field will be
unset, and the task's failure will be communicated via "results" object (in
particular its "state" field).

If -wait is set, will wait for at most -timeout duration or until SIGTERM. Upon
hitting the timeout the JSON entries of all still pending or running tasks will
contain literal "rpc_timeout" value in their "error" fields. Similarly, if
waiting was aborted by SIGTERM, the "error" field will contain "rpc_canceled"
value.

Flag -task-output-stdout controls where to dump the console log of completed
tasks. Its possible values:
  * "none" (default): don't fetch the console log at all.
  * "console": dump the log only to stdout.
  * "json": dump the log only into the JSON output (in "result.output" field).
  * "all": dump the log to stdout and also into the JSON output.

Flag -output-dir controls where to store isolated outputs of completed tasks.
If it is unset (default), isolated outputs will not be fetched. Otherwise
isolated outputs of a completed task with ID <task-ID> will be downloaded to
<output-dir>/<task-ID> directory. If such directory already exists, it will be
cleared first. If a task has no isolated outputs or it has not completed yet,
its output directory will be empty. The JSON output will contain a list of
downloaded files (relative to the task output directory) in "result.outputs"
field.
`,
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &collectImpl{}, base.Features{
				MinArgs:         0,
				MaxArgs:         base.Unlimited,
				MeasureDuration: true,
				UsesCAS:         true,
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
}

func (cmd *collectImpl) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.wait, "wait", true, "If set, wait for tasks to complete. Otherwise just poll their current state.")
	fs.DurationVar(&cmd.timeout, "timeout", 0, "Timeout to wait for tasks to complete when -wait is set. Set to 0 for no timeout.")
	fs.BoolVar(&cmd.eager, "eager", false, "If set, stop waiting whenever any task finishes, do not wait for all of them to finish.")

	//TODO(tikuta): Remove this flag once crbug.com/894045 is fixed.
	fs.BoolVar(&cmd.taskSummaryPython, "task-summary-python", false, "Generate python client compatible task summary json.")

	fs.BoolVar(&cmd.perf, "perf", false, "Include performance statistics.")
	fs.Var(&cmd.taskOutput, "task-output-stdout", "Where to output each task's console output (combined stderr and stdout). (none|json|console|all)")
	fs.StringVar(&cmd.outputDir, "output-dir", "", "Where to download isolated output to.")

	fs.StringVar(&cmd.jsonInput, "requests-json", "", "Load the task IDs from a .json file as saved by \"trigger -json-output\".")
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
	if len(cmd.taskIDs) == 0 {
		return errors.Reason("must specify at least one task id, either directly or through -requests-json").Err()
	}

	// Verify they all look like Swarming task IDs.
	//
	// TODO(vadimsh): Extract and reuse in other subcommands.
	for _, taskID := range cmd.taskIDs {
		if !taskIDRe.MatchString(taskID) {
			return errors.Reason("task ID %q must be hex ([a-f0-9])", taskID).Err()
		}
	}

	// Verify there are no duplicates. This may break some map look ups.
	seen := stringset.New(len(cmd.taskIDs))
	for _, taskID := range cmd.taskIDs {
		if !seen.Add(taskID) {
			return errors.Reason("task ID %s is given more than once", taskID).Err()
		}
	}

	return nil
}

// fetchTaskResults updates `res` in-place with outputs of the task.
func (cmd *collectImpl) fetchTaskResults(ctx context.Context, svc swarming.Client, res *taskResult) {
	// Prepare the output directory, even if the task failed or is still running.
	// It will be empty in this case, signifying the task produced no outputs.
	outputDir := ""
	if cmd.outputDir != "" {
		var outErr error
		outputDir, outErr = prepareOutputDir(cmd.outputDir, res.taskID)
		if outErr != nil && res.err == nil {
			res.err = outErr
		}
	}

	// If failed to fetch the task status (or create the output directory), don't
	// even bother to fetch the results.
	if res.err != nil {
		res.err = normalizeCtxErr(res.err)
		logging.Warningf(ctx, "%s", res.SummaryLine())
		return
	}

	if res.result.State == swarmingv2.TaskState_PENDING || res.result.State == swarmingv2.TaskState_RUNNING {
		logging.Infof(ctx, "%s", res.SummaryLine())
		return
	}

	eg, ctx := errgroup.WithContext(ctx)

	// Fetch combined stderr/stdout (aka console) output if asked for it.
	wantConsoleOut := cmd.taskOutput != taskOutputNone
	if wantConsoleOut {
		eg.Go(func() error {
			logging.Debugf(ctx, "%s: fetching console output", res.taskID)
			var output bytes.Buffer
			_, err := svc.TaskOutput(ctx, res.taskID, &output)
			if err != nil {
				return errors.Annotate(err, "fetching console output of %s", res.taskID).Err()
			}
			res.output = strings.ToValidUTF8(output.String(), "\uFFFD")
			return nil
		})
	}

	// Fetch isolated files if asked for them and the task has them.
	wantIsolatedOut := outputDir != "" && res.result.CasOutputRoot != nil
	if wantIsolatedOut {
		eg.Go(func() error {
			logging.Debugf(ctx, "%s: fetching isolated output", res.taskID)
			output, err := svc.FilesFromCAS(ctx, outputDir, &swarmingv2.CASReference{
				CasInstance: res.result.CasOutputRoot.CasInstance,
				Digest: &swarmingv2.Digest{
					Hash:      res.result.CasOutputRoot.Digest.Hash,
					SizeBytes: res.result.CasOutputRoot.Digest.SizeBytes,
				},
			})
			if err != nil {
				return errors.Annotate(err, "fetching isolated output of %s", res.taskID).Err()
			}
			res.outputs = output
			return nil
		})
	}

	if wantConsoleOut || wantIsolatedOut {
		res.err = normalizeCtxErr(eg.Wait())
		if res.err == nil {
			logging.Debugf(ctx, "%s: finished fetching outputs", res.taskID)
		}
	}

	// Log as soon as we are done with the task.
	if res.err != nil {
		logging.Warningf(ctx, "%s", res.SummaryLine())
	} else {
		logging.Infof(ctx, "%s", res.SummaryLine())
	}
}

// normalizeCtxErr replaces context errors with ones that serialize to
// documented values.
func normalizeCtxErr(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("rpc_timeout")
	case errors.Is(err, context.Canceled):
		return errors.New("rpc_canceled")
	default:
		return err
	}
}

// prepareOutputDir creates the directory for storing isolated outputs.
func prepareOutputDir(outputDir, taskID string) (string, error) {
	// This should never happen, but check anyway since we do not want to
	// accidentally delete all of `outputDir`.
	if taskID == "" {
		panic("should never happen")
	}
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

// summarizeResultsPython generates a summary compatible with python's client.
func summarizeResultsPython(taskIDs []string, results map[string]taskResult) (any, error) {
	shards := make([]map[string]any, 0, len(results))

	for _, taskID := range taskIDs {
		result := results[taskID]
		if result.result == nil {
			// This means there was an error fetching the task. Note that python
			// results format has no way to communicate errors. We just write `null`
			// into the corresponding slot to indicate the task is not ready.
			shards = append(shards, nil)
			continue
		}

		// Convert TaskResultResponse proto to a free-form map[string]any to inject
		// `output` as an extra field not present in the original proto.
		buf, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(result.result)
		if err != nil {
			return nil, err
		}
		var jsonResult map[string]any
		if err := json.Unmarshal(buf, &jsonResult); err != nil {
			return nil, err
		}
		jsonResult["output"] = result.output

		// Report the completed task result.
		shards = append(shards, jsonResult)
	}

	return map[string]any{"shards": shards}, nil
}

// summarizeResults generates a summary of the task results.
func (cmd *collectImpl) summarizeResults(results map[string]taskResult) (map[string]*clipb.ResultSummaryEntry, error) {
	summary := make(map[string]*clipb.ResultSummaryEntry, len(results))
	for taskID, result := range results {
		entry := &clipb.ResultSummaryEntry{
			Results: result.result,
			Outputs: result.outputs,
		}
		if result.err != nil {
			entry.Error = result.err.Error()
			if entry.Error == "" {
				entry.Error = "unknown"
			}
		}
		if result.result != nil {
			if cmd.taskOutput.includesJSON() {
				entry.Output = result.output
			}
		}
		summary[taskID] = entry
	}
	return summary, nil
}

func (cmd *collectImpl) Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra base.Extra) error {
	// The context used for waiting for task completion.
	var wctx context.Context
	var wcancel context.CancelFunc
	if cmd.timeout > 0 {
		wctx, wcancel = clock.WithTimeout(ctx, cmd.timeout)
	} else {
		wctx, wcancel = context.WithCancel(ctx)
	}
	defer wcancel()

	var mode swarming.WaitMode
	switch {
	case cmd.wait && cmd.eager:
		mode = swarming.WaitAny
	case cmd.wait && !cmd.eager:
		mode = swarming.WaitAll
	default:
		mode = swarming.NoWait
	}

	fields := swarming.TaskResultFields{
		WithPerf: cmd.perf,
	}

	// Collect statuses of all tasks and start fetching their results as soon
	// as they are available, in parallel. Fetch results using the root `ctx`
	// (to not be affected by -timeout, which is a *waiting* timeout).
	resultsCh := make(chan taskResult, len(cmd.taskIDs))
	swarming.GetMany(wctx, svc, cmd.taskIDs, &fields, mode, func(taskID string, res *swarmingv2.TaskResultResponse, err error) {
		go func() {
			taskRes := taskResult{taskID: taskID, result: res, err: err}
			cmd.fetchTaskResults(ctx, svc, &taskRes)
			resultsCh <- taskRes
		}()
	})

	// Wait for all fetchTaskResults(...) calls to complete.
	resultByID := make(map[string]taskResult, len(cmd.taskIDs))
	for i := 0; i < len(cmd.taskIDs); i++ {
		res := <-resultsCh
		resultByID[res.taskID] = res
	}

	// Report results to stdout if requested. Preserve the order.
	if cmd.taskOutput.includesConsole() {
		for _, taskID := range cmd.taskIDs {
			res := resultByID[taskID]
			fmt.Fprintln(extra.Stdout, res.SummaryLine())
			if res.output != "" {
				fmt.Fprintln(extra.Stdout, res.output)
			}
		}
	}

	// Don't bother assembling the summary if we aren't going to store it.
	if extra.OutputJSON == "" {
		return nil
	}

	// TODO(crbug.com/894045): Python-compatible summary is actually the most
	// commonly used now (used by recipes).
	if cmd.taskSummaryPython {
		summary, err := summarizeResultsPython(cmd.taskIDs, resultByID)
		if err != nil {
			return err
		}
		return output.JSON(sink, summary)
	}

	summary, err := cmd.summarizeResults(resultByID)
	if err != nil {
		return err
	}
	return output.Map(sink, summary)
}
