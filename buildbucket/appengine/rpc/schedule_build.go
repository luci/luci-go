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

package rpc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildid"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/internal/search"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// validateExecutable validates the given executable.
func validateExecutable(exe *pb.Executable) error {
	var err error
	switch {
	case exe.GetCipdPackage() != "":
		return errors.Reason("cipd_package must not be specified").Err()
	case exe.GetCipdVersion() != "" && teeErr(common.ValidateInstanceVersion(exe.CipdVersion), &err) != nil:
		return errors.Annotate(err, "cipd_version").Err()
	default:
		return nil
	}
}

// validateSchedule validates the given request.
func validateSchedule(req *pb.ScheduleBuildRequest) error {
	var err error
	switch {
	case strings.Contains(req.GetRequestId(), "/"):
		return errors.Reason("request_id cannot contain '/'").Err()
	case req.GetBuilder() == nil && req.GetTemplateBuildId() == 0:
		return errors.Reason("builder or template_build_id is required").Err()
	case req.Builder != nil && teeErr(protoutil.ValidateRequiredBuilderID(req.Builder), &err) != nil:
		return errors.Annotate(err, "builder").Err()
	case teeErr(validateExecutable(req.Exe), &err) != nil:
		return errors.Annotate(err, "exe").Err()
	case req.GitilesCommit != nil && teeErr(validateCommitWithRef(req.GitilesCommit), &err) != nil:
		return errors.Annotate(err, "gitiles_commit").Err()
	case teeErr(validateTags(req.Tags, TagNew), &err) != nil:
		return errors.Annotate(err, "tags").Err()
	case req.Priority < 0 || req.Priority > 255:
		return errors.Reason("priority must be in [0, 255]").Err()
	}

	// TODO(crbug/1042991): Validate Properties, Gerrit Changes, Dimensions, Notify.
	return nil
}

// templateBuildMask enumerates properties to read from template builds. See
// scheduleRequestFromTemplate.
var templateBuildMask = mask.MustFromReadMask(
	&pb.Build{},
	"builder",
	"canary",
	"critical",
	"exe",
	"input.experimental",
	"input.gerrit_changes",
	"input.gitiles_commit",
	"input.properties",
	"tags",
)

// scheduleRequestFromTemplate returns a request with fields populated by the
// given template_build_id if there is one. Fields set in the request override
// fields populated from the template. Does not modify the incoming request.
func scheduleRequestFromTemplate(ctx context.Context, req *pb.ScheduleBuildRequest) (*pb.ScheduleBuildRequest, error) {
	if req.GetTemplateBuildId() == 0 {
		return req, nil
	}

	bld, err := getBuild(ctx, req.TemplateBuildId)
	if err != nil {
		return nil, err
	}
	if err := perm.HasInBuilder(ctx, perm.BuildsGet, bld.Proto.Builder); err != nil {
		return nil, err
	}

	b := bld.ToSimpleBuildProto(ctx)
	if err := model.LoadBuildDetails(ctx, templateBuildMask, b); err != nil {
		return nil, err
	}

	ret := &pb.ScheduleBuildRequest{
		Builder:       b.Builder,
		Critical:      b.Critical,
		Exe:           b.Exe,
		GerritChanges: b.Input.GerritChanges,
		GitilesCommit: b.Input.GitilesCommit,
		Properties:    b.Input.Properties,
		Tags:          b.Tags,
	}

	// Convert bool to the corresponding pb.Trinary values.
	ret.Canary = pb.Trinary_NO
	ret.Experimental = pb.Trinary_NO
	if b.Canary {
		ret.Canary = pb.Trinary_YES
	}
	if b.Input.Experimental {
		ret.Experimental = pb.Trinary_YES
	}

	// proto.Merge concatenates repeated fields. Here the desired behavior is replacement,
	// so clear slices from the return value before merging, if specified in the request.
	if req.Exe != nil {
		ret.Exe = nil
	}
	if len(req.GerritChanges) > 0 {
		ret.GerritChanges = nil
	}
	if req.Properties != nil {
		ret.Properties = nil
	}
	if len(req.Tags) > 0 {
		ret.Tags = nil
	}
	proto.Merge(ret, req)
	ret.TemplateBuildId = 0
	return ret, nil
}

// fetchBuilderConfigs returns the Builder configs referenced by the given
// requests in a map of Bucket ID -> Builder name -> *pb.Builder.
func fetchBuilderConfigs(ctx context.Context, reqs []*pb.ScheduleBuildRequest) (map[string]map[string]*pb.Builder, error) {
	cfgs := map[string]map[string]*pb.Builder{}
	var bldrs []*model.Builder
	for _, req := range reqs {
		bucket := fmt.Sprintf("%s/%s", req.Builder.Project, req.Builder.Bucket)
		if _, ok := cfgs[bucket]; !ok {
			cfgs[bucket] = make(map[string]*pb.Builder)
		}
		if _, ok := cfgs[bucket][req.Builder.Builder]; ok {
			continue
		}
		b := &model.Builder{
			Parent: model.BucketKey(ctx, req.Builder.Project, req.Builder.Bucket),
			ID:     req.Builder.Builder,
		}
		cfgs[bucket][req.Builder.Builder] = &b.Config
		bldrs = append(bldrs, b)
	}
	if err := datastore.Get(ctx, bldrs); err != nil {
		// TODO(crbug/1042991): Return InvalidArgument if the error is "not found".
		return nil, err
	}
	return cfgs, nil
}

// generateBuildNumbers mutates the given builds, setting build numbers and
// build address tags.
func generateBuildNumbers(ctx context.Context, builds []*model.Build) error {
	seq := make(map[string][]*model.Build)
	for _, b := range builds {
		name := fmt.Sprintf("%s/%s/%s", b.Proto.Builder.Project, b.Proto.Builder.Bucket, b.Proto.Builder.Builder)
		seq[name] = append(seq[name], b)
	}
	return parallel.WorkPool(64, func(work chan<- func() error) {
		for name, blds := range seq {
			name := name
			blds := blds
			work <- func() error {
				n, err := model.GenerateSequenceNumbers(ctx, name, len(blds))
				if err != nil {
					return err
				}
				for i, b := range blds {
					b.Proto.Number = n + int32(i)
					addr := fmt.Sprintf("build_address:luci.%s.%s/%s/%d", b.Proto.Builder.Project, b.Proto.Builder.Bucket, b.Proto.Builder.Builder, b.Proto.Number)
					b.Tags = append(b.Tags, addr)
					sort.Strings(b.Tags)
				}
				return nil
			}
		}
	})
}

// setExperiments computes the experiments from the given request and builder
// config, setting them in the model and proto. Mutates the given *model.Build.
// model.Build.Proto.Input must not be nil.
func setExperiments(ctx context.Context, req *pb.ScheduleBuildRequest, cfg *pb.Builder, build *model.Build) {
	// Experiment -> enabled.
	exps := make(map[string]bool, len(req.GetExperiments()))

	// Experiment values in the experiments field of the request have highest precedence,
	// followed by legacy fields in the request, followed by values in the builder config.
	for exp, en := range req.GetExperiments() {
		exps[exp] = en
	}

	// Legacy.
	if _, ok := exps[bb.ExperimentBBCanarySoftware]; !ok && req.GetCanary() != pb.Trinary_UNSET {
		exps[bb.ExperimentBBCanarySoftware] = req.Canary == pb.Trinary_YES
	}
	if _, ok := exps[bb.ExperimentNonProduction]; !ok && req.GetExperimental() != pb.Trinary_UNSET {
		exps[bb.ExperimentNonProduction] = req.Experimental == pb.Trinary_YES
	}

	for exp, pct := range cfg.GetExperiments() {
		if _, ok := exps[exp]; ok {
			continue
		}
		exps[exp] = mathrand.Int31n(ctx, 100) < pct
	}

	// The model stores all experiment values, while the proto only contains enabled experiments.
	for exp, en := range exps {
		if en {
			build.Experiments = append(build.Experiments, fmt.Sprintf("+%s", exp))
			build.Proto.Input.Experiments = append(build.Proto.Input.Experiments, exp)
		} else {
			build.Experiments = append(build.Experiments, fmt.Sprintf("-%s", exp))
		}
	}

	// Set legacy field values.
	if en := exps[bb.ExperimentBBCanarySoftware]; en {
		build.Canary = true
		build.Proto.Canary = true
	}
	if en := exps[bb.ExperimentNonProduction]; en {
		build.Experimental = true
		build.Proto.Input.Experimental = true
	}
	sort.Strings(build.Proto.Input.Experiments)
	sort.Strings(build.Experiments)
}

// setTags computes the tags from the given request, setting them in the model.
// Mutates the given *model.Build.
func setTags(req *pb.ScheduleBuildRequest, build *model.Build) {
	tags := protoutil.StringPairMap(req.GetTags())
	if req.GetBuilder() != nil {
		tags.Add("builder", req.Builder.Builder)
	}
	if req.GetGitilesCommit() != nil {
		tags.Add("buildset", protoutil.GitilesBuildSet(req.GitilesCommit))
		tags.Add("gitiles_ref", req.GitilesCommit.Ref)
	}
	for _, ch := range req.GetGerritChanges() {
		tags.Add("buildset", protoutil.GerritBuildSet(ch))
	}
	build.Tags = tags.Format()
}

// scheduleBuilds handles requests to schedule builds. Requests must be
// validated and authorized.
func scheduleBuilds(ctx context.Context, reqs ...*pb.ScheduleBuildRequest) ([]*model.Build, error) {
	// TODO(crbug/1042991): Deduplicate request IDs.
	now := clock.Now(ctx).UTC()

	// Bucket -> Builder -> *pb.Builder.
	cfgs, err := fetchBuilderConfigs(ctx, reqs)
	if err != nil {
		return nil, errors.Annotate(err, "error fetching builders").Err()
	}

	blds := make([]*model.Build, len(reqs))
	nums := make([]*model.Build, 0, len(reqs))
	ids := buildid.NewBuildIDs(ctx, now, len(reqs))
	for i := range blds {
		bucket := fmt.Sprintf("%s/%s", reqs[i].Builder.Project, reqs[i].Builder.Bucket)
		cfg := cfgs[bucket][reqs[i].Builder.Builder]

		// TODO(crbug/1042991): Fill in relevant proto fields from the builder config.
		// TODO(crbug/1042991): Fill in relevant proto fields from the request.
		// TODO(crbug/1042991): Parallelize build creation from requests if necessary.
		blds[i] = &model.Build{
			ID:         ids[i],
			CreateTime: now,
			Proto: pb.Build{
				Builder:    reqs[i].Builder,
				CreateTime: timestamppb.New(now),
				Id:         ids[i],
				Input:      &pb.Build_Input{},
			},
		}

		setExperiments(ctx, reqs[i], cfg, blds[i])
		setTags(reqs[i], blds[i])

		exp := make(map[int64]struct{})
		for _, d := range blds[i].Proto.Infra.GetSwarming().GetTaskDimensions() {
			exp[d.Expiration.Seconds] = struct{}{}
		}
		if len(exp) > 6 {
			return nil, appstatus.BadRequest(errors.Reason("build %d contains more than 6 unique expirations", i).Err())
		}

		if cfg.GetBuildNumbers() == pb.Toggle_YES {
			nums = append(nums, blds[i])
		}
	}
	if err := generateBuildNumbers(ctx, nums); err != nil {
		return nil, errors.Annotate(err, "error generating build numbers").Err()
	}

	// TODO(crbug/1150607): Create ResultDB invocations.
	// TODO(crbug/1042991): Update Builders in the datastore to point to the latest build.

	err = parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() error { return search.UpdateTagIndex(ctx, blds) }
	})
	if err != nil {
		return nil, err
	}

	// This parallel work isn't combined with the above parallel work to ensure build entities and Swarming
	// task creation tasks are only created if everything else has succeeded (since everything can't be done
	// in one transaction).
	err = parallel.WorkPool(64, func(work chan<- func() error) {
		for i, b := range blds {
			b := b
			// blds and reqs slices map 1:1.
			reqID := reqs[i].RequestId
			work <- func() error {
				toPut := []interface{}{b}
				if b.Proto.Infra != nil {
					toPut = append(toPut, &model.BuildInfra{
						Build: datastore.KeyForObj(ctx, b),
						Proto: model.DSBuildInfra{
							BuildInfra: *b.Proto.Infra,
						},
					})
				}
				if b.Proto.Input.Properties != nil {
					toPut = append(toPut, &model.BuildInputProperties{
						Build: datastore.KeyForObj(ctx, b),
						Proto: model.DSStruct{
							Struct: *b.Proto.Input.Properties,
						},
					})
				}
				r := model.NewRequestID(ctx, b.ID, now, reqID)

				// Write the entities and trigger a task queue task to create the Swarming task.
				err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					// Deduplicate by request ID.
					if reqID != "" {
						switch err := datastore.Get(ctx, r); {
						case err == datastore.ErrNoSuchEntity:
							toPut = append(toPut, r)
						case err != nil:
							return errors.Annotate(err, "failed to deduplicate request ID: %d", b.ID).Err()
						default:
							b.ID = r.BuildID
							if err := datastore.Get(ctx, b); err != nil {
								return errors.Annotate(err, "failed to fetch deduplicated build: %d", b.ID).Err()
							}
							return nil
						}
					}

					// Request was not a duplicate.
					switch err := datastore.Get(ctx, &model.Build{ID: b.ID}); {
					case err == nil:
						return appstatus.Errorf(codes.AlreadyExists, "build already exists: %d", b.ID)
					case err != datastore.ErrNoSuchEntity:
						return errors.Annotate(err, "failed to fetch build: %d", b.ID).Err()
					}

					if err := datastore.Put(ctx, toPut...); err != nil {
						return errors.Annotate(err, "failed to store build: %d", b.ID).Err()
					}

					if err := tasks.CreateSwarmingTask(ctx, &taskdefs.CreateSwarmingTask{
						BuildId: b.ID,
					}); err != nil {
						return errors.Annotate(err, "failed to enqueue swarming task creation task: %d", b.ID).Err()
					}
					return nil
				}, nil)
				if err != nil {
					return err
				}

				// TODO(crbug/1042991): Update build creation metric.
				return nil
			}
		}
	})
	if err != nil {
		return nil, err
	}

	return blds, nil
}

// ScheduleBuild handles a request to schedule a build. Implements pb.BuildsServer.
func (*Builds) ScheduleBuild(ctx context.Context, req *pb.ScheduleBuildRequest) (*pb.Build, error) {
	var err error
	if err = validateSchedule(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	m, err := getFieldMask(req.Fields)
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "fields").Err())
	}

	if req, err = scheduleRequestFromTemplate(ctx, req); err != nil {
		return nil, err
	}
	if err = perm.HasInBucket(ctx, perm.BuildsAdd, req.Builder.Project, req.Builder.Bucket); err != nil {
		return nil, err
	}

	blds, err := scheduleBuilds(ctx, req)
	if err != nil {
		return nil, err
	}
	return blds[0].ToProto(ctx, m)
}
