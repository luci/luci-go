// Copyright 2022 The LUCI Authors.
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

// Package rerun handles rerun for a build.
package rerun

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/testfailureanalysis"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

// TriggerOptions contains information how the rerun should be triggered.
type TriggerOptions struct {
	// The builder we should trigger the rerun on. Required.
	Builder *buildbucketpb.BuilderID
	// The gitiles commit for the revision that we want to run the rerun build. Required.
	GitilesCommit *buildbucketpb.GitilesCommit
	// The buildbucket ID that the rerun build should copy the properties
	// and dimension from. Required.
	SampleBuildID int64
	// Extra properties we want the rerun build to have.
	ExtraProperties map[string]any
	// Extra dimensions we want the rerun build to have.
	ExtraDimensions map[string]string
	// Priority of the rerun build.
	Priority int32
}

// TriggerRerun triggers a rerun build given the options.
func TriggerRerun(c context.Context, options *TriggerOptions) (*buildbucketpb.Build, error) {
	err := validateOptions(options)
	if err != nil {
		return nil, errors.Fmt("validate rerun options: %w", err)
	}
	logging.Infof(c, "triggerRerun with commit %s", options.GitilesCommit.Id)
	properties, dimensions, err := getRerunPropertiesAndDimensions(c, options.SampleBuildID, options.ExtraProperties, options.ExtraDimensions)
	if err != nil {
		logging.Errorf(c, "Failed getRerunPropertiesAndDimension for build %d", options.SampleBuildID)
		return nil, err
	}
	req := &buildbucketpb.ScheduleBuildRequest{
		Builder:       options.Builder,
		Properties:    properties,
		Dimensions:    dimensions,
		Tags:          getRerunTags(c, options.SampleBuildID),
		GitilesCommit: options.GitilesCommit,
		Priority:      options.Priority,
	}
	build, err := buildbucket.ScheduleBuild(c, req)
	if err != nil {
		logging.Errorf(c, "Failed trigger rerun for build %d: %w", options.SampleBuildID, err)
		return nil, err
	}
	logging.Infof(c, "Rerun build %d triggered for build: %d", build.GetId(), options.SampleBuildID)
	return build, nil
}

func validateOptions(options *TriggerOptions) error {
	if options == nil {
		return errors.New("option must not be nil")
	}
	if options.Builder == nil {
		return errors.New("builder must not be nil")
	}
	if options.GitilesCommit == nil {
		return errors.New("gitiles commit must not be nil")
	}
	if options.SampleBuildID == 0 {
		return errors.New("sample build id must be specified")
	}
	return nil
}

// getRerunTags returns the build bucket tags for the rerun build
func getRerunTags(c context.Context, bbid int64) []*buildbucketpb.StringPair {
	return []*buildbucketpb.StringPair{
		{
			// analyzed_build_id is the buildbucket ID of the build which we want to rerun.
			Key:   "analyzed_build_id",
			Value: strconv.FormatInt(bbid, 10),
		},
	}
}

// getRerunPropertiesAndDimensions returns the properties and dimensions for a rerun of a buildID.
// If the builder is a tester, the dimension will be derived from its parent build.
func getRerunPropertiesAndDimensions(c context.Context, bbid int64, props map[string]any, dims map[string]string) (*structpb.Struct, []*buildbucketpb.RequestedDimension, error) {
	mask := &buildbucketpb.BuildMask{
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"input.properties", "builder", "infra.swarming.task_dimensions", "infra.backend.task_dimensions"},
		},
	}
	build, err := buildbucket.GetBuild(c, bbid, mask)
	if err != nil {
		return nil, nil, errors.Fmt("failed to get properties for build %d: %w", bbid, err)
	}
	properties, err := getRerunProperties(c, build, props)
	if err != nil {
		return nil, nil, err
	}
	parentBuildIDStr, found := build.GetInput().GetProperties().GetFields()["parent_build_id"]

	// If builder is not a tester, return the dimension derived by this build.
	if !found {
		dimens := getRerunDimensions(c, build, dims)
		return properties, dimens, nil
	}
	parentBuildID, err := strconv.Atoi(parentBuildIDStr.GetStringValue())
	if err != nil {
		return nil, nil, errors.Fmt("parse parent_build_id %s: %w", parentBuildIDStr, err)
	}
	// If builder is a tester, return the dimension derived by the parent build.
	parentBuild, err := buildbucket.GetBuild(c, int64(parentBuildID), mask)
	if err != nil {
		return nil, nil, errors.Fmt("failed to get properties for parent build %d: %w", int64(parentBuildID), err)
	}
	dimens := getRerunDimensions(c, parentBuild, dims)
	return properties, dimens, nil
}

func getRerunProperties(c context.Context, build *buildbucketpb.Build, props map[string]any) (*structpb.Struct, error) {
	fields := map[string]any{}
	properties := build.GetInput().GetProperties()
	if properties != nil {
		m := properties.GetFields()
		if builderGroup, ok := m["builder_group"]; ok {
			fields["builder_group"] = builderGroup
			fields["target_builder"] = map[string]string{
				"builder": build.Builder.Builder,
				"group":   builderGroup.GetStringValue(),
			}
		}
		if bootstrapProperties, ok := m["$bootstrap/properties"]; ok {
			fields["$bootstrap/properties"] = bootstrapProperties
		}
	}

	for k, v := range props {
		fields[k] = v
	}

	spb, err := toStructPB(fields)
	if err != nil {
		return nil, fmt.Errorf("cannot convert %v to structpb: %w", fields, err)
	}
	return spb, nil
}

func getRerunDimensions(c context.Context, build *buildbucketpb.Build, dims map[string]string) []*buildbucketpb.RequestedDimension {
	result := []*buildbucketpb.RequestedDimension{}

	// Only copy these dimensions from the analyzed builder to the rerun job request.
	allowedDimensions := map[string]bool{"os": true, "gpu": true}

	if dimens := util.GetTaskDimensions(build); dimens != nil {
		dimens := util.GetTaskDimensions(build)
		for _, d := range dimens {
			if _, ok := allowedDimensions[d.Key]; ok {
				result = append(result, &buildbucketpb.RequestedDimension{
					Key:   d.Key,
					Value: d.Value,
				})
			}
		}
	}

	// Add extra dimension from dims
	for k, v := range dims {
		result = append(result, &buildbucketpb.RequestedDimension{
			Key:   k,
			Value: v,
		})
	}

	return result
}

// CreateRerunBuildModel creates a CompileRerunBuild (and SingleRerun) in datastore
func CreateRerunBuildModel(c context.Context, build *buildbucketpb.Build, rerunType model.RerunBuildType, suspect *model.Suspect, nsa *model.CompileNthSectionAnalysis, priority int32) (*model.CompileRerunBuild, error) {
	if rerunType == model.RerunBuildType_CulpritVerification && suspect == nil {
		return nil, fmt.Errorf("CreateRerunBuildModel requires suspect when type is CulpritVerification")
	}
	if rerunType == model.RerunBuildType_NthSection && nsa == nil {
		return nil, fmt.Errorf("CreateRerunBuildModel requires nth section analysis when type is NthSection")
	}

	gitilesCommit := *build.GetInput().GetGitilesCommit()
	startTime := build.StartTime.AsTime()
	createTime := build.CreateTime.AsTime()
	rerunBuild := &model.CompileRerunBuild{
		Id: build.GetId(),
		LuciBuild: model.LuciBuild{
			BuildId:    build.GetId(),
			Project:    build.Builder.Project,
			Bucket:     build.Builder.Bucket,
			Builder:    build.Builder.Builder,
			CreateTime: createTime,
			StartTime:  startTime,
			Status:     build.GetStatus(),
			GitilesCommit: buildbucketpb.GitilesCommit{
				Host:    gitilesCommit.Host,
				Project: gitilesCommit.Project,
				Ref:     gitilesCommit.Ref,
				Id:      gitilesCommit.Id,
			},
		},
	}
	err := datastore.Put(c, rerunBuild)
	if err != nil {
		logging.Errorf(c, "Error in creating CompileRerunBuild model for build %d", build.GetId())
		return nil, err
	}
	dimensions, err := buildbucket.GetBuildTaskDimension(c, build.GetId())
	if err != nil {
		return nil, errors.Fmt("get build task dimension bbid %v: %w", build.GetId(), err)
	}
	// Create the first SingleRerun for CompileRerunBuild
	// It will be updated when we receive updates from recipe
	singleRerun := &model.SingleRerun{
		RerunBuild: datastore.KeyForObj(c, rerunBuild),
		Status:     pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		GitilesCommit: buildbucketpb.GitilesCommit{
			Host:    gitilesCommit.Host,
			Project: gitilesCommit.Project,
			Ref:     gitilesCommit.Ref,
			Id:      gitilesCommit.Id,
		},
		CreateTime: createTime,
		StartTime:  startTime,
		Type:       rerunType,
		Priority:   priority,
		Dimensions: dimensions,
	}

	if rerunType == model.RerunBuildType_CulpritVerification {
		singleRerun.Analysis = suspect.ParentAnalysis.Parent()
		singleRerun.Suspect = datastore.KeyForObj(c, suspect)
	}
	if rerunType == model.RerunBuildType_NthSection {
		singleRerun.Analysis = nsa.ParentAnalysis
		singleRerun.NthSectionAnalysis = datastore.KeyForObj(c, nsa)
	}

	err = datastore.Put(c, singleRerun)
	if err != nil {
		logging.Errorf(c, "Error in creating SingleRerun model for build %d", build.GetId())
		return nil, err
	}

	return rerunBuild, nil
}

// UpdateCompileRerunStatus updates the start/end time and status of rerun builds and single rerun (when we received buildbucket pubsub messages)
func UpdateCompileRerunStatus(c context.Context, bbid int64) error {
	logging.Infof(c, "UpdateCompileRerunStatus for build %d", bbid)
	rerunModel := &model.CompileRerunBuild{
		Id: bbid,
	}

	err := datastore.Get(c, rerunModel)
	if err == datastore.ErrNoSuchEntity {
		// There are cases where we cannot find datastore entries, like
		// luci-bisection-dev receives pubsub message for a prod run
		// In this case, just log and return nil
		logging.Warningf(c, "Couldn't find compile rerun to update status: %d", bbid)
		return nil
	}
	if err != nil {
		return errors.Fmt("couldn't get rerun model %d: %w", bbid, err)
	}

	lastRerun, err := datastoreutil.GetLastRerunForRerunBuild(c, rerunModel)
	if err != nil {
		return errors.Fmt("failed getting last rerun for build %d: %w", rerunModel.Id, err)
	}

	build, err := buildbucket.GetBuild(c, bbid, &buildbucketpb.BuildMask{
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"id", "builder", "end_time", "start_time", "status"},
		},
	})
	if err != nil {
		return errors.Fmt("couldn't get build %d: %w", bbid, err)
	}

	startTime := build.StartTime.AsTime()
	endTime := build.EndTime.AsTime()

	err = datastore.RunInTransaction(c, func(ctx context.Context) error {
		e := datastore.Get(c, rerunModel)
		if e != nil {
			return e
		}
		rerunModel.StartTime = startTime
		rerunModel.EndTime = endTime
		rerunModel.Status = build.Status
		return datastore.Put(c, rerunModel)
	}, nil)

	if err != nil {
		return errors.Fmt("couldn't save rerun model %d: %w", bbid, err)
	}

	err = datastore.RunInTransaction(c, func(ctx context.Context) error {
		e := datastore.Get(c, lastRerun)
		if e != nil {
			return e
		}
		buildEnded := build.Status&buildbucketpb.Status_ENDED_MASK == buildbucketpb.Status_ENDED_MASK
		if buildEnded && !lastRerun.HasEnded() {
			// Edge case: when the build ends but the rerun isn't ended,
			// this suggests that there is a infra failure in the rerun build
			// which prevent it from sending back the update via the UpdateAnalysisProgress RPC.
			// TODO (nqmtuan): Perhaps we need to update Analysis and NthSection analysis status too?
			lastRerun.Status = pb.RerunStatus_RERUN_STATUS_INFRA_FAILED
			lastRerun.EndTime = endTime
		}
		lastRerun.StartTime = startTime
		return datastore.Put(c, lastRerun)
	}, nil)

	if err != nil {
		return errors.Fmt("failed saving last rerun for build %d: %w", rerunModel.Id, err)
	}
	return nil
}

// UpdateTestRerunStatus is called when we receive updates from buildbucket
// for test rerun build.
func UpdateTestRerunStatus(ctx context.Context, build *buildbucketpb.Build) error {
	bbid := build.Id
	logging.Infof(ctx, "UpdateTestRerunStatus for build %d", bbid)
	rerunFailed := false
	singleRerun := &model.TestSingleRerun{
		ID: bbid,
	}

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		err := datastore.Get(ctx, singleRerun)
		if err == datastore.ErrNoSuchEntity {
			// There are cases where we cannot find datastore entries, like
			// luci-bisection-dev receives pubsub message for a prod run.
			// In this case, just log and return nil.
			logging.Warningf(ctx, "Couldn't find test rerun to update status : %d", bbid)
			return nil
		}
		if err != nil {
			return errors.Fmt("couldn't get TestSingleRerun %d: %w", bbid, err)
		}

		singleRerun.LUCIBuild.StartTime = build.StartTime.AsTime()
		singleRerun.LUCIBuild.EndTime = build.EndTime.AsTime()
		singleRerun.LUCIBuild.Status = build.Status
		buildEnded := build.Status&buildbucketpb.Status_ENDED_MASK == buildbucketpb.Status_ENDED_MASK

		if buildEnded && !singleRerun.HasEnded() {
			// Edge case: when the build ends but the rerun isn't ended,
			// this suggests that there is a infra failure in the rerun build
			// which prevent it from sending back the update via the UpdateTestAnalysisProgress RPC.
			singleRerun.Status = pb.RerunStatus_RERUN_STATUS_INFRA_FAILED
			rerunFailed = true
		}

		err = datastore.Put(ctx, singleRerun)
		if err != nil {
			return errors.Fmt("couldn't save single rerun %d: %w", bbid, err)
		}
		return nil
	}, nil)

	if err != nil {
		return errors.Fmt("saving test single rerun: %w", err)
	}

	if rerunFailed {
		tfa, err := datastoreutil.GetTestFailureAnalysis(ctx, singleRerun.AnalysisKey.IntID())
		if err != nil {
			return errors.Fmt("get test failure analysis: %w", err)
		}
		// Update analysis and nthsection analysis if applicable.
		// The reason why we put it here instead of the above transaction
		// was because read-after-write within a transaction does not work.
		// (https://cloud.google.com/datastore/docs/concepts/transactions#isolation_and_consistency)
		err = testfailureanalysis.UpdateAnalysisStatusWhenError(ctx, tfa)
		if err != nil {
			return errors.Fmt("update analysis status when error: %w", err)
		}
	}
	return nil
}

// TODO (nqmtuan): Move this into a helper class if it turns out we need to use
// it for more than one place
// toStructPB convert an any s to structpb.Struct, as long as s is marshallable.
// s can be a general Go type, structpb.Struct type, or mixed.
// For example, s can be a map of mixed type, like
// {"key1": "val1", "key2": structpb.NewStringValue("val2")}
func toStructPB(s any) (*structpb.Struct, error) {
	// We used json as an intermediate format to convert
	j, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(j, &m); err != nil {
		return nil, err
	}
	return structpb.NewStruct(m)
}

// CreateTestRerunModelOptions contains parameters for creating a test rerun model.
type CreateTestRerunModelOptions struct {
	TestFailureAnalysis   *model.TestFailureAnalysis
	NthSectionAnalysisKey *datastore.Key
	SuspectKey            *datastore.Key
	TestFailures          []*model.TestFailure
	Build                 *buildbucketpb.Build
	RerunType             model.RerunBuildType
}

// CreateTestRerunModel creates a TestSingleRerun in datastore for test failure analysis.
func CreateTestRerunModel(ctx context.Context, options CreateTestRerunModelOptions) (*model.TestSingleRerun, error) {
	build := options.Build
	dimensions, err := buildbucket.GetBuildTaskDimension(ctx, build.GetId())
	if err != nil {
		return nil, errors.Fmt("get build task dimension bbid %v: %w", build.GetId(), err)
	}
	testResults := model.RerunTestResults{}
	for _, tf := range options.TestFailures {
		testResults.Results = append(testResults.Results, model.RerunSingleTestResult{
			TestFailureKey: datastore.KeyForObj(ctx, tf),
		})
	}

	rerun := &model.TestSingleRerun{
		ID: build.GetId(),
		LUCIBuild: model.LUCIBuild{
			BuildID:     build.GetId(),
			Project:     build.Builder.Project,
			Bucket:      build.Builder.Bucket,
			Builder:     build.Builder.Builder,
			BuildNumber: int(build.Number),
			GitilesCommit: &buildbucketpb.GitilesCommit{
				Host:    build.Input.GitilesCommit.Host,
				Project: build.Input.GitilesCommit.Project,
				Id:      build.Input.GitilesCommit.Id,
				Ref:     build.Input.GitilesCommit.Ref,
			},
			Status:     build.Status,
			CreateTime: build.CreateTime.AsTime(),
			StartTime:  build.StartTime.AsTime(),
		},
		Type:                  options.RerunType,
		AnalysisKey:           datastore.KeyForObj(ctx, options.TestFailureAnalysis),
		CulpritKey:            options.SuspectKey,
		NthSectionAnalysisKey: options.NthSectionAnalysisKey,
		Status:                pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		Priority:              options.TestFailureAnalysis.Priority,
		TestResults:           testResults,
		Dimensions:            dimensions,
	}

	if err = datastore.Put(ctx, rerun); err != nil {
		return nil, errors.Fmt("save single rerun: %w", err)
	}
	return rerun, nil
}
