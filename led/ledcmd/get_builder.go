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

	swarmbucket "go.chromium.org/luci/common/api/buildbucket/swarmbucket/v1"
	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/job/jobcreate"
)

// GetBuildersOpts are the options for GetBuilder.
type GetBuildersOpts struct {
	BuildbucketHost string
	Bucket          string
	Builder         string
	Canary          bool
	ExtraTags       []string
	PriorityDiff    int

	KitchenSupport job.KitchenSupport
}

// GetBuilder retrieves a new job Definition from a Buildbucket builder.
func GetBuilder(ctx context.Context, authClient *http.Client, opts GetBuildersOpts) (*job.Definition, error) {
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
		ctx, newTask, fmt.Sprintf(`get-builder %s:%s`, opts.Bucket, opts.Builder),
		answer.SwarmingHost, opts.KitchenSupport, opts.PriorityDiff, nil, authClient)
	if err != nil {
		return nil, err
	}

	if err := fillCasDefaults(jd); err != nil {
		return nil, err
	}

	return jd, nil
}
