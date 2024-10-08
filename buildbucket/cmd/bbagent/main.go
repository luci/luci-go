// Copyright 2019 The LUCI Authors.
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

// Command bbagent is Buildbucket's agent running in swarming.
//
// This executable creates a luciexe 'host' environment, and runs the
// Buildbucket build's exe within this environment. Please see
// https://go.chromium.org/luci/luciexe for details about the 'luciexe'
// protocol.
//
// This command is an implementation detail of Buildbucket.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cipd/client/cipd/platform"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/luciexe"
	"go.chromium.org/luci/luciexe/host"
	"go.chromium.org/luci/luciexe/invoke"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"

	_ "net/http/pprof"
)

type closeOnceCh struct {
	ch   chan struct{}
	once sync.Once
}

func newCloseOnceCh() *closeOnceCh {
	return &closeOnceCh{
		ch:   make(chan struct{}),
		once: sync.Once{},
	}
}

func (c *closeOnceCh) close() {
	c.once.Do(func() { close(c.ch) })
}

type clientInput struct {
	bbclient BuildsClient
	input    *bbpb.BBAgentArgs
}

// stopInfo contains different channels involved in causing the build to stop and shutdown
type stopInfo struct {
	invokeErr       chan error
	shutdownCh      *closeOnceCh
	canceledBuildCh *closeOnceCh
	dispatcherErrCh <-chan error
}

// stopEvents monitors events that cause the build to stop.
// * Monitors the `dispatcherErrCh` and checks for fatal error.
//   - Stops the build shuttling and shuts down the luciexe if a fatal error
//     is received.
//
// * Monitors the returned build from UpdateBuild rpcs and checks if the
//
//	build has been canceled.
//	* Shuts down the luciexe if the build is canceled.
func (si stopInfo) stopEvents(ctx context.Context, c clientInput, fatalUpdateBuildErrorSlot *atomic.Value) {
	stopped := false
	for {
		select {
		case err := <-si.invokeErr:
			checkReport(ctx, c, errors.Annotate(err, "could not invoke luciexe").Err())
		case err := <-si.dispatcherErrCh:
			if !stopped && grpcutil.Code(err) == codes.InvalidArgument {
				si.shutdownCh.close()
				fatalUpdateBuildErrorSlot.Store(err)
				stopped = true
			}
		case <-si.canceledBuildCh.ch:
			// The build has been canceled, bail out early.
			si.shutdownCh.close()
		case <-ctx.Done():
			return
		}
	}
}

func check(ctx context.Context, err error) {
	if err != nil {
		logging.Errorf(ctx, err.Error())
		os.Exit(1)
	}
}

// checkReport logs errors, tries to report them to buildbucket, then exits
func checkReport(ctx context.Context, c clientInput, err error) {
	if err != nil {
		logging.Errorf(ctx, err.Error())
		if _, bbErr := c.bbclient.UpdateBuild(
			ctx,
			&bbpb.UpdateBuildRequest{
				Build: &bbpb.Build{
					Id: c.input.Build.Id,
					Output: &bbpb.Build_Output{
						Status:          bbpb.Status_INFRA_FAILURE,
						SummaryMarkdown: fmt.Sprintf("fatal error in startup: %s", err),
					},
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"build.output.status", "build.output.summary_markdown"}},
			}); bbErr != nil {
			logging.Errorf(ctx, "Failed to report INFRA_FAILURE status to Buildbucket: %s", bbErr)
		}
		os.Exit(1)
	}
}

func cancelBuild(ctx context.Context, bbclient BuildsClient, bld *bbpb.Build) (retCode int) {
	logging.Infof(ctx, "The build is in the cancel process, cancel time is %s. Actually cancel it now.", bld.CancelTime.AsTime())
	_, err := bbclient.UpdateBuild(
		ctx,
		&bbpb.UpdateBuildRequest{
			Build: &bbpb.Build{
				Id: bld.Id,
				Output: &bbpb.Build_Output{
					Status: bbpb.Status_CANCELED,
				},
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"build.output.status"}},
		})
	if err != nil {
		logging.Errorf(ctx, "failed to actually cancel the build: %s", err)
		return 1
	}
	return 0
}

func parseBbAgentArgs(ctx context.Context, arg string, bbagentCtx *bbpb.BuildbucketAgentContext) clientInput {
	// TODO(crbug/1219018): Remove CLI BBAgentArgs mode in favor of -host + -build-id.
	logging.Debugf(ctx, "parsing BBAgentArgs")
	input, err := bbinput.Parse(arg)
	check(ctx, errors.Annotate(err, "could not unmarshal BBAgentArgs").Err())
	bbclient, err := newBuildsClient(ctx, bbagentCtx, input.Build.Infra.Buildbucket.GetHostname(), retry.Default)
	check(ctx, errors.Annotate(err, "could not connect to Buildbucket").Err())
	return clientInput{bbclient, input}
}

func startBuild(ctx context.Context, bbclient BuildsClient, buildID int64, taskID string) (*bbpb.StartBuildResponse, error) {
	return bbclient.StartBuild(
		ctx,
		&bbpb.StartBuildRequest{
			RequestId: uuid.NewSHA1(uuid.Nil, []byte(taskID)).String(),
			BuildId:   buildID,
			TaskId:    taskID,
		},
	)
}

func retrieveTaskIDFromContext(ctx context.Context) string {
	swarmingCtx := lucictx.GetSwarming(ctx)
	if swarmingCtx == nil || swarmingCtx.GetTask() == nil || swarmingCtx.Task.GetTaskId() == "" {
		check(ctx, fmt.Errorf("incomplete swarming context: %+v", swarmingCtx))
	}
	return swarmingCtx.Task.TaskId
}

// Get the build info by the provided buildID.
//
// If the swarming task id can be found from swarming part of luci context, this
// function also starts the build.
//
// Note: taskID may be updated if the swarming task id can be found from
// swarming part of luci context.
func parseHostBuildID(ctx context.Context, hostname *string, buildID *int64, bbagentCtx *bbpb.BuildbucketAgentContext) clientInput {
	logging.Debugf(ctx, "fetching build %d", *buildID)
	bbclient, err := newBuildsClient(ctx, bbagentCtx, *hostname, defaultRetryStrategy)
	check(ctx, errors.Annotate(err, "could not connect to Buildbucket").Err())

	var build *bbpb.Build
	if bbagentCtx.TaskId == "" {
		// Get everything from the build.
		// Here we use UpdateBuild instead of GetBuild, so that
		// * bbagent can always get the build because of the build token.
		//   * This was not guaranteed for GetBuild, because it's possible that a
		//     service account has permission to run a build but doesn't have
		//     permission to view the build.
		//   * bbagent could tear down the build earlier if the parent build is canceled.
		// TODO(crbug.com/1019272):  we should also use this RPC to set the initial
		// status of the build to STARTED (and also be prepared to quit in the case
		// that this build got double-scheduled).
		build, err = bbclient.UpdateBuild(
			ctx,
			&bbpb.UpdateBuildRequest{
				Build: &bbpb.Build{
					Id: *buildID,
				},
				Mask: &bbpb.BuildMask{
					AllFields: true,
				},
			})
		check(ctx, errors.Annotate(err, "failed to fetch build").Err())
	} else {
		var res *bbpb.StartBuildResponse
		res, err = startBuild(ctx, bbclient, *buildID, bbagentCtx.TaskId)
		if err != nil && buildbucket.DuplicateTask.In(err) {
			// This task is a duplicate, bail out.
			logging.Infof(ctx, err.Error())
			os.Exit(0)
		}
		check(ctx, errors.Annotate(err, "failed to start build").Err())
		build = res.Build
		bbagentCtx.Secrets.BuildToken = res.UpdateBuildToken
	}

	input := &bbpb.BBAgentArgs{
		Build:                  build,
		CacheDir:               protoutil.CacheDir(build),
		KnownPublicGerritHosts: build.Infra.Buildbucket.KnownPublicGerritHosts,
		PayloadPath:            protoutil.ExePayloadPath(build),
	}
	return clientInput{bbclient, input}
}

// backFillTaskInfoNeeded checks if need to backfill the backend task info.
func backFillTaskInfoNeeded(ci clientInput) bool {
	swarming := ci.input.Build.GetInfra().GetSwarming()
	if swarming != nil {
		// Only need to back fill if bot dimensions have not been populated.
		return len(swarming.GetBotDimensions()) == 0
	}

	// Check backend task.
	task := ci.input.Build.GetInfra().GetBackend().GetTask()
	if task == nil {
		// Should not happen, but we cannot backfill in this case anyway.
		return false
	}
	if !strings.HasPrefix(task.GetId().GetTarget(), "swarming://") {
		// Not a Swarming backend, no need to back fill.
		return false
	}
	details := task.GetDetails().GetFields()
	if len(details) == 0 {
		// No task details populated, back fill is needed.
		return true
	}
	_, ok := details["bot_dimensions"]
	// Only need to back fill if bot dimensions have not been populated.
	return !ok

}

func chooseCacheDir(input *bbpb.BBAgentArgs, cacheBaseFlag string) string {
	cache := input.GetCacheDir()
	if input.Build.GetInfra().GetBackend() != nil && cacheBaseFlag != "" {
		cache = cacheBaseFlag
	}
	return cache
}

// backFillTaskInfo gets the task info from LUCI_CONTEXT and backs fill it to input.Build.
//
// Currently only read from swarming part of LUCI_CONTEXT and only update
// input.Infra.Swarming.BotDimensions.
// TODO(crbug.com/1370221): read LUCI_CONTEXT based on the backend task target,
// and then update input.Infra.Backend.Task.
func backFillTaskInfo(ctx context.Context, ci clientInput) int {
	swarmingCtx := lucictx.GetSwarming(ctx)
	if swarmingCtx == nil || swarmingCtx.GetTask() == nil || len(swarmingCtx.Task.GetBotDimensions()) == 0 {
		logging.Errorf(ctx, "incomplete swarming context")
		return 1
	}

	botDimensions := make([]*bbpb.StringPair, 0, len(swarmingCtx.Task.BotDimensions))
	for _, dim := range swarmingCtx.Task.BotDimensions {
		parts := strings.SplitN(dim, ":", 2)
		if len(parts) != 2 {
			logging.Errorf(ctx, "bot_dimensions %s in swarming context is malformatted", dim)
			continue
		}
		botDimensions = append(botDimensions, &bbpb.StringPair{Key: parts[0], Value: parts[1]})
	}

	if ci.input.Build.Infra.Swarming != nil {
		ci.input.Build.Infra.Swarming.BotDimensions = botDimensions
		return 0
	}

	if ci.input.Build.Infra.Backend.GetTask() == nil {
		logging.Errorf(ctx, "Neither infra.swarming nor infra.backend are found in the build")
		return 1
	}
	var err error
	ci.input.Build.Infra.Backend.Task.Details, err = protoutil.AddBotDimensionsToTaskDetails(botDimensions, ci.input.Build.Infra.Backend.Task.Details)
	if err != nil {
		logging.Errorf(ctx, "failed to back fill infra.backend.task.details: %s", err)
		return 1
	}
	return 0
}

// prepareInputBuild sets status=STARTED and adds log entries
func prepareInputBuild(ctx context.Context, build, updatedBuild *bbpb.Build) {
	if updatedBuild != nil {
		build.Status = updatedBuild.Status
		build.StartTime = updatedBuild.StartTime
		build.UpdateTime = updatedBuild.UpdateTime
	}

	// TODO(iannucci): this is sketchy, but we preemptively add the log entries
	// for the top level user stdout/stderr streams.
	//
	// Really, `invoke.Start` is the one that knows how to arrange the
	// Output.Logs, but host.Run makes a copy of this build immediately. Find
	// a way to set these up nicely (maybe have opts.BaseBuild be a function
	// returning an immutable bbpb.Build?).
	build.Output = &bbpb.Build_Output{
		Logs: []*bbpb.Log{
			{Name: "stdout", Url: "stdout"},
			{Name: "stderr", Url: "stderr"},
		},
		Status: build.GetOutput().GetStatus(),
	}
	populateSwarmingInfoFromEnv(build, environ.System())
}

// downloadInputs downloads CIPD and CAS inputs then updates the build.
func downloadInputs(ctx context.Context, cwd, cacheBase string, c clientInput) int {
	// Most likely happens in `led get-build` process where it creates from an old build
	// before new Agent field was there. This new feature shouldn't work for those builds.
	if c.input.Build.Infra.Buildbucket.Agent == nil {
		checkReport(ctx, c, errors.New("Cannot enable downloading cipd pkgs feature; Build Agent field is not set"))
	}

	agent := c.input.Build.Infra.Buildbucket.Agent
	agent.Output = &bbpb.BuildInfra_Buildbucket_Agent_Output{
		Status: bbpb.Status_STARTED,
	}
	updateReq := &bbpb.UpdateBuildRequest{
		Build: &bbpb.Build{
			Id: c.input.Build.Id,
			Infra: &bbpb.BuildInfra{
				Buildbucket: &bbpb.BuildInfra_Buildbucket{
					Agent: agent,
				},
			},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"build.infra.buildbucket.agent.output"}},
		Mask:       readMask,
	}
	bldStartCipd, err := c.bbclient.UpdateBuild(ctx, updateReq)
	if err != nil {
		// Carry on and bear the non-fatal update failure.
		logging.Warningf(ctx, "Failed to report build agent STARTED status: %s", err)
	}
	// The build has been canceled, bail out early.
	if bldStartCipd.CancelTime != nil {
		return cancelBuild(ctx, c.bbclient, bldStartCipd)
	}

	agent.Output.AgentPlatform = platform.CurrentPlatform()

	// Encapsulate all the installation logic with a defer to set the
	// TotalDuration. As we add more installation logic (e.g. RBE-CAS),
	// TotalDuration should continue to surround that logic.
	err = func() error {
		start := clock.Now(ctx)
		defer func() {
			agent.Output.TotalDuration = &durationpb.Duration{
				Seconds: int64(clock.Since(ctx, start).Round(time.Second).Seconds()),
			}
		}()
		// Download CIPD.
		if err := installCipd(ctx, c.input.Build, cwd, cacheBase, agent.Output.AgentPlatform); err != nil {
			return err
		}
		if err := prependPath(c.input.Build, cwd); err != nil {
			return err
		}
		if err := installCipdPackages(ctx, c.input.Build, cwd, cacheBase); err != nil {
			return err
		}
		return downloadCasFiles(ctx, c.input.Build, cwd)
	}()

	if err != nil {
		logging.Errorf(ctx, "Failure in installing user packages: %s", err)
		agent.Output.Status = bbpb.Status_FAILURE
		agent.Output.SummaryMarkdown = err.Error()
		updateReq.Build.Output = &bbpb.Build_Output{
			Status:          bbpb.Status_INFRA_FAILURE,
			SummaryMarkdown: "Failed to install user packages for this build",
		}
		updateReq.UpdateMask.Paths = append(updateReq.UpdateMask.Paths, "build.output.status", "build.output.summary_markdown")
	} else {
		agent.Output.Status = bbpb.Status_SUCCESS
		if c.input.Build.Exe != nil {
			updateReq.UpdateMask.Paths = append(updateReq.UpdateMask.Paths, "build.infra.buildbucket.agent.purposes")
		}
	}

	bldCompleteCipd, bbErr := c.bbclient.UpdateBuild(ctx, updateReq)
	if bbErr != nil {
		logging.Warningf(ctx, "Failed to report build agent output status: %s", bbErr)
	} else if bldCompleteCipd.CancelTime != nil {
		// The build has been canceled, bail out early.
		return cancelBuild(ctx, c.bbclient, bldCompleteCipd)
	}
	if err != nil {
		return -1
	}
	return 0
}

func resolveExe(path string) (string, error) {
	if filepath.Ext(path) != "" {
		return path, nil
	}

	lme := errors.NewLazyMultiError(2)
	for i, ext := range []string{".exe", ".bat"} {
		candidate := path + ext
		if _, err := os.Stat(candidate); !lme.Assign(i, err) {
			return candidate, nil
		}
	}

	me := lme.Get().(errors.MultiError)
	return path, errors.Reason("cannot find .exe (%q) or .bat (%q)", me[0], me[1]).Err()
}

// processCmd resolves the cmd by constructing the absolute path and resolving
// the exe suffix.
func processCmd(path, cmd string) (string, error) {
	relPath := filepath.Join(path, cmd)
	absPath, err := filepath.Abs(relPath)
	if err != nil {
		return "", errors.Annotate(err, "absoluting %q", relPath).Err()
	}
	if runtime.GOOS == "windows" {
		absPath, err = resolveExe(absPath)
		if err != nil {
			return "", errors.Annotate(err, "resolving %q", absPath).Err()
		}
	}
	return absPath, nil
}

// processExeArgs processes the given "Executable" message into a single command
// which bbagent will invoke as a luciexe.
//
// This includes resolving paths relative to the current working directory
// (expected to be the task's root).
func processExeArgs(ctx context.Context, c clientInput) []string {
	exeArgs := make([]string, 0, len(c.input.Build.Exe.Wrapper)+len(c.input.Build.Exe.Cmd)+1)

	if len(c.input.Build.Exe.Wrapper) != 0 {
		exeArgs = append(exeArgs, c.input.Build.Exe.Wrapper...)
		exeArgs = append(exeArgs, "--")

		if strings.Contains(exeArgs[0], "/") || strings.Contains(exeArgs[0], "\\") {
			absPath, err := filepath.Abs(exeArgs[0])
			checkReport(ctx, c, errors.Annotate(err, "absoluting wrapper path: %q", exeArgs[0]).Err())
			exeArgs[0] = absPath
		}

		cmdPath, err := exec.LookPath(exeArgs[0])
		checkReport(ctx, c, errors.Annotate(err, "wrapper not found: %q", exeArgs[0]).Err())
		exeArgs[0] = cmdPath
	}

	exeCmd := c.input.Build.Exe.Cmd[0]
	payloadPath := c.input.PayloadPath
	if len(c.input.Build.Exe.Cmd) == 0 {
		// TODO(iannucci): delete me with ExecutablePath.
		payloadPath, exeCmd = path.Split(c.input.ExecutablePath)
	} else {
		for p, purpose := range c.input.Build.GetInfra().GetBuildbucket().GetAgent().GetPurposes() {
			if purpose == bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD {
				payloadPath = p
				break
			}
		}
	}
	exePath, err := processCmd(payloadPath, exeCmd)
	checkReport(ctx, c, err)
	exeArgs = append(exeArgs, exePath)
	exeArgs = append(exeArgs, c.input.Build.Exe.Cmd[1:]...)

	return exeArgs
}

// readyToFinalize returns true if fatalErr is nil and there's no additional
// errors finalizing the build.
func readyToFinalize(ctx context.Context, finalBuild *bbpb.Build, fatalErr error, statusDetails *bbpb.StatusDetails, outputFile *luciexe.OutputFlag) bool {
	if statusDetails != nil {
		if finalBuild.StatusDetails == nil {
			finalBuild.StatusDetails = &bbpb.StatusDetails{}
		}
		proto.Merge(finalBuild.StatusDetails, statusDetails)
	}

	// set final times
	now := timestamppb.New(clock.Now(ctx))
	finalBuild.UpdateTime = now
	finalBuild.EndTime = now

	var finalErrs errors.MultiError
	if fatalErr != nil {
		finalErrs = append(finalErrs, errors.Annotate(fatalErr, "fatal error in buildbucket.UpdateBuild").Err())
	}
	if err := outputFile.Write(finalBuild); err != nil {
		finalErrs = append(finalErrs, errors.Annotate(err, "writing final build").Err())
	}

	if len(finalErrs) > 0 {
		errors.Log(ctx, finalErrs)

		// we had some really bad error, just downgrade status and add a message to
		// summary markdown.
		finalBuild.Status = bbpb.Status_INFRA_FAILURE
		originalSM := finalBuild.SummaryMarkdown
		finalBuild.SummaryMarkdown = fmt.Sprintf("FATAL: %s", finalErrs.Error())
		if originalSM != "" {
			finalBuild.SummaryMarkdown += "\n\n" + originalSM
		}
	}

	return len(finalErrs) == 0
}

func finalizeBuild(ctx context.Context, bbclient BuildsClient, finalBuild *bbpb.Build, updateMask []string) int {
	var retcode int
	// No need to check the returned build here because it's already finalizing
	// the build.
	_, bbErr := bbclient.UpdateBuild(
		ctx,
		&bbpb.UpdateBuildRequest{
			Build:      finalBuild,
			UpdateMask: &fieldmaskpb.FieldMask{Paths: updateMask},
		})
	if bbErr != nil {
		logging.Errorf(ctx, "Failed to update Buildbucket due to %s", bbErr)
		retcode = 2
	}
	return retcode
}

func mainImpl() int {
	ctx := logging.SetLevel(gologger.StdConfig.Use(context.Background()), logging.Info)

	hostname := flag.String("host", "", "Buildbucket server hostname")
	buildID := flag.Int64("build-id", 0, "Buildbucket build ID")
	useGCEAccount := flag.Bool("use-gce-account", false, "Use GCE metadata service account for all calls")
	cacheBase := flag.String("cache-base", "", "Directory where all the named caches are mounted for the build")
	contextFile := flag.String("context-file", "", "Path to the BbagentContext file. Must be a .json file.")
	outputFile := luciexe.AddOutputFlagToSet(flag.CommandLine)

	flag.Parse()
	args := flag.Args()

	if *useGCEAccount {
		ctx = setLocalAuth(ctx)
	}

	var bbclientInput clientInput
	var err error

	bbagentCtx, err := getBuildbucketAgentContext(ctx, *contextFile)
	if err != nil {
		check(ctx, errors.Annotate(err, "BbagentContext could not be created").Err())
	}

	switch {
	case len(args) == 1:
		bbclientInput = parseBbAgentArgs(ctx, args[0], bbagentCtx)
	case *hostname != "" && *buildID > 0:
		bbclientInput = parseHostBuildID(ctx, hostname, buildID, bbagentCtx)
	default:
		check(ctx, errors.Reason("-host and -build-id are required").Err())
	}

	// The build has been canceled, bail out early.
	if bbclientInput.input.Build.CancelTime != nil {
		return cancelBuild(ctx, bbclientInput.bbclient, bbclientInput.input.Build)
	}

	// Manipulate the context and obtain a context with cancel
	ctx = setBuildbucketContext(ctx, hostname, bbagentCtx.Secrets)
	ctx = setRealmContext(ctx, bbclientInput.input)

	logdogOutput, err := mkLogdogOutput(ctx, bbclientInput.input.Build.Infra.Logdog)
	check(ctx, errors.Annotate(err, "could not create logdog output").Err())

	var (
		cctx   context.Context
		cancel func()
	)
	if dl := lucictx.GetDeadline(ctx); dl.GetSoftDeadline() != 0 {
		softDeadline := dl.SoftDeadlineTime()
		gracePeriod := time.Duration(dl.GetGracePeriod() * float64(time.Second))
		cctx, cancel = context.WithDeadline(ctx, softDeadline.Add(gracePeriod))
	} else {
		cctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	var updatedBuild *bbpb.Build
	if bbagentCtx.TaskId == "" {
		// We send a single status=STARTED here, and will send the final build status
		// after the user executable completes.
		// TODO(crbug.com/1416971): remove this UpdateBuild call, after it's fully
		// enabled for bbagent to call StartBuild.
		updatedBuild, err = bbclientInput.bbclient.UpdateBuild(
			cctx,
			&bbpb.UpdateBuildRequest{
				Build: &bbpb.Build{
					Id: bbclientInput.input.Build.Id,
					Output: &bbpb.Build_Output{
						Status: bbpb.Status_STARTED,
					},
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"build.output.status"}},
				Mask:       readMask,
			})
		check(ctx, errors.Annotate(err, "failed to report status STARTED to Buildbucket").Err())

		// The build has been canceled, bail out early.
		if updatedBuild.CancelTime != nil {
			return cancelBuild(ctx, bbclientInput.bbclient, updatedBuild)
		}
	}

	prepareInputBuild(cctx, bbclientInput.input.Build, updatedBuild)

	cctx = setResultDBContext(cctx, bbclientInput.input.Build, bbagentCtx.Secrets)

	// TODO(crbug.com/1211789) - As part of adding 'dry_run' functionality
	// to ScheduleBuild, it was necessary to start saving `tags` in the
	// Build message (previously they were ephemeral in the datastore model).
	// This had the side effect that setting host.Options.BaseBuild = input.Build
	// would cause bbagent to regurgitate input tags back to Buildbucket.
	//
	// Normally this would be fine (Buildbucket would deduplicate them),
	// except in the case of some special tags (like "buildset").
	//
	// We strip the input tags here just for host.Options.BaseBuild to
	// avoid this scenario; however it has the side effect that led tasks
	// which are scheduled directly on Swarming will not show the tags on
	// the Milo UI. When led jobs eventually become "real" buildbucket jobs
	// this discrepancy would go away (and it may also make sense to remove
	// BaseBuild from host.Options, since it really only needs to carry the
	// user-code-generated-delta at that point).
	hostOptionsBaseBuild := proto.Clone(bbclientInput.input.Build).(*bbpb.Build)
	hostOptionsBaseBuild.Tags = nil

	opts := &host.Options{
		BaseBuild:      hostOptionsBaseBuild,
		ButlerLogLevel: logging.Warning,
		// TODO(crbug.com/1219086) - generate a correct URL for LED tasks.
		ViewerURL: fmt.Sprintf("https://%s/build/%d",
			bbclientInput.input.Build.Infra.Buildbucket.Hostname, bbclientInput.input.Build.Id),
		LogdogOutput: logdogOutput,
		ExeAuth:      host.DefaultExeAuth("bbagent", bbclientInput.input.KnownPublicGerritHosts),
	}
	cwd, err := os.Getwd()
	checkReport(ctx, bbclientInput, errors.Annotate(err, "getting cwd").Err())
	opts.BaseDir = filepath.Join(cwd, "x")

	initialJSONPB, err := (&jsonpb.Marshaler{
		OrigName: true, Indent: "  ",
	}).MarshalToString(bbclientInput.input)
	checkReport(ctx, bbclientInput, errors.Annotate(err, "marshalling input args").Err())
	logging.Infof(ctx, "Input args:\n%s", initialJSONPB)

	// Downloading cipd packages if any.
	resolvedCacheBase := chooseCacheDir(bbclientInput.input, *cacheBase)
	if stringset.NewFromSlice(bbclientInput.input.Build.Input.Experiments...).Has(buildbucket.ExperimentBBAgentDownloadCipd) {
		if retcode := downloadInputs(cctx, cwd, resolvedCacheBase, bbclientInput); retcode != 0 {
			return retcode
		}
	}

	if backFillTaskInfoNeeded(bbclientInput) {
		if retcode := backFillTaskInfo(ctx, bbclientInput); retcode != 0 {
			return retcode
		}
	}

	exeArgs := processExeArgs(ctx, bbclientInput)
	dispatcherOpts, dispatcherErrCh := channelOpts(cctx)
	canceledBuildCh := newCloseOnceCh()
	invokeErr := make(chan error)
	// Use a dedicated BuildsClient for dispatcher, which turns off retries.
	// dispatcher.Channel will handle retries instead.
	// Create the build client with a provided secrets, because secrets could have
	// been updated with a build token from the response of the StartBuild call.
	bbclientForDispatcher, err := newBuildsClientWithSecrets(
		cctx, bbclientInput.input.Build.Infra.Buildbucket.GetHostname(), func() retry.Iterator { return nil }, bbagentCtx.Secrets)
	if err != nil {
		checkReport(ctx, bbclientInput, errors.Annotate(err, "could not connect to Buildbucket").Err())
	}
	buildsCh, err := dispatcher.NewChannel[*bbpb.Build](
		cctx, dispatcherOpts, mkSendFn(cctx, bbclientForDispatcher, bbclientInput.input.Build.Id, canceledBuildCh))
	checkReport(ctx, bbclientInput, errors.Annotate(err, "could not create builds dispatcher channel").Err())
	defer buildsCh.CloseAndDrain(cctx)

	shutdownCh := newCloseOnceCh()
	var statusDetails *bbpb.StatusDetails
	var subprocErr error
	builds, err := host.Run(cctx, opts, func(ctx context.Context, hostOpts host.Options, deadlineEvntCh <-chan lucictx.DeadlineEvent, shutdown func()) {
		logging.Infof(ctx, "running luciexe: %q", exeArgs)
		logging.Infof(ctx, "  (cache dir): %q", resolvedCacheBase)
		invokeOpts := &invoke.Options{
			BaseDir:  hostOpts.BaseDir,
			CacheDir: resolvedCacheBase,
			Env:      environ.System(),
		}

		go func() {
			select {
			case <-shutdownCh.ch:
				shutdown()
			case evt := <-deadlineEvntCh:
				// Got interrupt signal.
				// Try to send the cancel time back to Buildbucket at best effort.
				// So error of this update is only logged.
				if evt == lucictx.InterruptEvent {
					_, err := bbclientInput.bbclient.UpdateBuild(
						ctx,
						&bbpb.UpdateBuildRequest{
							Build: &bbpb.Build{
								Id:                   bbclientInput.input.Build.Id,
								CancelTime:           timestamppb.New(clock.Now(ctx)),
								CancellationMarkdown: "backend task is interrupted",
							},
							UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"build.cancel_time", "build.cancellation_markdown"}},
						})
					if err != nil {
						logging.Errorf(ctx, "failed to update the build's cancel_time: %s", err)
					}
					shutdown()
				}
			case <-ctx.Done():
			}
		}()
		defer close(invokeErr)

		subp, err := invoke.Start(ctx, exeArgs, bbclientInput.input.Build, invokeOpts)
		if err != nil {
			invokeErr <- err
			return
		}

		var build *bbpb.Build
		build, subprocErr = subp.Wait()
		logging.Infof(ctx, fmt.Sprintf("Final build status from subprocess: %s", build.Status.String()))
		statusDetails = build.StatusDetails
	})
	if err != nil {
		checkReport(ctx, bbclientInput, errors.Annotate(err, "could not start luciexe host environment").Err())
	}

	var (
		finalBuild                *bbpb.Build = proto.Clone(bbclientInput.input.Build).(*bbpb.Build)
		fatalUpdateBuildErrorSlot atomic.Value
	)

	si := stopInfo{
		invokeErr,
		shutdownCh,
		canceledBuildCh,
		dispatcherErrCh,
	}

	go si.stopEvents(cctx, bbclientInput, &fatalUpdateBuildErrorSlot)

	// Now all we do is shuttle builds through to the buildbucket client channel
	// until there are no more builds to shuttle.
	for build := range builds {
		if fatalUpdateBuildErrorSlot.Load() == nil {
			buildsCh.C <- build
			finalBuild = build
		}
	}
	buildsCh.CloseAndDrain(cctx)

	// Now that the builds channel has been closed, update bb directly.
	updateMask := []string{
		"build.output.status",
		"build.output.status_details",
		"build.summary_markdown",
	}
	var retcode int

	fatalUpdateBuildErr, _ := fatalUpdateBuildErrorSlot.Load().(error)
	if readyToFinalize(cctx, finalBuild, fatalUpdateBuildErr, statusDetails, outputFile) {
		updateMask = append(updateMask, "build.steps", "build.output")
	} else {
		// readyToFinalize indicated that something is really wrong; Omit steps and
		// output from the final push to minimize potential issues.
		retcode = 1
	}

	retcode = finalizeBuild(cctx, bbclientInput.bbclient, finalBuild, updateMask)
	if retcode == 0 && subprocErr != nil {
		errors.Walk(subprocErr, func(err error) bool {
			logging.Infof(cctx, "Subprocess error: %s", err)
			exit, ok := err.(*exec.ExitError)
			if ok {
				retcode = exit.ExitCode()
				logging.Infof(cctx, "Returning exit code from user subprocess: %d", retcode)
			}
			return !ok
		})
		if retcode == 0 {
			retcode = 3
			logging.Errorf(cctx, "Non retcode-containing error from user subprocess: %s", subprocErr)
		}
	}
	return retcode
}

func main() {
	go func() {
		// serves "/debug" endpoints for pprof.
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	os.Exit(mainImpl())
}
