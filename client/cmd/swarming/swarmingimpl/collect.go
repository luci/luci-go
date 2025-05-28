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
	"slices"
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
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

type taskOutputOption []string

func (t taskOutputOption) includesJSON() bool {
	return slices.Contains(t, "json")
}

func (t taskOutputOption) includesConsole() bool {
	return slices.Contains(t, "console")
}

func (t taskOutputOption) includesDir() (path string, ok bool) {
	for _, v := range t {
		if v == "dir" {
			return "", true
		}
		if path, ok := strings.CutPrefix(v, "dir:"); ok {
			return path, true
		}
	}
	return "", false
}

func (t taskOutputOption) String() string {
	if len(t) == 0 {
		return "none"
	}
	return strings.Join(t, ",")
}

func (t *taskOutputOption) Set(s string) error {
	if slices.Contains(*t, s) {
		return nil
	}
	switch {
	case s == "none" || s == "":
		// Nothing.
	case s == "console":
		*t = append(*t, s)
	case s == "json":
		*t = append(*t, s)
	case s == "dir" || strings.HasPrefix(s, "dir:"):
		if _, yes := t.includesDir(); yes {
			return errors.Reason("cannot have more than one \"dir\" destination").Err()
		}
		*t = append(*t, s)
	case s == "all":
		_ = t.Set("console")
		_ = t.Set("json")
	default:
		return errors.Reason("invalid task output option").Err()
	}
	return nil
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
	// output will only be populated if requested (nil otherwise).
	output *textOutput

	// outputs is a list of file outputs from a task, downloaded from an isolate server.
	// outputs will only be populated if requested.
	outputs []string

	// err is set if an operational error occurred while doing RPCs to gather the
	// task result, which includes errors received from the server.
	err error

	// summaryLogged is true if we already logged the task summary.
	summaryLogged bool
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

// logSummary logs the task summary if it hasn't been logged before.
func (t *taskResult) logSummary(ctx context.Context) {
	if !t.summaryLogged {
		t.summaryLogged = true
		if t.err != nil {
			logging.Warningf(ctx, "%s", t.SummaryLine())
		} else {
			logging.Infof(ctx, "%s", t.SummaryLine())
		}
	}
}

// textOutput is a container for the fetched task's console output.
//
// Task console output can be huge (hundreds of megabytes). We store it in a
// file to avoid OOMs. When `-task-output-stdout dir` is set, this is the final
// output file returned to the caller. Otherwise it is some temporary file
// deleted after we are done with it.
//
// Assumes `fetch` is called before `dump` and no calls are happening
// concurrently.
type textOutput struct {
	file *os.File // the backing file open in RW mode
	temp bool     // if true, delete the file after closing it
}

func (t *textOutput) close() error {
	var merr errors.MultiError
	merr.MaybeAdd(t.file.Close())
	if t.temp {
		merr.MaybeAdd(os.Remove(t.file.Name()))
	}
	return merr.AsError()
}

func (t *textOutput) fetch(ctx context.Context, svc swarming.Client, taskID string) error {
	_, err := svc.TaskOutput(ctx, taskID, t.file)
	return err
}

func (t *textOutput) dump(out io.Writer) (n int64, err error) {
	if _, err := t.file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	return io.Copy(out, t.file)
}

func (t *textOutput) dumpToUTF8() (string, error) {
	var buf strings.Builder
	if _, err := t.dump(&buf); err != nil {
		return "", err
	}
	return strings.ToValidUTF8(buf.String(), "\uFFFD"), nil
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
tasks. Can be specified multiple times to emit the log into multiple places.
Its possible values:
  * "none" (default): don't fetch the console log at all.
  * "console": dump the log to stdout.
  * "json": dump the log into the JSON output (in "result.output" field).
  * "dir:<path>": dump the log into <path>/<task-ID>.txt.
  * "dir": dump the log into <output-dir>/<task-ID>.txt (see -output-dir flag).
  * "all": a legacy alias for combination of "console" and "json".

Flag -output-dir controls where to store isolated outputs of completed tasks,
as well as tasks' console log (when using "-task-output-stdout dir" flag). If it
is unset (default), isolated outputs will not be fetched. Otherwise isolated
outputs of a completed task with ID <task-ID> will be downloaded to
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
	textOutputDir     string // where to store console output or "" for temp
	eager             bool
	perf              bool
	jsonInput         string

	outputFetchConcurrency int
}

func (cmd *collectImpl) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.wait, "wait", true, "If set, wait for tasks to complete. Otherwise just poll their current state.")
	fs.DurationVar(&cmd.timeout, "timeout", 0, "Timeout to wait for tasks to complete when -wait is set. Set to 0 for no timeout.")
	fs.BoolVar(&cmd.eager, "eager", false, "If set, stop waiting whenever any task finishes, do not wait for all of them to finish.")

	//TODO(tikuta): Remove this flag once crbug.com/894045 is fixed.
	fs.BoolVar(&cmd.taskSummaryPython, "task-summary-python", false, "Generate python client compatible task summary json.")

	fs.BoolVar(&cmd.perf, "perf", false, "Include performance statistics.")
	fs.Var(&cmd.taskOutput, "task-output-stdout", "Where to put each task's console output (combined stderr and stdout): none, json, console, dir[:<path>], all (a legacy alias for json+console). Can be specified multiple times.")
	fs.StringVar(&cmd.outputDir, "output-dir", "", "Where to download outputs to.")

	fs.StringVar(&cmd.jsonInput, "requests-json", "", "Load the task IDs from a .json file as saved by \"trigger -json-output\".")

	fs.IntVar(&cmd.outputFetchConcurrency, "output-fetch-concurrency", 8, "Limits how many concurrent result fetches are allowed (to avoid OOMs). 0 is unlimited.")
}

func (cmd *collectImpl) ParseInputs(ctx context.Context, args []string, env subcommands.Env, extra base.Extra) error {
	if cmd.timeout < 0 {
		return errors.Reason("negative timeout is not allowed").Err()
	}
	if !cmd.wait && cmd.timeout > 0 {
		return errors.Reason("do not specify -timeout with -wait=false").Err()
	}

	// Figure out where to store files with tasks' stdout.
	var ok bool
	cmd.textOutputDir, ok = cmd.taskOutput.includesDir()
	if ok {
		if cmd.textOutputDir == "" {
			cmd.textOutputDir = cmd.outputDir
		}
		if cmd.textOutputDir == "" {
			return errors.Reason(
				"cannot figure out where to store task console output: " +
					"either specify the directory as `-task-output-stdout dir:<path>` " +
					"(if only console output is required) or pass `-output-dir` " +
					"(if both text and isolated output are required)",
			).Err()
		}
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

	// If asked to fetch task's console output, prepare the storage file for it.
	wantConsoleOut := len(cmd.taskOutput) != 0
	if wantConsoleOut {
		var outErr error
		res.output, outErr = prepareTextOutput(cmd.textOutputDir, res.taskID)
		if outErr != nil && res.err == nil {
			res.err = outErr
		}
	}

	// If failed to fetch the task status (or create the output directory), don't
	// even bother to fetch the results.
	if res.err != nil {
		res.err = normalizeCtxErr(res.err)
		res.logSummary(ctx)
		return
	}

	if res.result.State == swarmingv2.TaskState_PENDING || res.result.State == swarmingv2.TaskState_RUNNING {
		res.logSummary(ctx)
		return
	}

	eg, ectx := errgroup.WithContext(ctx)

	// Fetch combined stderr/stdout (aka console) output if asked for it.
	if wantConsoleOut {
		eg.Go(func() error {
			logging.Debugf(ectx, "%s: fetching console output", res.taskID)
			if err := res.output.fetch(ectx, svc, res.taskID); err != nil {
				return errors.Annotate(err, "fetching console output of %s", res.taskID).Err()
			}
			return nil
		})
	}

	// Fetch isolated files if asked for them and the task has them.
	wantIsolatedOut := outputDir != "" && res.result.CasOutputRoot != nil
	if wantIsolatedOut {
		eg.Go(func() error {
			logging.Debugf(ectx, "%s: fetching isolated output", res.taskID)
			output, err := svc.FilesFromCAS(ectx, outputDir, &swarmingv2.CASReference{
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
		if res.err != nil && ctx.Err() != nil {
			// When the root context expires, `res.err` may end up having all sorts of
			// errors depending on what exactly was happening when the context
			// expired. Use a cleaner context error in that case.
			res.err = normalizeCtxErr(ctx.Err())
		}
		if res.err == nil {
			logging.Debugf(ctx, "%s: finished fetching outputs", res.taskID)
		}
	}

	res.logSummary(ctx)
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

// prepareTextOutput creates a file for storing task's console output.
//
// If `outputDir` is empty, will use a temporary file.
func prepareTextOutput(outputDir, taskID string) (*textOutput, error) {
	if outputDir == "" {
		file, err := os.CreateTemp("", fmt.Sprintf("swarming_%s_*.txt", taskID))
		if err != nil {
			return nil, errors.Annotate(err, "failed to create a temp file for storing console output").Err()
		}
		return &textOutput{file: file, temp: true}, nil
	}
	if err := os.MkdirAll(outputDir, 0777); err != nil {
		return nil, errors.Annotate(err, "failed to create directory: %s", outputDir).Err()
	}
	file, err := os.Create(filepath.Join(outputDir, taskID+".txt"))
	if err != nil {
		return nil, errors.Annotate(err, "failed to create a file for storing console output").Err()
	}
	return &textOutput{file: file}, nil
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

	// A limiter on number of concurrent fetches to avoid OOMs.
	acquireSlot := func() error { return nil }
	releaseSlot := func() {}
	if cmd.outputFetchConcurrency > 0 {
		sem := semaphore.NewWeighted(int64(cmd.outputFetchConcurrency))
		acquireSlot = func() error { return sem.Acquire(ctx, 1) }
		releaseSlot = func() { sem.Release(1) }
	}

	// Collect statuses of all tasks and start fetching their results as soon
	// as they are available, in parallel. Fetch results using the root `ctx`
	// (to not be affected by -timeout, which is a *waiting* timeout). Call
	// GetMany in a background goroutine in order to start reading from
	// `resultsCh` below in parallel (to report results as soon as they are
	// available).
	resultsCh := make(chan taskResult)
	go func() {
		swarming.GetMany(wctx, svc, cmd.taskIDs, &fields, mode, func(taskID string, res *swarmingv2.TaskResultResponse, err error) {
			go func() {
				taskRes := taskResult{taskID: taskID, result: res, err: err}
				if acqErr := acquireSlot(); acqErr != nil {
					taskRes.err = normalizeCtxErr(acqErr)
				} else {
					cmd.fetchTaskResults(ctx, svc, &taskRes)
					releaseSlot()
				}
				resultsCh <- taskRes
			}()
		})
	}()

	// TODO(crbug.com/894045): Get rid of taskSummaryPython mode.
	var emitter summaryEmitter
	switch {
	case extra.OutputJSON == "":
		emitter = noopSummaryEmitter{}
	case cmd.taskSummaryPython:
		emitter = &legacySummaryEmitter{
			sink:           sink,
			populateStdout: cmd.taskOutput.includesJSON(),
			taskIDs:        cmd.taskIDs,
			resultByID:     make(map[string]*taskResult, len(cmd.taskIDs)),
		}
	default:
		emitter = &defaultSummaryEmitter{
			sink:           sink,
			populateStdout: cmd.taskOutput.includesJSON(),
		}
	}

	// All errors dealing with the output.
	var outputErrs errors.MultiError

	// Wait for all fetchTaskResults(...) calls to complete. Emit their output
	// as soon as it is available.
	emitter.start(&outputErrs)
	for i := 0; i < len(cmd.taskIDs); i++ {
		res := <-resultsCh
		res.logSummary(ctx) // might be context cancellation, log it
		if cmd.taskOutput.includesConsole() {
			fmt.Fprintln(extra.Stdout, res.SummaryLine())
			if res.output != nil {
				switch written, err := res.output.dump(extra.Stdout); {
				case err != nil:
					outputErrs.MaybeAdd(errors.Annotate(err, "emitting stdout of %q", res.taskID).Err())
				case written != 0:
					fmt.Fprintln(extra.Stdout)
				}
			}
		}
		emitter.emit(&res, &outputErrs)
	}
	emitter.finish(&outputErrs)

	return outputErrs.AsError()
}

////////////////////////////////////////////////////////////////////////////////

// summaryEmitters knows how to write task result entries to the JSON output.
//
// Takes ownership of *taskResult passed to it. Writes all errors into the
// given MultiError.
type summaryEmitter interface {
	start(merr *errors.MultiError)
	emit(res *taskResult, merr *errors.MultiError)
	finish(merr *errors.MultiError)
}

// Just closes outputs without reading them.
type noopSummaryEmitter struct{}

func (noopSummaryEmitter) start(merr *errors.MultiError) {}

func (noopSummaryEmitter) emit(res *taskResult, merr *errors.MultiError) {
	if res.output != nil {
		merr.MaybeAdd(errors.WrapIf(res.output.close(), "closing console output of %q", res.taskID))
	}
}

func (noopSummaryEmitter) finish(merr *errors.MultiError) {}

// The non-legacy summary format is an unordered dict. We can write entries
// for it in any order. This allows us to "forget" them (and free memory
// allocated for task's stdout) as soon as possible.
type defaultSummaryEmitter struct {
	sink           *output.Sink
	populateStdout bool
}

func (e *defaultSummaryEmitter) start(merr *errors.MultiError) {
	merr.MaybeAdd(output.StartMap(e.sink))
}

func (e *defaultSummaryEmitter) emit(res *taskResult, merr *errors.MultiError) {
	entry := &clipb.ResultSummaryEntry{
		Results: res.result,
		Outputs: res.outputs,
	}

	if res.err != nil {
		entry.Error = res.err.Error()
		if entry.Error == "" {
			entry.Error = "unknown"
		}
	}

	if e.populateStdout && res.result != nil && res.output != nil {
		var err error
		entry.Output, err = res.output.dumpToUTF8()
		merr.MaybeAdd(errors.WrapIf(err, "reading task output %q", res.taskID))
	}
	if res.output != nil {
		merr.MaybeAdd(errors.WrapIf(res.output.close(), "closing console output of %q", res.taskID))
	}

	err := output.MapEntry(e.sink, res.taskID, entry)
	merr.MaybeAdd(errors.WrapIf(err, "writing JSON output for task %q", res.taskID))
}

func (e *defaultSummaryEmitter) finish(merr *errors.MultiError) {}

// Legacy Python summary is a list of proto message ordered in the same order
// as `cmd.taskIDs`. We get results from the channel in some arbitrary order.
// It means we'll generally have to buffer them all before we can write them.
type legacySummaryEmitter struct {
	sink           *output.Sink
	populateStdout bool
	taskIDs        []string // task IDs in order we need to write them
	resultByID     map[string]*taskResult
}

func (e *legacySummaryEmitter) start(merr *errors.MultiError) {
	// All output is emitted in finish all at once.
}

func (e *legacySummaryEmitter) emit(res *taskResult, merr *errors.MultiError) {
	e.resultByID[res.taskID] = res
}

func (e *legacySummaryEmitter) finish(merr *errors.MultiError) {
	shards := make([]map[string]any, 0, len(e.taskIDs))

	for _, taskID := range e.taskIDs {
		result := e.resultByID[taskID]
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
			merr.MaybeAdd(errors.Annotate(err, "JSON serializing results of %q", taskID).Err())
			shards = append(shards, nil) // indicates there was an error
			continue
		}
		var jsonResult map[string]any
		if err := json.Unmarshal(buf, &jsonResult); err != nil {
			merr.MaybeAdd(errors.Annotate(err, "JSON deserializing results of %q", taskID).Err())
			shards = append(shards, nil) // indicates there was an error
			continue
		}

		// Load output as UTF-8 and put into the JSON struct.
		jsonResult["output"] = ""
		if e.populateStdout && result.output != nil {
			jsonResult["output"], err = result.output.dumpToUTF8()
			merr.MaybeAdd(errors.WrapIf(err, "reading task output %q", taskID))
		}

		// Report the completed task result.
		shards = append(shards, jsonResult)
	}

	// Close all outputs, we don't need them anymore.
	for _, res := range e.resultByID {
		if res.output != nil {
			merr.MaybeAdd(errors.WrapIf(res.output.close(), "closing console output of %q", res.taskID))
		}
	}

	// Finally write the combined JSON summary for all shards.
	err := output.JSON(e.sink, map[string]any{"shards": shards})
	merr.MaybeAdd(errors.WrapIf(err, "writing JSON summary"))
}
