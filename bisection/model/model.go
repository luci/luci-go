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
	"context"
	"errors"
	"strings"
	"time"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/bisection/proto/v1"
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
	SuspectType_GenAI      SuspectType = "GenAI"
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

// LuciBuild represents one LUCI build.
// Deprecated. Please use LUCIBuild model instead.
// It is kept here because some old code is still using it.
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

type LUCIBuild struct {
	BuildID       int64                        `gae:"build_id"`
	Project       string                       `gae:"project"`
	Bucket        string                       `gae:"bucket"`
	Builder       string                       `gae:"builder"`
	BuildNumber   int                          `gae:"build_number"`
	GitilesCommit *buildbucketpb.GitilesCommit `gae:"gitiles_commit"`
	CreateTime    time.Time                    `gae:"create_time"`
	EndTime       time.Time                    `gae:"end_time"`
	StartTime     time.Time                    `gae:"start_time"`
	Status        buildbucketpb.Status         `gae:"status"`
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
	// The sheriff rotations that watch the builder.
	SheriffRotations []string `gae:"sheriff_rotations"`
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
	// Key to the suspects that was verified by Culprit verification
	// In some rare cases, there are more than 1 culprit for the regression range.
	VerifiedCulprits []*datastore.Key `gae:"verified_culprits"`
	// Indicates whether the analysis should be cancelled or not,
	// such as in the situation where the corresponding builder start passing again
	ShouldCancel bool `gae:"should_cancel"`
	// Is this analysis for a tree closer failure.
	// If it is, all reruns of this analysis should have higher priority.
	IsTreeCloser bool `gae:"is_tree_closer"`
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
// Initially, we wanted to trigger multiple reruns for different commits in the same build,
// but it is not possible. We can only trigger one rerun per rerun build, i.e. the
// relationship between single rerun : rerun build is 1:1.
type SingleRerun struct {
	Id int64 `gae:"$id"`
	// Key to the parent CompileRerunBuild
	RerunBuild *datastore.Key `gae:"rerun_build"`
	// Type for the rerun build
	// Either culprit verification or nth section run.
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
	Suspect *datastore.Key `gae:"suspect"`
	// Key to NthSectionAnalysis, if this is for nthsection
	NthSectionAnalysis *datastore.Key `gae:"nthsection_analysis"`
	// Priority of this run
	Priority int32 `gae:"priority"`
	// The dimensions of the rerun build.
	Dimensions *pb.Dimensions `gae:"dimensions"`
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

	// HasTakenActions indicates if the all actions for culprit
	// (e.g. comment, revert...) have been taken.
	// This field is only valid if a culprit is confirmed by culprit verification.
	// If there is no action to be taken (e.g. if actions are disabled),
	// then this field is set to true.
	HasTakenActions bool `gae:"has_taken_actions"`
}

// Suspect is the suspect of GenAI analysis or nthsection.
type Suspect struct {
	Id int64 `gae:"$id"`

	// Type of the suspect, either GenAI or nthsection
	Type SuspectType `gae:"type"`

	// Key to the CompileGenAIAnalysis or CompileNthSectionAnalysis
	// or TestNthSectionAnalysis that results in this suspect
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
	// Only applies to GenAI suspect
	Score int `gae:"score"`

	// A short, human-readable string that concisely describes a fact about the
	// suspect. e.g. 'add a/b/x.cc'
	// Only applies to GenAI suspect
	Justification string `gae:"justification,noindex"`

	// Whether if a suspect has been verified
	VerificationStatus SuspectVerificationStatus `gae:"verification_status"`

	// Key to the CompileRerunBuild or TestSingleRerun of the suspect, for culprit verification purpose.
	SuspectRerunBuild *datastore.Key `gae:"suspect_rerun_build"`

	// Key to the CompileRerunBuild or TestSingleRerun of the parent commit of the suspect, for culprit verification purpose.
	ParentRerunBuild *datastore.Key `gae:"parent_rerun_build"`

	// Details of actions performed by LUCI Bisection for this suspect.
	ActionDetails

	// Type of the suspect.
	AnalysisType pb.AnalysisType `gae:"analysis_type"`

	// The time that this suspect was committed.
	// For now, it is only populated for test failure suspects.
	CommitTime time.Time `gae:"commit_time"`

	// For backward compatibility due to removed fields.
	// See https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/gae/service/datastore/pls.go;l=100
	_ datastore.PropertyMap `gae:"-,extra"`
}

// CompileGenAIAnalysis is GenAI analysis for compile failures.
type CompileGenAIAnalysis struct {
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

// TestFailure represents a failure on a test variant.
type TestFailure struct {
	ID int64 `gae:"$id"`
	// The LUCI project of this test variant.
	Project string `gae:"project"`
	// Test ID of the test variant.
	TestID string `gae:"test_id"`
	// Variant hash of the test variant.
	VariantHash string `gae:"variant_hash"`
	// The variant of the test.
	Variant *pb.Variant `gae:"variant"`
	// The name of the test (used in recipe). Note that it is different
	// from TestID.
	TestName string `gae:"test_name"`
	// The name of the test suite that this test variant belongs to.
	// For chromium, this information can be derived from Variant field.
	TestSuiteName string `gae:"test_suite_name"`
	// Hash of the ref to identify the branch in the source control.
	RefHash string `gae:"ref_hash"`
	// The LUCI bucket for the builder of this test failure.
	Bucket string `gae:"bucket"`
	// The name for the builder of this test failure.
	Builder string `gae:"builder"`
	// The branch where this failure happens.
	Ref *pb.SourceRef `gae:"ref"`
	// Start commit position of the regression range exclusive.
	RegressionStartPosition int64 `gae:"regression_start_position"`
	// End commit position of the regression range inclusive.
	RegressionEndPosition int64 `gae:"regression_end_position"`
	// The unexpected test result rate at regression_start_position, between 0 and 1 inclusive.
	StartPositionFailureRate float64 `gae:"start_position_failure_rate"`
	// The unexpected test result rate at regression_end_position, between 0 and 1 inclusive.
	EndPositionFailureRate float64 `gae:"end_position_failure_rate"`
	// When run multiple test variants in a bisection build, the bisection path
	// follows the test variant of the primary test failure.
	IsPrimary bool
	// IsDiverged is true when the bisection path of this test failure diverges from
	// the primary test failure. This suggests that this test failure has a different root cause.
	// We will not attempt to re-bisect this test failure.
	IsDiverged bool `gae:"is_diverged"`
	// Key to the TestFailureAnalysis that analyses this TestFailure.
	AnalysisKey *datastore.Key `gae:"analysis_key"`
	// RedundancyScore of the test failure, between 0 and 1, larger score means more redundant.
	// Only set for primary test failure.
	RedundancyScore float64 `gae:"redundancy_score"`
	// The time when the failure starts, truncated into hours.
	StartHour time.Time `gae:"start_hour"`
	// The time when we get last got the failure result, truncated into hours.
	EndHour time.Time `gae:"end_hour"`
}

// TestFailureAnalysis is the analysis for test failure.
// This stores information that is needed during the analysis, and also
// some metadata for the analysis.
type TestFailureAnalysis struct {
	ID int64 `gae:"$id"`
	// The LUCI project of the test variants this analysis analyses.
	Project string `gae:"project"`
	// The LUCI bucket for the builder that this analysis analyses.
	Bucket string `gae:"bucket"`
	// The name for the builder that this analysis analyses.
	Builder string `gae:"builder"`
	// Key to the primary TestFailure entity that this analysis analyses.
	TestFailure *datastore.Key `gae:"test_failure"`
	// Time when the entity is first created.
	CreateTime time.Time `gae:"create_time"`
	// Time when the analysis starts to run.
	StartTime time.Time `gae:"start_time"`
	// Time when the analysis ends, or canceled.
	EndTime time.Time `gae:"end_time"`
	// Status of the analysis
	Status pb.AnalysisStatus `gae:"status"`
	// Run status of the analysis
	RunStatus pb.AnalysisRunStatus `gae:"run_status"`
	// Key to the suspect that was verified by Culprit verification
	VerifiedCulpritKey *datastore.Key `gae:"verified_culprit_key"`
	// Priority of this run.
	Priority int32 `gae:"priority"`
	// The start commit hash (exclusive) of the regression range that this analysis analyses.
	// It corresponds to the RegressionStartPosition.
	StartCommitHash string `gae:"start_commit_hash"`
	// The end commit hash (inclusive) of the regression range that this analysis analyses.
	// It corresponds to the RegressionEndPosition.
	EndCommitHash string `gae:"end_commit_hash"`
	// An example Buildbucket ID in which the test failed.
	FailedBuildID int64 `gae:"failed_build_id"`
	// The sheriff rotations that watch the builder.
	SheriffRotations []string `gae:"sheriff_rotations"`
}

// TestGenAIAnalysis is GenAI analysis for test failures.
type TestGenAIAnalysis struct {
	ID int64 `gae:"$id"`
	// Key to the parent TestFailureAnalysis.
	ParentAnalysisKey *datastore.Key `gae:"parent_analysis_key"`
	// Time when the analysis starts to run.
	StartTime time.Time `gae:"start_time"`
	// Time when the analysis ends, or canceled.
	EndTime time.Time `gae:"end_time"`
	// Status of the analysis.
	Status pb.AnalysisStatus `gae:"status"`
	// Run status of the analysis.
	RunStatus pb.AnalysisRunStatus `gae:"run_status"`
}

// TestNthSectionAnalysis is nth-section analysis for test failures.
type TestNthSectionAnalysis struct {
	ID int64 `gae:"$id"`
	// Key to the parent TestFailureAnalysis.
	ParentAnalysisKey *datastore.Key `gae:"parent_analysis_key"`
	// Time when the analysis starts to run.
	StartTime time.Time `gae:"start_time"`
	// Time when the analysis ends, or canceled.
	EndTime time.Time `gae:"end_time"`
	// Status of the analysis.
	Status pb.AnalysisStatus `gae:"status"`
	// Run status of the analysis.
	RunStatus pb.AnalysisRunStatus `gae:"run_status"`

	// When storing protobuf message, datastore will compress the data if it is big
	// https://source.corp.google.com/chops_infra_internal/infra/go/src/go.chromium.org/luci/gae/service/datastore/protos.go;l=88
	BlameList *pb.BlameList `gae:"blame_list"`

	// Culprit is the result of nthsection analysis.
	// Nthsection analysis follows the path of the primary test failure,
	// so the culprit here is the culprit of the primary test failure.
	CulpritKey *datastore.Key `gae:"culprit"`
}

// TestSingleRerun represents one rerun for test failures
// at a particular commit.
// A TestSingleRerun corresponds to one buildbucket run, and may
// run multiple test failures at the same time.
// A TestSingleRerun may be for nth-section or for culprit verification.
type TestSingleRerun struct {
	// The buildbucket ID of the rerun.
	ID int64 `gae:"$id"`
	// LUCI build data for the rerun build.
	LUCIBuild `gae:"luci_build"`
	// Type for the rerun build
	// Either culprit verification or nth section run.
	Type RerunBuildType `gae:"rerun_type"`
	// Key to the TestFailureAnalysis of this SingleRerun
	AnalysisKey *datastore.Key `gae:"analysis_key"`
	// Time when the rerun send the result to bisection from recipe.
	ReportTime time.Time `gae:"report_time"`
	// The dimensions of the rerun build.
	Dimensions *pb.Dimensions `gae:"dimensions"`
	// Key to the culprit (Suspect model), if this is for culprit verification
	CulpritKey *datastore.Key `gae:"culprit_key"`
	// Key to TestNthSectionAnalysis, if this is for nthsection.
	NthSectionAnalysisKey *datastore.Key `gae:"nthsection_analysis_key"`
	// Priority of this run.
	Priority int32 `gae:"priority"`
	// Results of the test runs.
	// The TestFailureKey field of test result will be populated when this model
	// is first created.
	// This is useful to know which test failures are running for this rerun without
	// waiting for the result.
	TestResults RerunTestResults `gae:"test_results"`
	// Status of the rerun.
	// If the rerun ended, this result will base on the result
	// of the primary test failure. See pb.RerunStatus for more information.
	Status pb.RerunStatus `gae:"status"`
}

// RerunTestResults captures test results of TestSingleRerun.
type RerunTestResults struct {
	// IsFinalized indicates whether the results have been finalized and
	// is ready to be consumed.
	IsFinalized bool                    `gae:"is_finalized"`
	Results     []RerunSingleTestResult `gae:"results"`
}

// RerunSingleTestResult is the result for one TestFailure.
type RerunSingleTestResult struct {
	// Key to TestFailure model.
	TestFailureKey *datastore.Key `gae:"test_failure_key"`

	// TODO (nqmtuan): Consider breaking this to status level (e.g.
	// unexpected pass count). But for now, keeping the total expected
	// and unexpected count may be enough, as we only support bisection
	// from expected to unexpected.
	// We use number count instead of status to open for possibility
	// to support flakiness bisection in the future (where a test may need
	// to be rerun multiple times to get the flakiness level).
	// The number of expected results. Skipped results are not counted.
	ExpectedCount int64 `gae:"expected_count"`
	// The number of unexpected results. Skipped results are not counted.
	UnexpectedCount int64 `gae:"unexpected_count"`
}

func (cfa *CompileFailureAnalysis) HasEnded() bool {
	return cfa.RunStatus == pb.AnalysisRunStatus_ENDED || cfa.RunStatus == pb.AnalysisRunStatus_CANCELED
}

func (tfa *TestFailureAnalysis) HasEnded() bool {
	return tfa.RunStatus == pb.AnalysisRunStatus_ENDED || tfa.RunStatus == pb.AnalysisRunStatus_CANCELED
}

func (tfa *TestFailureAnalysis) HasStarted() bool {
	return tfa.RunStatus == pb.AnalysisRunStatus_ENDED || tfa.RunStatus == pb.AnalysisRunStatus_CANCELED || tfa.RunStatus == pb.AnalysisRunStatus_STARTED
}

func (ga *CompileGenAIAnalysis) HasEnded() bool {
	return ga.RunStatus == pb.AnalysisRunStatus_ENDED
}

func (nsa *CompileNthSectionAnalysis) HasEnded() bool {
	return nsa.RunStatus == pb.AnalysisRunStatus_ENDED || nsa.RunStatus == pb.AnalysisRunStatus_CANCELED
}

func (rerun *SingleRerun) HasEnded() bool {
	return rerun.Status == pb.RerunStatus_RERUN_STATUS_FAILED || rerun.Status == pb.RerunStatus_RERUN_STATUS_PASSED || rerun.Status == pb.RerunStatus_RERUN_STATUS_INFRA_FAILED || rerun.Status == pb.RerunStatus_RERUN_STATUS_CANCELED
}

func (rerun *TestSingleRerun) HasEnded() bool {
	return rerun.Status == pb.RerunStatus_RERUN_STATUS_FAILED || rerun.Status == pb.RerunStatus_RERUN_STATUS_PASSED || rerun.Status == pb.RerunStatus_RERUN_STATUS_INFRA_FAILED || rerun.Status == pb.RerunStatus_RERUN_STATUS_CANCELED || rerun.Status == pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED
}

func (rerun *TestSingleRerun) HasStarted() bool {
	return rerun.LUCIBuild.Status != buildbucketpb.Status_STATUS_UNSPECIFIED && rerun.LUCIBuild.Status != buildbucketpb.Status_SCHEDULED
}

func (nsa *TestNthSectionAnalysis) HasEnded() bool {
	return nsa.RunStatus == pb.AnalysisRunStatus_ENDED || nsa.RunStatus == pb.AnalysisRunStatus_CANCELED
}

func (ga *TestGenAIAnalysis) HasEnded() bool {
	return ga.RunStatus == pb.AnalysisRunStatus_ENDED || ga.RunStatus == pb.AnalysisRunStatus_CANCELED
}

// TestFailureBundle contains TestFailure models that will be bisected together.
type TestFailureBundle struct {
	// Primary test failure.
	primaryFailure *TestFailure
	// Non-primary test failures.
	otherTestFailures []*TestFailure
	// Contains metadata that is common for this bundle that
	// is not suitable to put in TestFailure model.
	Metadata *BundleMetaData
}

type BundleMetaData struct {
	SheriffRotations []string
}

func (tfb *TestFailureBundle) Add(testFailures []*TestFailure) error {
	for _, tf := range testFailures {
		if tf.IsPrimary {
			if tfb.primaryFailure != nil {
				return errors.New("added more than 1 primary test failure in bundle")
			}
			tfb.primaryFailure = tf
		} else {
			tfb.otherTestFailures = append(tfb.otherTestFailures, tf)
		}
	}
	return nil
}

// Primary returns the primary test failure for the bundle.
func (tfb *TestFailureBundle) Primary() *TestFailure {
	return tfb.primaryFailure
}

// Others returns other test failures for the bundle.
func (tfb *TestFailureBundle) Others() []*TestFailure {
	result := make([]*TestFailure, len(tfb.otherTestFailures))
	// Copy here to prevent callers from adding things to the slice.
	copy(result, tfb.otherTestFailures)
	return result
}

// All return all test failures for the bundle.
func (tfb *TestFailureBundle) All() []*TestFailure {
	if tfb.primaryFailure == nil {
		return tfb.Others()
	}
	return append(tfb.otherTestFailures, tfb.primaryFailure)
}

// NonDiverged returns all non-diverged test failures for the bundle.
func (tfb *TestFailureBundle) NonDiverged() []*TestFailure {
	result := []*TestFailure{}
	if tfb.primaryFailure != nil {
		result = append(result, tfb.primaryFailure)
	}
	for _, other := range tfb.otherTestFailures {
		if !other.IsDiverged {
			result = append(result, other)
		}
	}
	return result
}

func SuspectStatus(rerunStatus pb.RerunStatus, parentRerunStatus pb.RerunStatus) SuspectVerificationStatus {
	if rerunStatus == pb.RerunStatus_RERUN_STATUS_FAILED && parentRerunStatus == pb.RerunStatus_RERUN_STATUS_PASSED {
		return SuspectVerificationStatus_ConfirmedCulprit
	}
	if rerunStatus == pb.RerunStatus_RERUN_STATUS_PASSED || parentRerunStatus == pb.RerunStatus_RERUN_STATUS_FAILED {
		return SuspectVerificationStatus_Vindicated
	}
	if rerunStatus == pb.RerunStatus_RERUN_STATUS_INFRA_FAILED || parentRerunStatus == pb.RerunStatus_RERUN_STATUS_INFRA_FAILED {
		return SuspectVerificationStatus_VerificationError
	}
	if rerunStatus == pb.RerunStatus_RERUN_STATUS_UNSPECIFIED || parentRerunStatus == pb.RerunStatus_RERUN_STATUS_UNSPECIFIED {
		return SuspectVerificationStatus_Unverified
	}
	if rerunStatus == pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED || parentRerunStatus == pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED {
		return SuspectVerificationStatus_VerificationError
	}
	return SuspectVerificationStatus_UnderVerification
}

func PlatformFromOS(ctx context.Context, os string) Platform {
	val := strings.ToLower(os)
	if strings.Contains(val, "linux") || strings.Contains(val, "ubuntu") {
		return PlatformLinux
	}
	if strings.Contains(val, "win") {
		return PlatformWindows
	}
	if strings.Contains(val, "mac") {
		return PlatformMac
	}
	logging.Warningf(ctx, "Unknown OS platform: %s", val)
	return PlatformUnknown
}
