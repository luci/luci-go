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
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/authctx"
	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/cipd/client/cipd/platform"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/luciexe"
	"go.chromium.org/luci/luciexe/host"
	"go.chromium.org/luci/luciexe/invoke"
)

func main() {
	go func() {
		// serves "/debug" endpoints for pprof.
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	os.Exit(mainImpl())
}

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

func mainImpl() int {
	ctx := logging.SetLevel(gologger.StdConfig.Use(context.Background()), logging.Info)

	check := func(err error) {
		if err != nil {
			logging.Errorf(ctx, err.Error())
			os.Exit(1)
		}
	}

	hostname := flag.String("host", "", "Buildbucket server hostname")
	buildID := flag.Int64("build-id", 0, "Buildbucket build ID")
	useGCEAccount := flag.Bool("use-gce-account", false, "Use GCE metadata service account for all calls")
	outputFile := luciexe.AddOutputFlagToSet(flag.CommandLine)

	flag.Parse()
	args := flag.Args()

	if *useGCEAccount {
		// If asked to use the GCE account, create a new local auth context so it
		// can be properly picked through out the rest of bbagent process tree. Use
		// it as the default task account and as a "system" account (so it is used
		// for things like Logdog PubSub calls).
		authCtx := authctx.Context{
			ID:                  "bbagent",
			Options:             auth.Options{Method: auth.GCEMetadataMethod},
			ExposeSystemAccount: true,
		}
		err := authCtx.Launch(ctx, "")
		check(errors.Annotate(err, "failed launch the local LUCI auth context").Err())
		defer authCtx.Close(ctx)

		// Switch the default auth in the context to the one we just setup.
		ctx = authCtx.SetLocalAuth(ctx)
	}

	var input *bbpb.BBAgentArgs
	var bbclient BuildsClient
	var secrets *bbpb.BuildSecrets
	var err error

	// bbclientRetriesEnabled is a passed-py-pointer value which we use to turn
	// retries on and off during the build.
	//
	// We start with retries enabled to send the initial status==STARTED update,
	// but then disable retries for the main build updates because the
	// dispatcher.Channel will handle them during the execution of the user
	// process; In particular we want dispatcher.Channel to be able to move on to
	// a newer version of the Build if it encounters transient errors, rather than
	// retrying a potentially stale Build state.
	//
	// We enable them again after the user process has finished.
	bbclientRetriesEnabled := true

	switch {
	case len(args) == 1:
		// TODO(crbug/1219018): Remove CLI BBAgentArgs mode in favor of -host + -build-id.
		logging.Debugf(ctx, "parsing BBAgentArgs")
		input, err = bbinput.Parse(args[0])
		check(errors.Annotate(err, "could not unmarshal BBAgentArgs").Err())
		bbclient, secrets, err = newBuildsClient(ctx, input.Build.Infra.Buildbucket.GetHostname(), &bbclientRetriesEnabled)
		check(errors.Annotate(err, "could not connect to Buildbucket").Err())
	case *hostname != "" && *buildID > 0:
		logging.Debugf(ctx, "fetching build %d", *buildID)
		bbclient, secrets, err = newBuildsClient(ctx, *hostname, &bbclientRetriesEnabled)
		check(errors.Annotate(err, "could not connect to Buildbucket").Err())
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
		build, err := bbclient.UpdateBuild(ctx, &bbpb.UpdateBuildRequest{
			Build: &bbpb.Build{
				Id: *buildID,
			},
			Mask: &bbpb.BuildMask{
				AllFields: true,
			},
		})
		check(errors.Annotate(err, "failed to fetch build").Err())
		input = &bbpb.BBAgentArgs{
			Build:                  build,
			CacheDir:               build.Infra.Bbagent.CacheDir,
			KnownPublicGerritHosts: build.Infra.Bbagent.KnownPublicGerritHosts,
			PayloadPath:            build.Infra.Bbagent.PayloadPath,
		}
	default:
		check(errors.Reason("-host and -build-id are required").Err())
	}

	if protoutil.FormatBuilderID(input.Build.Builder) == "chromium/ci/mac-rel-swarming" {
		ctx = logging.SetLevel(gologger.StdConfig.Use(context.Background()), logging.Debug)
	}
	// Set `buildbucket` in the context.
	bbCtx := lucictx.GetBuildbucket(ctx)
	if bbCtx == nil || bbCtx.Hostname != *hostname || bbCtx.ScheduleBuildToken != secrets.BuildToken {
		ctx = lucictx.SetBuildbucket(ctx, &lucictx.Buildbucket{
			Hostname:           *hostname,
			ScheduleBuildToken: secrets.BuildToken,
		})
		if bbCtx != nil {
			logging.Warningf(ctx, "buildbucket context is overwritten.")
		}
	}

	// Populate `realm` in the context based on the build's bucket if there's no
	// realm there already.
	if lucictx.GetRealm(ctx).GetName() == "" {
		project := input.Build.Builder.Project
		bucket := input.Build.Builder.Bucket
		if project != "" && bucket != "" {
			ctx = lucictx.SetRealm(ctx, &lucictx.Realm{
				Name: fmt.Sprintf("%s:%s", project, bucket),
			})
		} else {
			logging.Warningf(ctx, "Bad BuilderID in the build proto: %s", input.Build.Builder)
		}
	}

	logdogOutput, err := mkLogdogOutput(ctx, input.Build.Infra.Logdog)
	check(errors.Annotate(err, "could not create logdog output").Err())

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
	// We send a single status=STARTED here, and will send the final build status
	// after the user executable completes.
	updatedBuild, err = bbclient.UpdateBuild(
		cctx,
		&bbpb.UpdateBuildRequest{
			Build: &bbpb.Build{
				Id:     input.Build.Id,
				Status: bbpb.Status_STARTED,
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"build.status"}},
			Mask:       readMask,
		})
	if err != nil {
		check(errors.Annotate(err, "failed to report status STARTED to Buildbucket").Err())
	}
	// The build has been canceled, bail out early.
	if updatedBuild.CancelTime != nil {
		logging.Infof(ctx, "The build is in the cancel process, cancel time is %s.", updatedBuild.CancelTime.AsTime().String())
		return 0
	}

	// from this point forward we want to try to report errors to buildbucket,
	// too.
	check = func(err error) {
		if err != nil {
			logging.Errorf(cctx, err.Error())
			if _, bbErr := bbclient.UpdateBuild(
				cctx,
				&bbpb.UpdateBuildRequest{
					Build: &bbpb.Build{
						Id:              input.Build.Id,
						Status:          bbpb.Status_INFRA_FAILURE,
						SummaryMarkdown: fmt.Sprintf("fatal error in startup: %s", err),
					},
					UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"build.status", "build.summary_markdown"}},
				}); bbErr != nil {
				logging.Errorf(cctx, "Failed to report INFRA_FAILURE status to Buildbucket: %s", bbErr)
			}
			os.Exit(1)
		}
	}

	cctx = setResultDBContext(cctx, input.Build, secrets)
	prepareInputBuild(cctx, input.Build)

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
	hostOptionsBaseBuild := proto.Clone(input.Build).(*bbpb.Build)
	hostOptionsBaseBuild.Tags = nil

	opts := &host.Options{
		BaseBuild:      hostOptionsBaseBuild,
		ButlerLogLevel: logging.Warning,
		// TODO(crbug.com/1219086) - generate a correct URL for LED tasks.
		ViewerURL: fmt.Sprintf("https://%s/build/%d",
			input.Build.Infra.Buildbucket.Hostname, input.Build.Id),
		LogdogOutput: logdogOutput,
		ExeAuth:      host.DefaultExeAuth("bbagent", input.KnownPublicGerritHosts),
	}
	cwd, err := os.Getwd()
	check(errors.Annotate(err, "getting cwd").Err())
	opts.BaseDir = filepath.Join(cwd, "x")

	initialJSONPB, err := (&jsonpb.Marshaler{
		OrigName: true, Indent: "  ",
	}).MarshalToString(input)
	check(errors.Annotate(err, "marshalling input args").Err())
	logging.Infof(ctx, "Input args:\n%s", initialJSONPB)

	// Downloading cipd packages if any.
	if stringset.NewFromSlice(input.Build.Input.Experiments...).Has(buildbucket.ExperimentBBAgentDownloadCipd) {
		// Most likely happens in `led get-build` process where it creates from an old build
		// before new Agent field was there. This new feature shouldn't work for those builds.
		if input.Build.Infra.Buildbucket.Agent == nil {
			check(errors.New("Cannot enable downloading cipd pkgs feature; Build Agent field is not set"))
		}

		bldForCipd := &bbpb.UpdateBuildRequest{
			Build: &bbpb.Build{
				Id: input.Build.Id,
				Infra: &bbpb.BuildInfra{
					Buildbucket: &bbpb.BuildInfra_Buildbucket{
						Agent: &bbpb.BuildInfra_Buildbucket_Agent{
							Output: &bbpb.BuildInfra_Buildbucket_Agent_Output{Status: bbpb.Status_STARTED},
						},
					},
				},
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"build.infra.buildbucket.agent.output"}},
		}
		if _, err := bbclient.UpdateBuild(cctx, bldForCipd); err != nil {
			// Carry on and bear the non-fatal update failure.
			logging.Warningf(ctx, "Failed to report build agent STARTED status: %s", err)
		}

		agentOutput := bldForCipd.Build.Infra.Buildbucket.Agent.Output
		agentOutput.AgentPlatform = platform.CurrentPlatform()
		start := clock.Now(ctx)
		resolved, err := installCipdPackages(ctx, input.Build, cwd)
		if err != nil {
			logging.Errorf(ctx, "Failure in installing cipd packages: %s", err)
			agentOutput.Status = bbpb.Status_FAILURE
			agentOutput.SummaryHtml = err.Error()
			bldForCipd.Build.Status = bbpb.Status_INFRA_FAILURE
			bldForCipd.Build.SummaryMarkdown = "Failed to install cipd packages for this build"
			bldForCipd.UpdateMask.Paths = append(bldForCipd.UpdateMask.Paths, "build.status", "build.summary_markdown")
		} else {
			agentOutput.ResolvedData = resolved
			agentOutput.Status = bbpb.Status_SUCCESS
		}
		agentOutput.TotalDuration = &durationpb.Duration{
			Seconds: int64(clock.Since(ctx, start).Round(time.Second).Seconds()),
		}
		if _, bbErr := bbclient.UpdateBuild(cctx, bldForCipd); bbErr != nil {
			logging.Warningf(ctx, "Failed to report build agent output status: %s", err)
		}
		if err != nil {
			os.Exit(1)
		}
		input.Build.Infra.Buildbucket.Agent.Output = bldForCipd.Build.Infra.Buildbucket.Agent.Output
	}

	exeArgs := processExeArgs(input, check)
	dispatcherOpts, dispatcherErrCh := channelOpts(cctx)
	canceledBuildCh := newCloseOnceCh()
	buildsCh, err := dispatcher.NewChannel(cctx, dispatcherOpts, mkSendFn(cctx, bbclient, input.Build.Id, canceledBuildCh))
	check(errors.Annotate(err, "could not create builds dispatcher channel").Err())
	defer buildsCh.CloseAndDrain(cctx)

	bbclientRetriesEnabled = false // dispatcher.Channel will handle retries.
	shutdownCh := newCloseOnceCh()
	var statusDetails *bbpb.StatusDetails
	var subprocErr error
	builds, err := host.Run(cctx, opts, func(ctx context.Context, hostOpts host.Options) error {
		logging.Infof(ctx, "running luciexe: %q", exeArgs)
		logging.Infof(ctx, "  (cache dir): %q", input.CacheDir)
		invokeOpts := &invoke.Options{
			BaseDir:  hostOpts.BaseDir,
			CacheDir: input.CacheDir,
			Env:      environ.System(),
		}
		if stringset.NewFromSlice(input.Build.Input.Experiments...).Has("luci.recipes.use_python3") {
			invokeOpts.Env.Set("RECIPES_USE_PY3", "true")
		}
		logging.Debugf(ctx, "PATH env is %s", invokeOpts.Env.String())
		// Buildbucket assigns some grace period to the surrounding task which is
		// more than what the user requested in `input.Build.GracePeriod`. We
		// reserve the difference here so the user task only gets what they asked
		// for.
		deadline := lucictx.GetDeadline(ctx)
		toReserve := deadline.GracePeriodDuration() - input.Build.GracePeriod.AsDuration()
		logging.Infof(
			ctx, "Reserving %s out of %s of grace_period from LUCI_CONTEXT.",
			toReserve, lucictx.GetDeadline(ctx).GracePeriodDuration())
		dctx, shutdown := lucictx.TrackSoftDeadline(ctx, toReserve)
		go func() {
			select {
			case <-shutdownCh.ch:
				shutdown()
			case <-dctx.Done():
			}
		}()
		subp, err := invoke.Start(dctx, exeArgs, input.Build, invokeOpts)
		if err != nil {
			return err
		}

		var build *bbpb.Build
		build, subprocErr = subp.Wait()
		statusDetails = build.StatusDetails
		return nil
	})
	if err != nil {
		check(errors.Annotate(err, "could not start luciexe host environment").Err())
	}

	var (
		finalBuild                *bbpb.Build = proto.Clone(input.Build).(*bbpb.Build)
		fatalUpdateBuildErrorSlot atomic.Value
	)

	go func() {
		// Monitors events that cause the build to stop.
		// * Monitors the `dispatcherErrCh` and checks for fatal error.
		//   * Stops the build shuttling and shuts down the luciexe if a fatal error
		//     is received.
		// * Monitors the returned build from UpdateBuild rpcs and checks if the
		//   build has been canceled.
		//   * Shuts down the luciexe if the build is canceled.
		stopped := false
		for {
			select {
			case err := <-dispatcherErrCh:
				if !stopped && grpcutil.Code(err) == codes.InvalidArgument {
					shutdownCh.close()
					fatalUpdateBuildErrorSlot.Store(err)
					stopped = true
				}
			case <-canceledBuildCh.ch:
				// The build has been canceled, bail out early.
				shutdownCh.close()
			case <-cctx.Done():
				return
			}
		}
	}()

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
		"build.status",
		"build.status_details",
		"build.summary_markdown",
	}
	var retcode int

	fatalUpdateBuildErr, _ := fatalUpdateBuildErrorSlot.Load().(error)
	if finalizeBuild(ctx, finalBuild, fatalUpdateBuildErr, statusDetails, outputFile) {
		updateMask = append(updateMask, "build.steps", "build.output")
	} else {
		// finalizeBuild indicated that something is really wrong; Omit steps and
		// output from the final push to minimize potential issues.
		retcode = 1
	}
	// No need to check the returned build here because it's already finalizing
	// the build.
	bbclientRetriesEnabled = true
	_, bbErr := bbclient.UpdateBuild(
		cctx,
		&bbpb.UpdateBuildRequest{
			Build:      finalBuild,
			UpdateMask: &fieldmaskpb.FieldMask{Paths: updateMask},
		})
	if bbErr != nil {
		logging.Errorf(cctx, "Failed to report error %s to Buildbucket due to %s", err, bbErr)
		retcode = 2
	}

	if retcode == 0 && subprocErr != nil {
		errors.Walk(subprocErr, func(err error) bool {
			if protoutil.FormatBuilderID(input.Build.Builder) == "chromium/ci/mac-rel-swarming" {
				logging.Infof(cctx, "subprocErr: %s", err)
				return true
			}
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

// finalizeBuild returns true if fatalErr is nil and there's no additional
// errors finalizing the build.
func finalizeBuild(ctx context.Context, finalBuild *bbpb.Build, fatalErr error, statusDetails *bbpb.StatusDetails, outputFile *luciexe.OutputFlag) bool {
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

func prepareInputBuild(ctx context.Context, build *bbpb.Build) {
	// mark started
	build.Status = bbpb.Status_STARTED
	now := timestamppb.New(clock.Now(ctx))
	build.StartTime, build.UpdateTime = now, now
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
	}
	populateSwarmingInfoFromEnv(build, environ.System())
	return
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

// processExeArgs processes the given "Executable" message into a single command
// which bbagent will invoke as a luciexe.
//
// This includes resolving paths relative to the current working directory
// (expected to be the task's root).
func processExeArgs(input *bbpb.BBAgentArgs, check func(err error)) []string {
	exeArgs := make([]string, 0, len(input.Build.Exe.Wrapper)+len(input.Build.Exe.Cmd)+1)

	if len(input.Build.Exe.Wrapper) != 0 {
		exeArgs = append(exeArgs, input.Build.Exe.Wrapper...)
		exeArgs = append(exeArgs, "--")

		if strings.Contains(exeArgs[0], "/") || strings.Contains(exeArgs[0], "\\") {
			absPath, err := filepath.Abs(exeArgs[0])
			check(errors.Annotate(err, "absoluting wrapper path: %q", exeArgs[0]).Err())
			exeArgs[0] = absPath
		}

		cmdPath, err := exec.LookPath(exeArgs[0])
		check(errors.Annotate(err, "wrapper not found: %q", exeArgs[0]).Err())
		exeArgs[0] = cmdPath
	}

	exeCmd := input.Build.Exe.Cmd[0]
	payloadPath := input.PayloadPath
	if len(input.Build.Exe.Cmd) == 0 {
		// TODO(iannucci): delete me with ExecutablePath.
		payloadPath, exeCmd = path.Split(input.ExecutablePath)
	}
	exeRelPath := filepath.Join(payloadPath, exeCmd)
	exePath, err := filepath.Abs(exeRelPath)
	check(errors.Annotate(err, "absoluting exe path %q", exeRelPath).Err())
	if runtime.GOOS == "windows" {
		exePath, err = resolveExe(exePath)
		check(errors.Annotate(err, "resolving %q", exePath).Err())
	}
	exeArgs = append(exeArgs, exePath)
	exeArgs = append(exeArgs, input.Build.Exe.Cmd[1:]...)

	return exeArgs
}
