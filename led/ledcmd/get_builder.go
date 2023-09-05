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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	swarmbucket "go.chromium.org/luci/common/api/buildbucket/swarmbucket/v1"
	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/retry/transient"
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
	RealBuild      bool
}

func getBuilderJobName(opts GetBuildersOpts) string {
	return fmt.Sprintf(`get-builder %s:%s`, opts.Bucket, opts.Builder)
}

// GetBuilder retrieves a new job Definition from a Buildbucket builder.
func GetBuilder(ctx context.Context, authClient *http.Client, opts GetBuildersOpts) (*job.Definition, error) {
	if opts.RealBuild {
		return synthesizeBuildFromBuilder(ctx, authClient, opts)
	}

	if opts.KitchenSupport == nil {
		opts.KitchenSupport = job.NoKitchenSupport()
	}

	sbucket := newSwarmbucketClient(authClient, opts.BuildbucketHost)

	type parameters struct {
		BuilderName     string `json:"builder_name"`
		APIExplorerLink bool   `json:"api_explorer_link"`
	}

	data, err := json.Marshal(&parameters{opts.Builder, false})
	if err != nil {
		return nil, err
	}

	canaryPref := "PROD"
	if opts.Canary {
		canaryPref = "CANARY"
	}

	args := &swarmbucket.LegacySwarmbucketApiGetTaskDefinitionRequestMessage{
		BuildRequest: &swarmbucket.LegacyApiPutRequestMessage{
			CanaryPreference: canaryPref,
			Bucket:           opts.Bucket,
			ParametersJson:   string(data),
			Tags:             opts.ExtraTags,
		},
	}
	answer, err := sbucket.GetTaskDef(args).Context(ctx).Do()
	if err != nil {
		return nil, transient.Tag.Apply(err)
	}

	newTask := &swarming.SwarmingRpcsNewTaskRequest{}
	r := strings.NewReader(answer.TaskDefinition)
	if err = json.NewDecoder(r).Decode(newTask); err != nil {
		return nil, err
	}

	jd, err := jobcreate.FromNewTaskRequest(
		ctx, newTask, getBuilderJobName(opts),
		answer.SwarmingHost, opts.KitchenSupport, opts.PriorityDiff, nil, opts.ExtraTags, authClient)
	if err != nil {
		return nil, err
	}

	if err := fillCasDefaults(jd); err != nil {
		return nil, err
	}

	return jd, nil
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
