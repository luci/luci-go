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
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

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

	BucketV1 string
}

func getBuilderJobName(opts GetBuildersOpts, realBuild bool) string {
	if realBuild {
		return fmt.Sprintf(`get-builder %s:%s:%s`, opts.Project, opts.Bucket, opts.Builder)
	}
	return fmt.Sprintf(`get-builder %s:%s`, opts.BucketV1, opts.Builder)
}

// GetBuilder retrieves a new job Definition from a Buildbucket builder.
func GetBuilder(ctx context.Context, authClient *http.Client, opts GetBuildersOpts) (*job.Definition, error) {
	jd, err := synthesizeBuildFromBuilder(ctx, authClient, opts)
	if opts.RealBuild {
		return jd, err
	}

	jdBucket := jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetBuilder().GetBucket()
	if err == nil && jdBucket != opts.Bucket {
		// The builder has shadow bucket enabled, switch to real build mode automatically.
		logging.Infof(ctx, "Bucket %s has shadow bucket %s, Switching to led real-build automatically.", opts.Bucket, jdBucket)
		return jd, err
	}

	logging.Warningf(ctx, "The legacy led jobs as raw swarming tasks is scheduled to be deprecated at the end of Q1, 2024. Please follow http://shortn/_9FEIA3yZbK to enable led real builds in your project/bucket.")

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
			Bucket:           opts.BucketV1,
			ParametersJson:   string(data),
			Tags:             opts.ExtraTags,
		},
	}
	answer, err := sbucket.GetTaskDef(args).Context(ctx).Do()
	if err != nil {
		return nil, transient.Tag.Apply(err)
	}

	newRpcsTask := &swarming.SwarmingRpcsNewTaskRequest{}
	r := strings.NewReader(answer.TaskDefinition)
	if err = json.NewDecoder(r).Decode(newRpcsTask); err != nil {
		return nil, err
	}

	newTask := toNewTaskRequest(newRpcsTask)

	jd, err = jobcreate.FromNewTaskRequest(
		ctx, newTask, getBuilderJobName(opts, false),
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
	return jobcreate.FromBuild(build, opts.BuildbucketHost, getBuilderJobName(opts, true), opts.PriorityDiff, opts.ExtraTags), nil
}

func toNewTaskRequest(r *swarming.SwarmingRpcsNewTaskRequest) *swarmingpb.NewTaskRequest {
	ret := &swarmingpb.NewTaskRequest{
		Name:            r.Name,
		ParentTaskId:    r.ParentTaskId,
		Priority:        int32(r.Priority),
		TaskSlices:      make([]*swarmingpb.TaskSlice, 0, len(r.TaskSlices)),
		Tags:            r.Tags,
		User:            r.User,
		ServiceAccount:  r.ServiceAccount,
		PubsubTopic:     r.PubsubTopic,
		PubsubAuthToken: r.PubsubAuthToken,
		PubsubUserdata:  r.PubsubUserdata,
		EvaluateOnly:    r.EvaluateOnly,
		PoolTaskTemplate: swarmingpb.NewTaskRequest_PoolTaskTemplateField(
			swarmingpb.NewTaskRequest_PoolTaskTemplateField_value[r.PoolTaskTemplate],
		),
		BotPingToleranceSecs: int32(r.BotPingToleranceSecs),
		RequestUuid:          r.RequestUuid,
		Resultdb:             &swarmingpb.ResultDBCfg{},
		Realm:                r.Realm,
	}
	for _, t := range r.TaskSlices {
		props := t.Properties
		nt := &swarmingpb.TaskSlice{
			Properties: &swarmingpb.TaskProperties{
				Caches: make([]*swarmingpb.CacheEntry, 0, len(props.Caches)),
				CipdInput: &swarmingpb.CipdInput{
					Packages: make([]*swarmingpb.CipdPackage, 0, len(props.CipdInput.Packages)),
				},
				Command:              props.Command,
				RelativeCwd:          props.RelativeCwd,
				Dimensions:           make([]*swarmingpb.StringPair, 0, len(props.Dimensions)),
				Env:                  make([]*swarmingpb.StringPair, 0, len(props.Env)),
				EnvPrefixes:          make([]*swarmingpb.StringListPair, 0, len(props.EnvPrefixes)),
				ExecutionTimeoutSecs: int32(props.ExecutionTimeoutSecs),
				GracePeriodSecs:      int32(props.GracePeriodSecs),
				Idempotent:           props.Idempotent,
				IoTimeoutSecs:        int32(props.IoTimeoutSecs),
				Outputs:              props.Outputs,
				SecretBytes:          []byte(props.SecretBytes),
			},

			ExpirationSecs:  int32(t.ExpirationSecs),
			WaitForCapacity: t.WaitForCapacity,
		}
		ret.TaskSlices = append(ret.TaskSlices, nt)
		if cir := props.CasInputRoot; cir != nil {
			nt.Properties.CasInputRoot = &swarmingpb.CASReference{
				CasInstance: cir.CasInstance,
			}
			if d := cir.Digest; d != nil {
				nt.Properties.CasInputRoot.Digest = &swarmingpb.Digest{
					Hash:      d.Hash,
					SizeBytes: d.SizeBytes,
				}
			}
		}
		if c := props.Containment; c != nil {
			nt.Properties.Containment = &swarmingpb.Containment{
				ContainmentType: swarmingpb.Containment_ContainmentType(
					swarmingpb.Containment_ContainmentType_value[c.ContainmentType],
				),
			}
		}
		for _, env := range props.Env {
			nt.Properties.Env = append(nt.Properties.Env, &swarmingpb.StringPair{
				Key:   env.Key,
				Value: env.Value,
			})
		}

		for _, path := range props.EnvPrefixes {
			nt.Properties.EnvPrefixes = append(nt.Properties.EnvPrefixes, &swarmingpb.StringListPair{
				Key:   path.Key,
				Value: path.Value,
			})
		}

		for _, cache := range props.Caches {
			nt.Properties.Caches = append(nt.Properties.Caches, &swarmingpb.CacheEntry{
				Name: cache.Name,
				Path: cache.Path,
			})
		}

		for _, pkg := range props.CipdInput.Packages {
			nt.Properties.CipdInput.Packages = append(nt.Properties.CipdInput.Packages, &swarmingpb.CipdPackage{
				PackageName: pkg.PackageName,
				Version:     pkg.Version,
				Path:        pkg.Path,
			})
		}

		for _, dim := range props.Dimensions {
			nt.Properties.Dimensions = append(nt.Properties.Dimensions, &swarmingpb.StringPair{
				Key:   dim.Key,
				Value: dim.Value,
			})
		}
	}
	if rdb := r.Resultdb; rdb != nil {
		ret.Resultdb.Enable = rdb.Enable
	}

	return ret
}
