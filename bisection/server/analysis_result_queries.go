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

// TODO (nqmtuan): Perhaps move this file to a different util package
package server

import (
	"context"

	gfim "go.chromium.org/luci/bisection/model"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
)

// GetBuild returns the failed build in the datastore with the given Buildbucket ID
// Note: if the build is not found, this will return (nil, nil)
func GetBuild(c context.Context, bbid int64) (*gfim.LuciFailedBuild, error) {
	build := &gfim.LuciFailedBuild{Id: bbid}
	switch err := datastore.Get(c, build); {
	case err == datastore.ErrNoSuchEntity:
		return nil, nil
	case err != nil:
		return nil, err
	}

	return build, nil
}

// GetAnalysisForBuild returns the failure analysis associated with the given Buildbucket ID
// Note: if the build or its analysis is not found, this will return (nil, nil)
func GetAnalysisForBuild(c context.Context, bbid int64) (*gfim.CompileFailureAnalysis, error) {
	buildModel, err := GetBuild(c, bbid)
	if (err != nil) || (buildModel == nil) {
		return nil, err
	}

	cfModel := &gfim.CompileFailure{
		Id:    bbid,
		Build: datastore.KeyForObj(c, buildModel),
	}
	switch err := datastore.Get(c, cfModel); {
	case err == datastore.ErrNoSuchEntity:
		return nil, nil
	case err != nil:
		return nil, err
	default:
		//continue
	}

	// If the compile failure was "merged" into another compile failure,
	// use the merged one instead.
	cfKey := datastore.KeyForObj(c, cfModel)
	if cfModel.MergedFailureKey != nil {
		cfKey = cfModel.MergedFailureKey
	}

	// Get the analysis for the compile failure
	q := datastore.NewQuery("CompileFailureAnalysis").Eq("compile_failure", cfKey)
	analyses := []*gfim.CompileFailureAnalysis{}
	err = datastore.GetAll(c, q, &analyses)
	if err != nil {
		return nil, err
	}
	if len(analyses) == 0 {
		return nil, nil
	}
	if len(analyses) > 1 {
		logging.Warningf(c, "Found more than one analysis for build %d", bbid)
	}
	return analyses[0], nil
}

// GetHeuristicAnalysis returns the heuristic analysis associated with the given failure analysis
func GetHeuristicAnalysis(c context.Context, analysis *gfim.CompileFailureAnalysis) (*gfim.CompileHeuristicAnalysis, error) {
	// Gets heuristic analysis results.
	q := datastore.NewQuery("CompileHeuristicAnalysis").Ancestor(datastore.KeyForObj(c, analysis))
	heuristicAnalyses := []*gfim.CompileHeuristicAnalysis{}
	err := datastore.GetAll(c, q, &heuristicAnalyses)

	if err != nil {
		return nil, err
	}

	if len(heuristicAnalyses) == 0 {
		// No heuristic analysis
		return nil, nil
	}

	if len(heuristicAnalyses) > 1 {
		logging.Warningf(c, "Found multiple heuristic analysis for analysis %d", analysis.Id)
	}

	heuristicAnalysis := heuristicAnalyses[0]
	return heuristicAnalysis, nil
}

// GetSuspects returns the heuristic suspects identified by the given heuristic analysis
func GetSuspects(c context.Context, heuristicAnalysis *gfim.CompileHeuristicAnalysis) ([]*gfim.Suspect, error) {
	// Getting the suspects for heuristic analysis
	suspects := []*gfim.Suspect{}
	q := datastore.NewQuery("Suspect").Ancestor(datastore.KeyForObj(c, heuristicAnalysis)).Order("-score")
	err := datastore.GetAll(c, q, &suspects)
	if err != nil {
		return nil, err
	}

	return suspects, nil
}

// GetCompileFailureForAnalysis gets CompileFailure for analysisID.
func GetCompileFailureForAnalysis(c context.Context, analysisID int64) (*gfim.CompileFailure, error) {
	analysis := &gfim.CompileFailureAnalysis{
		Id: analysisID,
	}
	err := datastore.Get(c, analysis)
	if err != nil {
		logging.Errorf(c, "Error getting analysis %d: %s", analysisID, err)
		return nil, err
	}
	compileFailure := &gfim.CompileFailure{
		Id: analysis.CompileFailure.IntID(),
		// We need to specify the parent here because this is a multi-part key.
		Build: analysis.CompileFailure.Parent(),
	}
	err = datastore.Get(c, compileFailure)
	if err != nil {
		logging.Errorf(c, "Error getting compile failure for analysisID %d: %s", analysisID, err)
		return nil, err
	}
	return compileFailure, nil
}
