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

	bbpb "go.chromium.org/luci/buildbucket/proto"

	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/job/jobcreate"
)

// GetBuildersOpts are the options for GetBuilder.
type GetBuildersOpts struct {
	BuildbucketHost string
	Project         string
	Bucket          string
	Builder         string
	Canary          bool
	ExtraTags       []string
	PriorityDiff    int
	Experiments     map[string]bool

	KitchenSupport job.KitchenSupport

	BucketV1 string
}

func getBuilderJobName(opts GetBuildersOpts) string {
	return fmt.Sprintf(`get-builder %s:%s:%s`, opts.Project, opts.Bucket, opts.Builder)
}

// GetBuilder retrieves a new job Definition from a Buildbucket builder.
func GetBuilder(ctx context.Context, authClient *http.Client, opts GetBuildersOpts) (*job.Definition, error) {
	return synthesizeBuildFromBuilder(ctx, authClient, opts)
}

func synthesizeBuildFromBuilder(ctx context.Context, authClient *http.Client, opts GetBuildersOpts) (*job.Definition, error) {
	bbClient := newBuildbucketClient(authClient, opts.BuildbucketHost)
	build, err := bbClient.SynthesizeBuild(ctx, &bbpb.SynthesizeBuildRequest{
		Builder: &bbpb.BuilderID{
			Project: opts.Project,
			Bucket:  opts.Bucket,
			Builder: opts.Builder,
		},
		Experiments: opts.Experiments,
	})
	if err != nil {
		return nil, err
	}
	return jobcreate.FromBuild(build, opts.BuildbucketHost, getBuilderJobName(opts), opts.PriorityDiff, opts.ExtraTags), nil
}
