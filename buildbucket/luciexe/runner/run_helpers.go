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

package runner

import (
	"context"
	"fmt"
	"net/url"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/buildbucket/luciexe/runner/buildspy"
	"go.chromium.org/luci/buildbucket/luciexe/runner/runnerbutler"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/common/types"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func (r *runner) startLogDog(ctx context.Context, args *pb.RunnerArgs, systemAuth *auth.Authenticator, spy *buildspy.Spy) (*runnerbutler.Server, error) {
	globalLogTags := make(map[string]string, 4)
	globalLogTags["logdog.viewer_url"] = fmt.Sprintf("https://%s/build/%d", args.BuildbucketHost, args.Build.Id)

	// SWARMING_SERVER is the full URL: https://example.com
	// We want just the hostname.
	env := environ.System()
	if v, ok := env.Get("SWARMING_SERVER"); ok {
		if u, err := url.Parse(v); err == nil && u.Host != "" {
			globalLogTags["swarming.host"] = u.Host
		}
	}
	if v, ok := env.Get("SWARMING_TASK_ID"); ok {
		globalLogTags["swarming.run_id"] = v
	}
	if v, ok := env.Get("SWARMING_BOT_ID"); ok {
		globalLogTags["swarming.bot_id"] = v
	}

	logdogServ := &runnerbutler.Server{
		WorkDir:                    args.WorkDir,
		Authenticator:              systemAuth,
		CoordinatorHost:            args.LogdogHost,
		Project:                    types.ProjectName(args.Build.Builder.Project),
		Prefix:                     types.StreamName(fmt.Sprintf("buildbucket/%s/%d", args.BuildbucketHost, args.Build.Id)),
		LocalFile:                  r.localLogFile,
		GlobalTags:                 globalLogTags,
		StreamRegistrationCallback: spy.StreamRegistrationCallback,
	}
	return logdogServ, logdogServ.Start(ctx)
}

// processFinalBuild adjusts the final state of the build if needed.
func processFinalBuild(ctx context.Context, build *pb.Build) {
	if !protoutil.IsEnded(build.Status) {
		build.SummaryMarkdown = fmt.Sprintf(
			"Invalid final build state: "+
				"`expected a terminal build status, got %s`. "+
				"Marking as `INFRA_FAILURE`.", build.Status)
		build.Status = pb.Status_INFRA_FAILURE
	}

	nowTS, err := ptypes.TimestampProto(clock.Now(ctx))
	if err != nil {
		panic(err)
	}

	// Mark incomplete steps as canceled.
	for _, s := range build.Steps {
		if !protoutil.IsEnded(s.Status) {
			s.Status = pb.Status_CANCELED
			if s.SummaryMarkdown != "" {
				s.SummaryMarkdown += "\n"
			}
			s.SummaryMarkdown += "step was never finalized; did the build crash?"
			s.EndTime = nowTS
		}
	}
}
