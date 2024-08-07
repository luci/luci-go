// Copyright 2020 The LUCI Authors.
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

package ledcmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/buildbucket"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/googleoauth"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/logdog/client/butler/output/directory"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/luciexe/host"
	"go.chromium.org/luci/luciexe/invoke"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/job/jobexport"
)

// UserAgentTag is added by default to all Swarming tasks launched by LED.
const UserAgentTag = "user_agent:led"

// LaunchSwarmingOpts are the options for LaunchSwarming.
type LaunchSwarmingOpts struct {
	// If true, just generates the NewTaskRequest but does not send it to swarming
	// (SwarmingRpcsTaskRequestMetadata will be nil).
	DryRun bool

	// If true, lauch task with local luciexe host instead of sending to
	// swarming. This can be used for testing locally.
	Local bool

	// A path to be leaked when running task locally. Only be effective if Local
	// is true.
	LeakDir string

	// Must be a unique user identity string and must not be empty.
	//
	// Picking a bad value here means that generated logdog prefixes will
	// possibly collide, and the swarming task's User field will be misreported.
	//
	// See GetUID to obtain a standardized value here.
	UserID string

	// If launched from within a swarming task, this will be the current swarming
	// task's task id to be attached as the parent of the launched task.
	ParentTaskId string

	// A path, relative to ${ISOLATED_OUTDIR} of where to place the final
	// build.proto from this build. If omitted, the build.proto will not be
	// dumped.
	FinalBuildProto string

	KitchenSupport job.KitchenSupport

	// A flag for swarming/ResultDB integration on the launched task.
	ResultDB job.RDBEnablement

	// If true, `user_agent:led` tag will not be added to the launched task tags,
	// which is otherwise added by default.
	NoLEDTag bool

	// If the launched real Buildbucket build can outlive its parent or not.
	// Only works in the real build mode.
	CanOutliveParent bool
}

// GetUID derives a user id string from the Authenticator for use with
// LaunchSwarming.
//
// If the given authenticator has the userinfo.email scope, this will be the
// email associated with the Authenticator. Otherwise, this will be
// 'uid:<opaque user id>'.
func GetUID(ctx context.Context, authenticator *auth.Authenticator) (string, error) {
	tok, err := authenticator.GetAccessToken(time.Minute)
	if err != nil {
		return "", errors.Annotate(err, "getting access token").Err()
	}
	info, err := googleoauth.GetTokenInfo(ctx, googleoauth.TokenInfoParams{
		AccessToken: tok.AccessToken,
	})
	if err != nil {
		return "", errors.Annotate(err, "getting access token info").Err()
	}
	if info.Email != "" {
		return info.Email, nil
	}
	return "uid:" + info.Sub, nil
}

// LaunchSwarming launches the given job Definition on swarming, returning the
// NewTaskRequest launched, as well as the launch metadata.
func LaunchSwarming(ctx context.Context, authClient *http.Client, jd *job.Definition, opts LaunchSwarmingOpts) (*swarmingpb.NewTaskRequest, *swarmingpb.TaskRequestMetadataResponse, error) {
	if opts.Local {
		return nil, nil, errors.New("local swarming task not supported. '--local' must be used with a buildbucket job.")
	}
	if opts.KitchenSupport == nil {
		opts.KitchenSupport = job.NoKitchenSupport()
	}
	if opts.UserID == "" {
		return nil, nil, errors.New("opts.UserID is empty")
	}

	logging.Infof(ctx, "building swarming task")
	if err := jd.FlattenToSwarming(ctx, opts.UserID, opts.ParentTaskId, opts.KitchenSupport, opts.ResultDB); err != nil {
		return nil, nil, errors.Annotate(err, "failed to flatten job definition to swarming").Err()
	}

	st, err := jobexport.ToSwarmingNewTask(jd.GetSwarming())
	if err != nil {
		return nil, nil, err
	}
	if !opts.NoLEDTag {
		addUserAgentTag(st)
	}
	logging.Infof(ctx, "building swarming task: done")

	if opts.DryRun {
		return st, nil, nil
	}

	swarmTasksClient := newSwarmTasksClient(authClient, jd.Info().SwarmingHostname())

	logging.Infof(ctx, "launching swarming task")
	resp, err := swarmTasksClient.NewTask(ctx, st)
	if err != nil {
		return nil, nil, err
	}
	logging.Infof(ctx, "launching swarming task: done")

	return st, resp, nil
}

func addUserAgentTag(req *swarmingpb.NewTaskRequest) {
	for _, t := range req.Tags {
		if t == UserAgentTag {
			return
		}
	}
	req.Tags = append(req.Tags, UserAgentTag)
}

// LaunchBuild creates a real Buildbucket build based on the given job Definition.
func LaunchBuild(ctx context.Context, authClient *http.Client, jd *job.Definition, opts LaunchSwarmingOpts) (*bbpb.Build, error) {
	if jd.GetBuildbucket() == nil {
		return nil, nil
	}
	bb := jd.GetBuildbucket()
	build := bb.GetBbagentArgs().GetBuild()
	build.CanOutliveParent = opts.CanOutliveParent
	err := bb.UpdateLedProperties()
	if err != nil {
		return nil, err
	}

	// Attach user tag.
	tags := build.Tags
	tags = append(tags, &bbpb.StringPair{
		Key:   "user",
		Value: opts.UserID,
	})
	sort.Slice(tags, func(i, j int) bool { return tags[i].Key < tags[j].Key })
	build.Tags = tags

	// TODO(b/338121346): Seperate dryrun, local, and remote.
	if opts.DryRun {
		return build, nil
	} else if opts.Local {
		baseDir := opts.LeakDir
		if baseDir == "" {
			if baseDir, err = os.MkdirTemp("", "led-"); err != nil {
				return nil, err
			}
			defer filesystem.RemoveAll(baseDir)
		}

		return launchLocal(ctx, build, baseDir)
	}

	bbClient := newBuildbucketClient(authClient, build.GetInfra().GetBuildbucket().GetHostname())

	bbCtx := lucictx.GetBuildbucket(ctx)
	if bbCtx != nil && bbCtx.GetScheduleBuildToken() != "" && bbCtx.GetScheduleBuildToken() != buildbucket.DummyBuildbucketToken {
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, bbCtx.ScheduleBuildToken))
	} else {
		build.CanOutliveParent = false
	}
	return bbClient.CreateBuild(ctx, &bbpb.CreateBuildRequest{
		Build: build,
	})
}

func launchLocal(ctx context.Context, build *bbpb.Build, baseDir string) (*bbpb.Build, error) {
	// TODO(b/338121346): We should warn user if the task launching on a
	// different platform.

	luciexeOpts := &host.Options{
		BaseDir:             baseDir,
		BaseBuild:           build,
		ButlerLogLevel:      logging.Warning,
		LogdogOutput:        directory.Options{Path: filepath.Join(baseDir, "log")}.New(ctx),
		ExeAuth:             host.DefaultExeAuth("led", nil),
		DownloadAgentInputs: true,
	}

	var runErr error
	finalBuild := build

	builds, err := host.Run(ctx, luciexeOpts, func(ctx context.Context, hostOpts host.Options, deadlineEvntCh <-chan lucictx.DeadlineEvent, shutdown func()) {
		logging.Infof(ctx, "running luciexe: %q", build.Exe.Cmd)
		logging.Infof(ctx, "  (cache dir): %q", hostOpts.CacheDir)
		invokeOpts := &invoke.Options{
			BaseDir:  hostOpts.BaseDir,
			CacheDir: hostOpts.CacheDir,
			Env:      environ.System(),
		}

		go func() {
			if evt := <-deadlineEvntCh; evt == lucictx.InterruptEvent {
				shutdown()
				logging.Infof(ctx, "interrupt event recieved")
			}
		}()

		exeArgs, err := host.ResolveExeCmd(&hostOpts, "")
		if err != nil {
			runErr = err
			return
		}
		subp, err := invoke.Start(ctx, exeArgs, build, invokeOpts)
		if err != nil {
			runErr = err
			return
		}

		var build *bbpb.Build
		build, err = subp.Wait()
		if err != nil {
			runErr = err
			return
		}
		logging.Infof(ctx, fmt.Sprintf("Final build status from subprocess: %s", build.Status.String()))
	})

	// TODO(b/338121346): run a local webserver and display the build state in a
	// browser (or any other ways).
	for build := range builds {
		finalBuild = build
	}

	return finalBuild, errors.Join(err, runErr)
}
