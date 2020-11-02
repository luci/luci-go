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
	"math"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
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

var maxReserveDuration = 2 * time.Minute

func main() {
	go func() {
		// serves "/debug" endpoints for pprof.
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	os.Exit(mainImpl())
}

func mainImpl() int {
	ctx := logging.SetLevel(gologger.StdConfig.Use(context.Background()), logging.Info)

	check := func(err error) {
		if err != nil {
			logging.Errorf(ctx, err.Error())
			os.Exit(1)
		}
	}

	outputFile := luciexe.AddOutputFlagToSet(flag.CommandLine)
	flag.Parse()
	args := flag.Args()

	if len(args) != 1 {
		check(errors.Reason("expected 1 argument: got %d", len(args)).Err())
	}

	input, err := bbinput.Parse(args[0])
	check(errors.Annotate(err, "could not unmarshal BBAgentArgs").Err())

	sctx, err := lucictx.SwitchLocalAccount(ctx, "system")
	check(errors.Annotate(err, "could not switch to 'system' account in LUCI_CONTEXT").Err())

	bbclient, secrets, err := newBuildsClient(sctx, input.Build.Infra.Buildbucket)
	check(errors.Annotate(err, "could not connect to Buildbucket").Err())
	logdogOutput, err := mkLogdogOutput(sctx, input.Build.Infra.Logdog)
	check(errors.Annotate(err, "could not create logdog output").Err())

	var (
		cctx   context.Context
		cancel func()
	)
	if dl := lucictx.GetDeadline(ctx); dl.GetSoftDeadline() != 0 {
		softDeadline := convertFloatToUnixTime(dl.GetSoftDeadline())
		gracePeriod := time.Duration(dl.GetGracePeriod() * float64(time.Second))
		cctx, cancel = context.WithDeadline(ctx, softDeadline.Add(gracePeriod))
	} else {
		cctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	dispatcherOpts, dispatcherErrCh := channelOpts(cctx)
	buildsCh, err := dispatcher.NewChannel(cctx, dispatcherOpts, mkSendFn(cctx, secrets, bbclient))
	check(errors.Annotate(err, "could not create builds dispatcher channel").Err())
	defer buildsCh.CloseAndDrain(cctx)

	// from this point forward we want to try to report errors to buildbucket,
	// too.
	check = func(err error) {
		if err != nil {
			logging.Errorf(cctx, err.Error())
			buildsCh.C <- &bbpb.Build{
				Status:          bbpb.Status_INFRA_FAILURE,
				SummaryMarkdown: fmt.Sprintf("fatal error in startup: %s", err),
			}
			buildsCh.CloseAndDrain(cctx)
			os.Exit(1)
		}
	}

	cctx = setResultDBContext(cctx, input.Build, secrets)
	prepareInputBuild(input.Build)

	opts := &host.Options{
		BaseBuild:      input.Build,
		ButlerLogLevel: logging.Warning,
		ViewerURL: fmt.Sprintf("https://%s/build/%d",
			input.Build.Infra.Buildbucket.Hostname, input.Build.Id),
		LogdogOutput: logdogOutput,
		ExeAuth:      host.DefaultExeAuth("bbagent", input.KnownPublicGerritHosts),
	}
	cwd, err := os.Getwd()
	check(errors.Annotate(err, "getting cwd").Err())
	opts.BaseDir = filepath.Join(cwd, "x")

	exeArgs := append(([]string)(nil), input.Build.Exe.Cmd...)
	payloadPath := input.PayloadPath
	if len(exeArgs) == 0 {
		// TODO(iannucci): delete me with ExecutablePath.
		var exe string
		payloadPath, exe = path.Split(input.ExecutablePath)
		exeArgs = []string{exe}
	}
	exePath, err := filepath.Abs(filepath.Join(payloadPath, exeArgs[0]))
	check(errors.Annotate(err, "absoluting exe path %q", input.ExecutablePath).Err())
	if runtime.GOOS == "windows" {
		exePath, err = resolveExe(exePath)
		check(errors.Annotate(err, "resolving %q", input.ExecutablePath).Err())
	}
	exeArgs[0] = exePath

	initialJSONPB, err := (&jsonpb.Marshaler{
		OrigName: true, Indent: "  ",
	}).MarshalToString(input)
	check(errors.Annotate(err, "marshalling input args").Err())
	logging.Infof(ctx, "Input args:\n%s", initialJSONPB)

	shutdownCh := make(chan struct{})
	builds, err := host.Run(cctx, opts, func(ctx context.Context, hostOpts host.Options) error {
		logging.Infof(ctx, "running luciexe: %q", exeArgs)
		logging.Infof(ctx, "  (cache dir): %q", input.CacheDir)
		invokeOpts := &invoke.Options{
			BaseDir:  hostOpts.BaseDir,
			CacheDir: input.CacheDir,
		}
		reserve := calcDeadlineReserve(ctx)
		deadlineEventCh, dctx, shutdown := lucictx.AdjustDeadline(ctx, reserve, 500*time.Millisecond)
		go func() {
			select {
			case <-shutdownCh:
				shutdown()
			case <-dctx.Done():
			}
		}()
		if newSoftDeadline := lucictx.GetDeadline(dctx).GetSoftDeadline(); newSoftDeadline != 0 {
			logging.Infof(dctx,
				"attempted to reserve %s from soft deadline for bbagent cleanup; new soft deadline: %s",
				reserve, convertFloatToUnixTime(newSoftDeadline))
		}
		subp, err := invoke.Start(dctx, exeArgs, input.Build, invokeOpts, deadlineEventCh)
		if err != nil {
			return err
		}
		_, err = subp.Wait()
		return err
	})
	if err != nil {
		check(errors.Annotate(err, "could not start luciexe host environment").Err())
	}

	var (
		finalBuild *bbpb.Build
		fatalErr   atomic.Value
	)

	go func() {
		// Monitors the `dispatcherErrCh` and checks for fatal error determined by
		// `fatalPred`. Stops the build shuttling and shuts down the luciexe if a
		// fatal error is received.
		stopped := false
		fatalPred := func(err error) bool {
			// TODO(crbug.com/1140612): Figure out a better solution to handle the
			// InvalidArgument error at the beginning of the build when build status
			// hasn't switched to STARTED yet.
			// Possible solution: keep calling GetBuild and start shuttling the build
			// once GetBuild returns STARTED status. For now, simply tolerate such
			// error.
			if grpcutil.Code(err) == codes.InvalidArgument && !strings.Contains(err.Error(), "cannot update steps of a SCHEDULED build") {
				return true
			}
			return false
		}
		for {
			select {
			case err := <-dispatcherErrCh:
				if !stopped && fatalPred(err) {
					close(shutdownCh)
					fatalErr.Store(err)
					stopped = true
				}
			case <-cctx.Done():
				return
			}
		}
	}()

	// Now all we do is shuttle builds through to the buildbucket client channel
	// until there are no more builds to shuttle.
	for build := range builds {
		if fatalErr.Load() == nil {
			buildsCh.C <- build
			finalBuild = build
		}
	}
	buildsCh.CloseAndDrain(cctx)

	// Now that the builds channel has been closed now. Update to bb directly.
	reportErrToBB := func(err error) {
		errors.Log(cctx, err)
		now := timestamppb.New(clock.Now(cctx))
		_, bbErr := bbclient.UpdateBuild(
			metadata.NewOutgoingContext(cctx, metadata.Pairs(buildbucket.BuildTokenHeader, secrets.BuildToken)),
			&bbpb.UpdateBuildRequest{
				Build: &bbpb.Build{
					Id:              input.Build.Id,
					Builder:         input.Build.Builder,
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: err.Error(),
					UpdateTime:      now,
					EndTime:         now,
				},
				UpdateMask: &fieldmaskpb.FieldMask{
					Paths: []string{
						"build.status",
						"build.summary_markdown",
					},
				},
			})
		if bbErr != nil {
			logging.Errorf(cctx, "Failed to report error %s to Buildbucket due to %s", err, bbErr)
		}
	}

	if err := fatalErr.Load(); err != nil {
		reportErrToBB(errors.Annotate(err.(error), "encounter fatal error while updating build to buildbucket").Err())
		return 1
	}

	if err := outputFile.Write(finalBuild); err != nil {
		reportErrToBB(errors.Annotate(err, "writing final build").Err())
		return 1
	}

	if finalBuild.Status != bbpb.Status_SUCCESS {
		return 1
	}
	return 0
}

func prepareInputBuild(build *bbpb.Build) {
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

// Returns min(1% of the remaining time towards current soft deadline,
// `maxReserveDuration`). Returns 0 If soft deadline doesn't exist or
// has already been exceeded.
func calcDeadlineReserve(ctx context.Context) time.Duration {
	curSoftDeadline := convertFloatToUnixTime(lucictx.GetDeadline(ctx).GetSoftDeadline())
	if now := clock.Now(ctx).UTC(); !curSoftDeadline.IsZero() && now.Before(curSoftDeadline) {
		if reserve := curSoftDeadline.Sub(now) / 100; reserve < maxReserveDuration {
			return reserve
		}
		return maxReserveDuration
	}
	return 0
}

func convertFloatToUnixTime(f float64) time.Time {
	if f == 0 {
		return time.Time{}
	}
	sec, frac := math.Modf(f)
	return time.Unix(int64(sec), int64(frac*float64(time.Second))).UTC()
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
