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

// package failure_detection analyses a failed build and determines if it needs
// to trigger a new analysis for it
package compilefailuredetection

import (
	"context"
	"fmt"

	"go.chromium.org/luci/bisection/compilefailureanalysis"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/model"
	gfipb "go.chromium.org/luci/bisection/proto"
	"go.chromium.org/luci/bisection/util"

	"go.chromium.org/luci/gae/service/datastore"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// AnalyzeBuild analyzes a build and trigger an analysis if necessary.
// Returns true if a new analysis is triggered, returns false otherwise.
func AnalyzeBuild(c context.Context, bbid int64) (bool, error) {
	logging.Infof(c, "AnalyzeBuild %d", bbid)
	build, err := buildbucket.GetBuild(c, bbid, &buildbucketpb.BuildMask{
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"id", "builder", "input", "status", "steps", "number", "start_time", "end_time", "create_time"},
		},
	})
	if err != nil {
		return false, err
	}

	if !shouldAnalyzeBuild(c, build) {
		return false, nil
	}

	lastPassedBuild, firstFailedBuild, err := getLastPassedFirstFailedBuilds(c, build)

	// Could not find last passed build, skip the analysis.
	if err != nil {
		logging.Infof(c, "Could not find last passed/first failed builds for failure of build %d. Exiting...", bbid)
		return false, nil
	}

	// Check if we need to trigger a new analysis.
	yes, cf, err := analysisExists(c, build, lastPassedBuild, firstFailedBuild)
	if err != nil {
		return false, err
	}
	// We don't need to trigger a new analysis.
	if !yes {
		logging.Infof(c, "There is already an analysis for regression range (%d, %d). No new analysis will be triggered for build %d", lastPassedBuild.Id, firstFailedBuild.Id, bbid)
		return false, nil
	}

	// No analysis for the regression range. Trigger one.
	// For now, it just triggers a heuristic analysis, which runs very fast.
	// In the future, once we have nth-section analysis, we should put in a task
	// queue and return immediately.
	_, err = compilefailureanalysis.AnalyzeFailure(c, cf, firstFailedBuild.Id, lastPassedBuild.Id)
	if err != nil {
		return false, err
	}
	return true, nil
}

func shouldAnalyzeBuild(c context.Context, build *buildbucketpb.Build) bool {
	// We only care about failed build
	// Note: We already check for status = bbv1.ResultFailure during pubsub ingestion.
	// But bbv1.ResultFailure is true for both failure and infra failure
	// So we need to check it here.
	if build.Status != buildbucketpb.Status_FAILURE {
		logging.Infof(c, "Build %d does not have FAILURE status", build.Id)
		return false
	}

	// We only care about builds with compile failure
	if !hasCompileStepStatus(c, build, buildbucketpb.Status_FAILURE) {
		logging.Infof(c, "No compile step for build %d", build.Id)
		return false
	}
	return true
}

// Search builds older than refBuild to find the last passed and first failed builds
func getLastPassedFirstFailedBuilds(c context.Context, refBuild *buildbucketpb.Build) (*buildbucketpb.Build, *buildbucketpb.Build, error) {
	firstFailedBuild := refBuild

	// Query buildbucket for the first build with compile failure
	// We only consider maximum of 100 builds before the failed build.
	// If we cannot find the regression range within 100 builds, the failure is
	// too old for the analysis to be useful.
	var buildsToSearch int32 = 100
	var batchSize int32 = 20
	var pageToken string = ""

	buildMask := &buildbucketpb.BuildMask{
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"id", "builder", "input", "status", "steps"},
		},
	}

	for buildsToSearch > 0 {
		// Tweak the batch size if necessary to respect the search limit
		if buildsToSearch < batchSize {
			batchSize = buildsToSearch
		}

		// Get the next batch of older builds
		olderBuilds, nextPageToken, err := buildbucket.SearchOlderBuilds(c, refBuild, buildMask, batchSize, pageToken)
		if err != nil {
			logging.Errorf(c, "Could not search for older builds: %s", err)
			return nil, nil, err
		}

		// Search this batch of older builds for the last passed and first failed build
		for _, oldBuild := range olderBuilds {
			// We found the last passed build
			if oldBuild.Status == buildbucketpb.Status_SUCCESS && hasCompileStepStatus(c, oldBuild, buildbucketpb.Status_SUCCESS) {
				return oldBuild, firstFailedBuild, nil
			}
			if oldBuild.Status == buildbucketpb.Status_FAILURE && hasCompileStepStatus(c, oldBuild, buildbucketpb.Status_FAILURE) {
				firstFailedBuild = oldBuild
			}
		}

		// Stop searching if there are no more older builds available
		if nextPageToken == "" {
			break
		}

		// Update the remaining number of builds to search and the page token
		buildsToSearch -= int32(len(olderBuilds))
		pageToken = nextPageToken
	}

	// If we have reached here, the last passed build could not be found within the search limit
	return nil, nil, fmt.Errorf("could not find last passed build")
}

// analysisExists checks if we need to trigger a new analysis.
// The function checks if there has been an analysis with the regression range.
// Returns true if a new analysis should be triggered, returns false otherwise.
// Also return the compileFailure model associated with the failure for convenience.
// Note that this function also create/update the associated CompileFailureModel
func analysisExists(c context.Context, refFailedBuild *buildbucketpb.Build, lastPassedBuild *buildbucketpb.Build, firstFailedBuild *buildbucketpb.Build) (bool, *model.CompileFailure, error) {
	logging.Infof(c, "check analysisExists for range (%d, %d)", lastPassedBuild.Id, firstFailedBuild.Id)

	// Create a CompileFailure record in datastore if necessary
	compileFailure, err := createCompileFailureModel(c, refFailedBuild)

	// Search in datastore if there is already an analysis with the same regression range
	// If not, trigger an analysis
	analysis, err := searchAnalysis(c, lastPassedBuild.Id, firstFailedBuild.Id)

	if err != nil {
		return false, nil, err
	}

	// There is an existing analysis.
	// We should not trigger another analysis, but instead we will "merge" the
	// compile failure with the existing one.
	if analysis != nil {
		compileFailureId := analysis.CompileFailure.IntID()
		logging.Infof(c, "An analysis already existed for compile failure with ID %d", compileFailureId)
		cf := &model.CompileFailure{
			Id:    compileFailureId,
			Build: analysis.CompileFailure.Parent(),
		}
		// Find the compile failure that the analysis runs on
		err := datastore.Get(c, cf)
		if err != nil {
			logging.Errorf(c, "Cannot find compile failure ID %d", compileFailureId)
			return false, nil, err
		}

		// If they are the same compileFailure, don't do anything.
		// This may happen when we receive duplicated/retried message from pubsub.
		if cf.Id == compileFailure.Id {
			return false, compileFailure, nil
		}

		// "Merge" the compile failures, so they use the same analysis
		compileFailure.MergedFailureKey = analysis.CompileFailure
		err = datastore.RunInTransaction(c, func(c context.Context) error {
			return datastore.Put(c, compileFailure)
		}, nil)

		if err != nil {
			return false, nil, err
		}

		return false, compileFailure, nil
	}

	return true, compileFailure, nil
}

func createCompileFailureModel(c context.Context, failedBuild *buildbucketpb.Build) (*model.CompileFailure, error) {
	// As we are using build ID as ID here, the entities will be created if not exist.
	// If it exists, we just update the entities.
	var compileFailure *model.CompileFailure
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		var gitilesCommit buildbucketpb.GitilesCommit
		input := failedBuild.GetInput()
		if input != nil && input.GitilesCommit != nil {
			gitilesCommit = *(input.GitilesCommit)
		}
		buildModel := &model.LuciFailedBuild{
			Id: failedBuild.Id,
			LuciBuild: model.LuciBuild{
				BuildId:       failedBuild.Id,
				Project:       failedBuild.GetBuilder().Project,
				Bucket:        failedBuild.GetBuilder().Bucket,
				Builder:       failedBuild.GetBuilder().Builder,
				BuildNumber:   int(failedBuild.Number),
				Status:        failedBuild.Status,
				StartTime:     failedBuild.StartTime.AsTime(),
				EndTime:       failedBuild.EndTime.AsTime(),
				CreateTime:    failedBuild.CreateTime.AsTime(),
				GitilesCommit: gitilesCommit,
			},
			BuildFailureType: gfipb.BuildFailureType_COMPILE,
		}
		e := datastore.Put(c, buildModel)
		if e != nil {
			return e
		}
		compileFailure = &model.CompileFailure{
			Id:    failedBuild.Id,
			Build: datastore.KeyForObj(c, buildModel),
		}
		return datastore.Put(c, compileFailure)
	}, nil)

	if err != nil {
		return nil, err
	}

	return compileFailure, nil
}

func searchAnalysis(c context.Context, lastPassedBuildId int64, firstFailedBuildId int64) (*model.CompileFailureAnalysis, error) {
	q := datastore.NewQuery("CompileFailureAnalysis").Eq("last_passed_build_id", lastPassedBuildId).Eq("first_failed_build_id", firstFailedBuildId)
	analyses := []*model.CompileFailureAnalysis{}
	err := datastore.GetAll(c, q, &analyses)
	if err != nil {
		logging.Errorf(c, "Error querying datastore for analysis for range (%d, %d): %s", lastPassedBuildId, firstFailedBuildId, err)
		return nil, err
	}
	if len(analyses) == 0 {
		return nil, nil
	}
	// There should only be at most one analysis for a range.
	if len(analyses) > 1 {
		logging.Warningf(c, "Found more than one analysis form range (%d, %d)", lastPassedBuildId, firstFailedBuildId)
	}
	return analyses[0], nil
}

// hasCompileStepStatus checks if the compile step for a build has the specified status.
func hasCompileStepStatus(c context.Context, build *buildbucketpb.Build, status buildbucketpb.Status) bool {
	for _, step := range build.Steps {
		if util.IsCompileStep(step) && step.Status == status {
			return true
		}
	}
	return false
}
