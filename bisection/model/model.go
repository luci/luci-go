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

// Package model contains the datastore model for LUCI Bisection.
package model

import (
	"time"

	pb "go.chromium.org/luci/bisection/proto/v1"

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
	SuspectVerificationStatus_Unverified SuspectVerificationStatus = "Unverified"
	// The suspect is scheduled to be verified (via a task queue)
	SuspectVerificationStatus_VerificationScheduled SuspectVerificationStatus = "Verification Scheduled"
	// The suspect is under verification
	SuspectVerificationStatus_UnderVerification SuspectVerificationStatus = "Under Verification"
	// The suspect is confirmed to be culprit
	SuspectVerificationStatus_ConfirmedCulprit SuspectVerificationStatus = "Confirmed Culprit"
	// This is a false positive - the suspect is not the culprit
	SuspectVerificationStatus_Vindicated SuspectVerificationStatus = "Vindicated"
	// Some error happened during verification
	SuspectVerificationStatus_VerificationError SuspectVerificationStatus = "Verification Error"
	// The verification is canceled
	SuspectVerificationStatus_Canceled SuspectVerificationStatus = "Canceled"
)

type SuspectType string

const (
	SuspectType_Heuristic  SuspectType = "Heuristic"
	SuspectType_NthSection SuspectType = "NthSection"
)

type Platform string

const (
	// The build didn't specified a platform
	PlatformUnspecified Platform = "unspecified"
	// Platform is not win, mac or linux
	PlatformUnknown Platform = "unknown"
	PlatformWindows Platform = "win"
	PlatformMac     Platform = "mac"
	PlatformLinux   Platform = "linux"
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
	BuildFailureType pb.BuildFailureType `gae:"build_failure_type"`
	// The platform of the failure
	Platform Platform `gae:"platform"`
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

	// The list of source files resulting in compile failures
	FailedFiles []string `gae:"failed_files"`

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
	// Time when the analysis ends, or canceled.
	EndTime time.Time `gae:"end_time"`
	// Status of the analysis
	Status pb.AnalysisStatus `gae:"status"`
	// Run status of the analysis
	RunStatus pb.AnalysisRunStatus `gae:"run_status"`
	// Id of the build in which the compile failures occurred the first time in
	// a sequence of consecutive failed builds.
	FirstFailedBuildId int64 `gae:"first_failed_build_id"`
	// Id of the latest build in which the failures did not happen.
	LastPassedBuildId int64 `gae:"last_passed_build_id"`
	// Initial regression range to find the culprit
	InitialRegressionRange *pb.RegressionRange `gae:"initial_regression_range"`
	// Key to the heuristic suspects that was verified by Culprit verification
	// In some rare cases, there are more than 1 culprit for the regression range.
	VerifiedCulprits []*datastore.Key `gae:"verified_culprits"`
	// Indicates whether the analysis should be cancelled or not,
	// such as in the situation where the corresponding builder start passing again
	ShouldCancel bool `gae:"should_cancel"`
	// Is this analysis for a tree closer failure.
	// If it is, all reruns of this analysis should have higher priority.
	IsTreeCloser bool `gae:"is_tree_closer"`
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
	// LUCI build data
	LuciBuild
	// For backward compatibility due to removed fields.
	// See https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/gae/service/datastore/pls.go;l=100
	_ datastore.PropertyMap `gae:"-,extra"`
}

// SingleRerun represents one rerun for a particular compile/test failures for a particular commit.
// Multiple SingleRerun may present in one RerunBuild for different commits.
type SingleRerun struct {
	Id int64 `gae:"$id"`
	// Key to the parent CompileRerunBuild
	RerunBuild *datastore.Key `gae:"rerun_build"`
	// Type for the rerun build
	// We need this for each rerun because we can mix running culprit verification
	// rerun and nth-section rerun in one build
	Type RerunBuildType `gae:"rerun_type"`
	// Key to the CompileFailureAnalysis of this SingleRerun
	// This is mainly used for getting all reruns for an analysis,
	// for the purpose of nth-section analysis
	Analysis *datastore.Key `gae:"analysis"`
	// The commit that this SingleRerun runs on
	buildbucketpb.GitilesCommit
	// Time when the rerun was created
	CreateTime time.Time `gae:"create_time"`
	// Time when the rerun starts.
	StartTime time.Time `gae:"start_time"`
	// Time when the rerun ends.
	EndTime time.Time `gae:"end_time"`
	// Status of the rerun
	Status pb.RerunStatus
	// Key to the Suspect, if this is for culprit verification
	// Note: We have the keys to suspect and nthsection here instead of CompileRerunBuild
	// because in theory, we can mix culprit verification and nthsection runs
	// within the same build
	Suspect *datastore.Key `gae:"suspect"`
	// Key to NthSectionAnalysis, if this is for nthsection
	NthSectionAnalysis *datastore.Key `gae:"nthsection_analysis"`
	// Priority of this run
	Priority int32 `gae:"priority"`
}

// ActionDetails encapsulate the details of actions performed by LUCI Bisection,
// e.g. creating a revert, commenting on a culprit, etc.
type ActionDetails struct {
	// URL to the code review of the revert.
	RevertURL string `gae:"revert_url"`

	// Whether LUCI Bisection has created the revert
	IsRevertCreated bool `gae:"is_revert_created"`

	// Time when the revert was created
	RevertCreateTime time.Time `gae:"revert_create_time"`

	// Whether LUCI Bisection has committed the revert
	IsRevertCommitted bool `gae:"is_revert_committed"`

	// Time when the revert for the suspect was bot-committed
	RevertCommitTime time.Time `gae:"revert_commit_time"`

	// Whether LUCI Bisection has added a supporting comment to an existing revert
	HasSupportRevertComment bool `gae:"has_support_revert_comment"`

	// Time when LUCI Bisection added a supporting comment to an existing revert
	SupportRevertCommentTime time.Time `gae:"support_revert_comment_time"`

	// Whether LUCI Bisection has added a comment to the culprit CL
	HasCulpritComment bool `gae:"has_culprit_comment"`

	// Time when LUCI Bisection commented on the culprit
	CulpritCommentTime time.Time `gae:"culprit_comment_time"`

	// Optional explanation for when processing the culprit results in no action.
	InactionReason pb.CulpritInactionReason `gae:"inaction_reason"`
}

// Suspect is the suspect of heuristic analysis.
type Suspect struct {
	Id int64 `gae:"$id"`

	// Type of the suspect, either heuristic or nthsection
	Type SuspectType `gae:"type"`

	// Key to the CompileFailureHeuristicAnalysis or CompileFailureNthSectionAnalysis
	// that results in this suspect
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
	// Only applies to Heuristic suspect
	Score int `gae:"score"`

	// A short, human-readable string that concisely describes a fact about the
	// suspect. e.g. 'add a/b/x.cc'
	// Only applies to Heuristic suspect
	Justification string `gae:"justification,noindex"`

	// Whether if a suspect has been verified
	VerificationStatus SuspectVerificationStatus `gae:"verification_status"`

	// Key to the CompileRerunBuild of the suspect, for culprit verification purpose.
	SuspectRerunBuild *datastore.Key `gae:"suspect_rerun_build"`

	// Key to the CompileRerunBuild of the parent commit of the suspect, for culprit verification purpose.
	ParentRerunBuild *datastore.Key `gae:"parent_rerun_build"`

	// Details of actions performed by LUCI Bisection for this suspect.
	ActionDetails

	// For backward compatibility due to removed fields.
	// See https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/gae/service/datastore/pls.go;l=100
	_ datastore.PropertyMap `gae:"-,extra"`
}

// CompileHeuristicAnalysis is heuristic analysis for compile failures.
type CompileHeuristicAnalysis struct {
	Id int64 `gae:"$id"`
	// Key to the parent CompileFailureAnalysis
	ParentAnalysis *datastore.Key `gae:"$parent"`
	// Time when the analysis starts to run.
	StartTime time.Time `gae:"start_time"`
	// Time when the analysis ends, or canceled
	EndTime time.Time `gae:"end_time"`
	// Status of the analysis
	Status pb.AnalysisStatus `gae:"status"`
	// Run status of the analysis
	RunStatus pb.AnalysisRunStatus `gae:"run_status"`
}

// CompileNthSectionAnalysis is nth-section analysis for compile failures.
type CompileNthSectionAnalysis struct {
	Id int64 `gae:"$id"`
	// Key to the parent CompileFailureAnalysis
	ParentAnalysis *datastore.Key `gae:"$parent"`
	// Time when the analysis starts to run.
	StartTime time.Time `gae:"start_time"`
	// Time when the analysis ends, or canceled
	EndTime time.Time `gae:"end_time"`
	// Status of the analysis
	Status pb.AnalysisStatus `gae:"status"`
	// Run status of the analysis
	RunStatus pb.AnalysisRunStatus `gae:"run_status"`

	// When storing protobuf message, datastore will compress the data if it is big
	// https://source.corp.google.com/chops_infra_internal/infra/go/src/go.chromium.org/luci/gae/service/datastore/protos.go;l=88
	// We can also declare zstd compression here, but there seems to be a bug where
	// the message size is 0
	BlameList *pb.BlameList `gae:"blame_list"`

	// Suspect is the result of nthsection analysis.
	// Note: We call it "suspect" because it has not been verified (by culprit verification component)
	Suspect *datastore.Key `gae:"suspect"`
}

func (cfa *CompileFailureAnalysis) HasEnded() bool {
	return cfa.RunStatus == pb.AnalysisRunStatus_ENDED || cfa.RunStatus == pb.AnalysisRunStatus_CANCELED
}

func (ha *CompileHeuristicAnalysis) HasEnded() bool {
	return ha.RunStatus == pb.AnalysisRunStatus_ENDED || ha.RunStatus == pb.AnalysisRunStatus_CANCELED
}

func (nsa *CompileNthSectionAnalysis) HasEnded() bool {
	return nsa.RunStatus == pb.AnalysisRunStatus_ENDED || nsa.RunStatus == pb.AnalysisRunStatus_CANCELED
}

func (rerun *SingleRerun) HasEnded() bool {
	return rerun.Status == pb.RerunStatus_RERUN_STATUS_FAILED || rerun.Status == pb.RerunStatus_RERUN_STATUS_PASSED || rerun.Status == pb.RerunStatus_RERUN_STATUS_INFRA_FAILED || rerun.Status == pb.RerunStatus_RERUN_STATUS_CANCELED
}
