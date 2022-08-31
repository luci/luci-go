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

// package model contains the datastore model for GoFindit.
package model

import (
	"time"

	gofinditpb "go.chromium.org/luci/bisection/proto"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/service/datastore"
)

type RerunBuildType string

const (
	RerunBuildType_CulpritVerification RerunBuildType = "Culprit Verification"
	RerunBuildType_NthSection          RerunBuildType = "NthSection"
)

type SuspectVerificationStatus string

const (
	// The suspect is not verified and no verification is happening
	SuspectVerificationStatus_Unverified = "Unverified"
	// The suspect is under verification
	SuspectVerificationStatus_UnderVerification = "Under Verification"
	// The suspect is confirmed to be culprit
	SuspectVerificationStatus_ConfirmedCulprit = "Confirmed Culprit"
	// This is a false positive - the suspect is not the culprit
	SuspectVerificationStatus_Vindicated = "Vindicated"
	// Some error happened during verification
	SuspectVerificationStatus_VerificationError = "Verification Error"
)

// LuciBuild represents one LUCI build
type LuciBuild struct {
	BuildId     int64  `gae:"build_id"`
	Project     string `gae:"project"`
	Bucket      string `gae:"bucket"`
	Builder     string `gae:"builder"`
	BuildNumber int    `gae:"build_number"`
	buildbucketpb.GitilesCommit
	CreateTime time.Time            `gae:"create_time"`
	EndTime    time.Time            `gae:"end_time"`
	StartTime  time.Time            `gae:"start_time"`
	Status     buildbucketpb.Status `gae:"status"`
}

type LuciFailedBuild struct {
	// Id is the build Id
	Id int64 `gae:"$id"`
	LuciBuild
	// Obsolete field - specify BuildFailureType instead
	FailureType string `gae:"failure_type"`
	// Failure type for the build
	BuildFailureType gofinditpb.BuildFailureType `gae:"build_failure_type"`
}

// CompileFailure represents a compile failure in one or more targets.
type CompileFailure struct {
	// Id is the build Id of the compile failure
	Id int64 `gae:"$id"`
	// The key to LuciFailedBuild that the failure belongs to.
	Build *datastore.Key `gae:"$parent"`

	// The list of output targets that failed to compile.
	// This is to speed up the compilation process, as we only want to rerun failed targets.
	OutputTargets []string `gae:"output_targets"`

	// Compile rule, e.g. ACTION, CXX, etc.
	// For chromium builds, it can be found in json.output[ninja_info] log of
	// compile step.
	// For chromeos builds, it can be found in an output property 'compile_failure'
	// of the build.
	Rule string `gae:"rule"`

	// Only for CC and CXX rules
	// These are the source files that this compile failure uses as input
	Dependencies []string `gae:"dependencies"`

	// Key to the CompileFailure that this failure merges into.
	// If this exists, no analysis on current failure, instead use the results
	// of merged_failure.
	MergedFailureKey *datastore.Key `gae:"merged_failure_key"`
}

// CompileFailureAnalysis is the analysis for CompileFailure.
// This stores information that is needed during the analysis, and also
// some metadata for the analysis.
type CompileFailureAnalysis struct {
	Id int64 `gae:"$id"`
	// Key to the CompileFailure that this analysis analyses.
	CompileFailure *datastore.Key `gae:"compile_failure"`
	// Time when the analysis is created.
	CreateTime time.Time `gae:"create_time"`
	// Time when the analysis starts to run.
	StartTime time.Time `gae:"start_time"`
	// Time when the analysis runs to the end.
	EndTime time.Time `gae:"end_time"`
	// Status of the analysis
	Status gofinditpb.AnalysisStatus `gae:"status"`
	// Id of the build in which the compile failures occurred the first time in
	// a sequence of consecutive failed builds.
	FirstFailedBuildId int64 `gae:"first_failed_build_id"`
	// Id of the latest build in which the failures did not happen.
	LastPassedBuildId int64 `gae:"last_passed_build_id"`
	// Initial regression range to find the culprit
	InitialRegressionRange *gofinditpb.RegressionRange `gae:"initial_regression_range"`
}

// CompileFailureInRerunBuild is a compile failure in a rerun build.
// Since we only need to keep a simple record on what's failed in rerun build,
// there is no need to reuse CompileFailure.
type CompileFailureInRerunBuild struct {
	// Json string of the failed output target
	// We store as json string instead of []string to avoid the "slice within
	// slice" error when saving to datastore
	OutputTargets string `gae:"output_targets"`
}

// CompileRerunBuild is one rerun build for CompileFailureAnalysis.
// The rerun build may be for nth-section analysis or for culprit verification.
type CompileRerunBuild struct {
	// Id is the buildbucket Id for the rerun build.
	Id int64 `gae:"$id"`
	// Type for the rerun build
	Type RerunBuildType `gae:"rerun_type"`
	// Key to the Suspect, if this is for culprit verification
	Suspect *datastore.Key `gae:"suspect"`
	// LUCI build data
	LuciBuild
}

// SingleRerun represents one rerun for a particular compile/test failures for a particular commit.
// Multiple SingleRerun may present in one RerunBuild for different commits.
type SingleRerun struct {
	Id int64 `gae:"$id"`
	// Key to the parent CompileRerunBuild
	RerunBuild *datastore.Key `gae:"rerun_build"`
	// The commit that this SingleRerun runs on
	buildbucketpb.GitilesCommit
	// Time when the rerun starts.
	StartTime time.Time `gae:"start_time"`
	// Time when the rerun ends.
	EndTime time.Time `gae:"end_time"`
	// Status of the rerun
	Status gofinditpb.RerunStatus
}

// Culprit is the culprit of rerun analysis.
type Culprit struct {
	// Key to the CompileFailureAnalysis that results in this culprit.
	ParentAnalysis *datastore.Key `gae:"$parent"`
	buildbucketpb.GitilesCommit
}

// Suspect is the suspect of heuristic analysis.
type Suspect struct {
	Id int64 `gae:"$id"`

	// Key to the CompileFailureHeuristicAnalysis that results in this suspect.
	ParentAnalysis *datastore.Key `gae:"$parent"`

	// The commit of the suspect
	buildbucketpb.GitilesCommit

	// The Url where the suspect was reviewed
	ReviewUrl string `gae:"review_url"`

	// Title of the review for the suspect
	ReviewTitle string `gae:"review_title"`

	// Score is an integer representing the how confident we believe the suspect
	// is indeed the culprit.
	// A higher score means a stronger signal that the suspect is responsible for
	// a failure.
	Score int `gae:"score"`

	// A short, human-readable string that concisely describes a fact about the
	// suspect. e.g. 'add a/b/x.cc'
	Justification string `gae:"justification,noindex"`

	// Whether if a suspect has been verified
	VerificationStatus SuspectVerificationStatus `gae:"verification_status"`

	// Key to the CompileRerunBuild of the suspect, for culprit verification purpose.
	SuspectRerunBuild *datastore.Key `gae:"suspect_rerun_build"`

	// Key to the CompileRerunBuild of the parent commit of the suspect, for culprit verification purpose.
	ParentRerunBuild *datastore.Key `gae:"parent_rerun_build"`
}

// CompileHeuristicAnalysis is heuristic analysis for compile failures.
type CompileHeuristicAnalysis struct {
	Id int64 `gae:"$id"`
	// Key to the parent CompileFailureAnalysis
	ParentAnalysis *datastore.Key `gae:"$parent"`
	// Time when the analysis starts to run.
	StartTime time.Time `gae:"start_time"`
	// Time when the analysis ends.
	EndTime time.Time `gae:"end_time"`
	// Status of the analysis
	Status gofinditpb.AnalysisStatus `gae:"status"`
}

// CompileNthSectionAnalysis is nth-section analysis for compile failures.
type CompileNthSectionAnalysis struct {
	Id int64 `gae:"$id"`
	// Key to the parent CompileFailureAnalysis
	ParentAnalysis *datastore.Key `gae:"$parent"`
	// Time when the analysis starts to run.
	StartTime time.Time `gae:"start_time"`
	// Time when the analysis ends.
	EndTime time.Time `gae:"end_time"`
	// Status of the analysis
	Status gofinditpb.AnalysisStatus `gae:"status"`
}
