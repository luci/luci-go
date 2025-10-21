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

// Package testutil contains utility functions for test.
package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
)

func CreateBlamelist(nCommits int) *pb.BlameList {
	blamelist := &pb.BlameList{}
	for i := range nCommits {
		blamelist.Commits = append(blamelist.Commits, &pb.BlameListSingleCommit{
			Commit:     fmt.Sprintf("commit%d", i),
			CommitTime: timestamppb.New(time.Unix(int64(i+1000), 0)),
		})
	}
	blamelist.LastPassCommit = &pb.BlameListSingleCommit{
		Commit: fmt.Sprintf("commit%d", nCommits),
	}
	return blamelist
}

func CreateLUCIFailedBuild(c context.Context, t testing.TB, id int64, project string) *model.LuciFailedBuild {
	t.Helper()
	fb := &model.LuciFailedBuild{
		Id: id,
		LuciBuild: model.LuciBuild{
			Project: project,
		},
		SheriffRotations: []string{"chromium"},
	}
	assert.Loosely(t, datastore.Put(c, fb), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return fb
}

func CreateCompileFailure(c context.Context, t testing.TB, fb *model.LuciFailedBuild) *model.CompileFailure {
	t.Helper()
	cf := &model.CompileFailure{
		Id:    fb.Id,
		Build: datastore.KeyForObj(c, fb),
	}
	assert.Loosely(t, datastore.Put(c, cf), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return cf
}

func CreateCompileFailureAnalysis(c context.Context, t testing.TB, id int64, cf *model.CompileFailure) *model.CompileFailureAnalysis {
	t.Helper()
	cfa := &model.CompileFailureAnalysis{
		Id:             id,
		CompileFailure: datastore.KeyForObj(c, cf),
	}
	assert.Loosely(t, datastore.Put(c, cfa), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return cfa
}

func CreateCompileFailureAnalysisAnalysisChain(c context.Context, t testing.TB, bbid int64, project string, analysisID int64) (*model.LuciFailedBuild, *model.CompileFailure, *model.CompileFailureAnalysis) {
	t.Helper()
	fb := CreateLUCIFailedBuild(c, t, bbid, project)
	cf := CreateCompileFailure(c, t, fb)
	cfa := CreateCompileFailureAnalysis(c, t, analysisID, cf)
	return fb, cf, cfa
}

func CreateNthSectionAnalysis(c context.Context, t testing.TB, cfa *model.CompileFailureAnalysis) *model.CompileNthSectionAnalysis {
	t.Helper()
	nsa := &model.CompileNthSectionAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
	}
	assert.Loosely(t, datastore.Put(c, nsa), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return nsa
}

type CompileGenAIAnalysisCreationOption struct {
	ParentAnalysis *model.CompileFailureAnalysis
	StartTime      time.Time
	EndTime        time.Time
	Status         pb.AnalysisStatus
	RunStatus      pb.AnalysisRunStatus
}

func CreateCompileGenAIAnalysis(c context.Context, t testing.TB, option *CompileGenAIAnalysisCreationOption) *model.CompileGenAIAnalysis {
	t.Helper()
	ga := &model.CompileGenAIAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, option.ParentAnalysis),
		StartTime:      option.StartTime,
		EndTime:        option.EndTime,
		Status:         option.Status,
		RunStatus:      option.RunStatus,
	}
	assert.Loosely(t, datastore.Put(c, ga), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return ga
}

func CreateNthSectionSuspect(c context.Context, t testing.TB, nsa *model.CompileNthSectionAnalysis) *model.Suspect {
	t.Helper()
	suspect := &model.Suspect{
		ParentAnalysis: datastore.KeyForObj(c, nsa),
		Type:           model.SuspectType_NthSection,
	}
	assert.Loosely(t, datastore.Put(c, suspect), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return suspect
}

type GenAISuspectCreationOption struct {
	ParentAnalysis *model.CompileGenAIAnalysis
	Status         model.SuspectVerificationStatus
	CommitID       string
	ReviewURL      string
	ReviewTitle    string
	Justification  string
	Ref            string
}

func CreateGenAISuspect(c context.Context, t testing.TB, option *GenAISuspectCreationOption) *model.Suspect {
	t.Helper()
	suspect := &model.Suspect{
		ParentAnalysis:     datastore.KeyForObj(c, option.ParentAnalysis),
		Type:               model.SuspectType_GenAI,
		VerificationStatus: option.Status,
		GitilesCommit: bbpb.GitilesCommit{
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Id:      option.CommitID,
			Ref:     option.Ref,
		},
		ReviewUrl:     option.ReviewURL,
		ReviewTitle:   option.ReviewTitle,
		Justification: option.Justification,
	}
	assert.Loosely(t, datastore.Put(c, suspect), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return suspect
}

type TestFailureCreationOption struct {
	ID               int64
	Project          string
	Variant          map[string]string
	IsPrimary        bool
	Analysis         *model.TestFailureAnalysis
	Ref              *pb.SourceRef
	TestID           string
	VariantHash      string
	StartHour        time.Time
	RefHash          string
	StartPosition    int64
	EndPosition      int64
	StartFailureRate float64
	EndFailureRate   float64
	IsDiverged       bool
}

func CreateTestFailure(ctx context.Context, t testing.TB, option *TestFailureCreationOption) *model.TestFailure {
	t.Helper()
	id := int64(100)
	project := "chromium"
	variant := map[string]string{}
	isPrimary := false
	var analysisKey *datastore.Key = nil
	var ref *pb.SourceRef = nil
	var testID = ""
	var variantHash = ""
	var startHour time.Time
	var refHash = ""
	var startPosition int64
	var endPosition int64
	var startFailureRate float64
	var endFailureRate float64
	var isDiverged bool

	if option != nil {
		if option.ID != 0 {
			id = option.ID
		}
		if option.Project != "" {
			project = option.Project
		}
		if option.Analysis != nil {
			analysisKey = datastore.KeyForObj(ctx, option.Analysis)
		}
		variant = option.Variant
		isPrimary = option.IsPrimary
		ref = option.Ref
		testID = option.TestID
		variantHash = option.VariantHash
		startHour = option.StartHour
		refHash = option.RefHash
		startPosition = option.StartPosition
		endPosition = option.EndPosition
		startFailureRate = option.StartFailureRate
		endFailureRate = option.EndFailureRate
		isDiverged = option.IsDiverged
	}

	tf := &model.TestFailure{
		ID:      id,
		Project: project,
		Variant: &pb.Variant{
			Def: variant,
		},
		IsPrimary:                isPrimary,
		AnalysisKey:              analysisKey,
		Ref:                      ref,
		TestID:                   testID,
		VariantHash:              variantHash,
		StartHour:                startHour,
		RefHash:                  refHash,
		RegressionStartPosition:  startPosition,
		RegressionEndPosition:    endPosition,
		StartPositionFailureRate: startFailureRate,
		EndPositionFailureRate:   endFailureRate,
		IsDiverged:               isDiverged,
	}

	assert.Loosely(t, datastore.Put(ctx, tf), should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).CatchupIndexes()
	return tf
}

type TestFailureAnalysisCreationOption struct {
	ID                 int64
	Project            string
	Bucket             string
	Builder            string
	TestFailureKey     *datastore.Key
	StartCommitHash    string
	EndCommitHash      string
	FailedBuildID      int64
	Priority           int32
	Status             pb.AnalysisStatus
	RunStatus          pb.AnalysisRunStatus
	CreateTime         time.Time
	StartTime          time.Time
	EndTime            time.Time
	VerifiedCulpritKey *datastore.Key
}

func CreateTestFailureAnalysis(ctx context.Context, t testing.TB, option *TestFailureAnalysisCreationOption) *model.TestFailureAnalysis {
	t.Helper()
	id := int64(1000)
	project := "chromium"
	bucket := "bucket"
	builder := "builder"
	var tfKey *datastore.Key = nil
	startCommitHash := ""
	endCommitHash := ""
	failedBuildID := int64(8000)
	var priority int32
	var status pb.AnalysisStatus
	var runStatus pb.AnalysisRunStatus
	var createTime time.Time
	var startTime time.Time
	var endTime time.Time
	var verifiedCulpritKey *datastore.Key = nil

	if option != nil {
		if option.ID != 0 {
			id = option.ID
		}
		if option.Project != "" {
			project = option.Project
		}
		if option.Bucket != "" {
			bucket = option.Bucket
		}
		if option.Builder != "" {
			builder = option.Builder
		}
		if option.StartCommitHash != "" {
			startCommitHash = option.StartCommitHash
		}
		if option.EndCommitHash != "" {
			endCommitHash = option.EndCommitHash
		}
		if option.FailedBuildID != 0 {
			failedBuildID = option.FailedBuildID
		}
		priority = option.Priority
		tfKey = option.TestFailureKey
		status = option.Status
		runStatus = option.RunStatus
		createTime = option.CreateTime
		startTime = option.StartTime
		endTime = option.EndTime
		verifiedCulpritKey = option.VerifiedCulpritKey
	}

	tfa := &model.TestFailureAnalysis{
		ID:                 id,
		Project:            project,
		Bucket:             bucket,
		Builder:            builder,
		TestFailure:        tfKey,
		StartCommitHash:    startCommitHash,
		EndCommitHash:      endCommitHash,
		FailedBuildID:      failedBuildID,
		Priority:           int32(priority),
		Status:             status,
		RunStatus:          runStatus,
		CreateTime:         createTime,
		StartTime:          startTime,
		EndTime:            endTime,
		VerifiedCulpritKey: verifiedCulpritKey,
	}
	assert.Loosely(t, datastore.Put(ctx, tfa), should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).CatchupIndexes()
	return tfa
}

type TestSingleRerunCreationOption struct {
	ID                    int64
	Status                pb.RerunStatus
	AnalysisKey           *datastore.Key
	Type                  model.RerunBuildType
	TestResult            model.RerunTestResults
	NthSectionAnalysisKey *datastore.Key
	CulpritKey            *datastore.Key
	CreateTime            time.Time
	StartTime             time.Time
	ReportTime            time.Time
	EndTime               time.Time
	BuildStatus           bbpb.Status
	GitilesCommit         *bbpb.GitilesCommit
}

func CreateTestSingleRerun(ctx context.Context, t testing.TB, option *TestSingleRerunCreationOption) *model.TestSingleRerun {
	t.Helper()
	id := int64(1000)
	status := pb.RerunStatus_RERUN_STATUS_UNSPECIFIED
	var analysisKey *datastore.Key
	var rerunType model.RerunBuildType
	testresult := model.RerunTestResults{}
	var nthSectionAnalysisKey *datastore.Key
	var culpritKey *datastore.Key
	var startTime time.Time
	var createTime time.Time
	var reportTime time.Time
	var endTime time.Time
	var buildStatus bbpb.Status
	var gitilesCommit *bbpb.GitilesCommit

	if option != nil {
		if option.ID != 0 {
			id = option.ID
		}
		status = option.Status
		analysisKey = option.AnalysisKey
		rerunType = option.Type
		testresult = option.TestResult
		nthSectionAnalysisKey = option.NthSectionAnalysisKey
		culpritKey = option.CulpritKey
		startTime = option.StartTime
		createTime = option.CreateTime
		reportTime = option.ReportTime
		endTime = option.EndTime
		buildStatus = option.BuildStatus
		gitilesCommit = option.GitilesCommit
	}
	rerun := &model.TestSingleRerun{
		ID:                    id,
		Status:                status,
		AnalysisKey:           analysisKey,
		Type:                  rerunType,
		TestResults:           testresult,
		NthSectionAnalysisKey: nthSectionAnalysisKey,
		CulpritKey:            culpritKey,
		ReportTime:            reportTime,
		LUCIBuild: model.LUCIBuild{
			CreateTime:    createTime,
			StartTime:     startTime,
			EndTime:       endTime,
			Status:        buildStatus,
			GitilesCommit: gitilesCommit,
		},
	}
	assert.Loosely(t, datastore.Put(ctx, rerun), should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).CatchupIndexes()
	return rerun
}

type TestNthSectionAnalysisCreationOption struct {
	ID                int64
	ParentAnalysisKey *datastore.Key
	BlameList         *pb.BlameList
	Status            pb.AnalysisStatus
	RunStatus         pb.AnalysisRunStatus
	StartTime         time.Time
	EndTime           time.Time
	CulpritKey        *datastore.Key
}

func CreateTestNthSectionAnalysis(ctx context.Context, t testing.TB, option *TestNthSectionAnalysisCreationOption) *model.TestNthSectionAnalysis {
	t.Helper()
	id := int64(1000)
	var parentAnalysis *datastore.Key = nil
	var blameList *pb.BlameList
	var status pb.AnalysisStatus
	var runStatus pb.AnalysisRunStatus
	var startTime time.Time
	var endTime time.Time
	var culpritKey *datastore.Key

	if option != nil {
		if option.ID != 0 {
			id = option.ID
		}
		parentAnalysis = option.ParentAnalysisKey
		blameList = option.BlameList
		status = option.Status
		runStatus = option.RunStatus
		startTime = option.StartTime
		endTime = option.EndTime
		culpritKey = option.CulpritKey
	}
	nsa := &model.TestNthSectionAnalysis{
		ID:                id,
		ParentAnalysisKey: parentAnalysis,
		BlameList:         blameList,
		Status:            status,
		RunStatus:         runStatus,
		StartTime:         startTime,
		EndTime:           endTime,
		CulpritKey:        culpritKey,
	}
	assert.Loosely(t, datastore.Put(ctx, nsa), should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).CatchupIndexes()
	return nsa
}

type SuspectCreationOption struct {
	ID                 int64
	ParentKey          *datastore.Key
	CommitID           string
	ReviewURL          string
	ReviewTitle        string
	SuspectRerunKey    *datastore.Key
	ParentRerunKey     *datastore.Key
	VerificationStatus model.SuspectVerificationStatus
	ActionDetails      model.ActionDetails
	AnalysisType       pb.AnalysisType
	Ref                string
}

func CreateSuspect(ctx context.Context, t testing.TB, option *SuspectCreationOption) *model.Suspect {
	t.Helper()
	var parentKey *datastore.Key
	id := int64(500)
	commitID := "1"
	reviewURL := ""
	reviewTitle := ""
	var suspectRerunKey *datastore.Key
	var parentRerunKey *datastore.Key
	var verificationStatus model.SuspectVerificationStatus
	var actionDetails model.ActionDetails
	var analysisType pb.AnalysisType
	ref := "ref"

	if option != nil {
		if option.ID != 0 {
			id = option.ID
		}
		if option.CommitID != "" {
			commitID = option.CommitID
		}
		if option.Ref != "" {
			ref = option.Ref
		}
		parentKey = option.ParentKey
		reviewURL = option.ReviewURL
		reviewTitle = option.ReviewTitle
		suspectRerunKey = option.SuspectRerunKey
		parentRerunKey = option.ParentRerunKey
		verificationStatus = option.VerificationStatus
		actionDetails = option.ActionDetails
		analysisType = option.AnalysisType
	}
	suspect := &model.Suspect{
		Id:             id,
		ParentAnalysis: parentKey,
		GitilesCommit: bbpb.GitilesCommit{
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     ref,
			Id:      commitID,
		},
		ReviewUrl:          reviewURL,
		ReviewTitle:        reviewTitle,
		SuspectRerunBuild:  suspectRerunKey,
		ParentRerunBuild:   parentRerunKey,
		VerificationStatus: verificationStatus,
		ActionDetails:      actionDetails,
		AnalysisType:       analysisType,
	}
	assert.Loosely(t, datastore.Put(ctx, suspect), should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).CatchupIndexes()
	return suspect
}

func UpdateIndices(c context.Context) {
	datastore.GetTestable(c).AddIndexes(
		&datastore.IndexDefinition{
			Kind: "SingleRerun",
			SortBy: []datastore.IndexColumn{
				{
					Property: "analysis",
				},
				{
					Property: "start_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "Suspect",
			SortBy: []datastore.IndexColumn{
				{
					Property: "analysis_type",
				},
				{
					Property: "is_revert_created",
				},
				{
					Property: "revert_create_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "Suspect",
			SortBy: []datastore.IndexColumn{
				{
					Property: "is_revert_committed",
				},
				{
					Property: "revert_commit_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "SingleRerun",
			SortBy: []datastore.IndexColumn{
				{
					Property: "rerun_build",
				},
				{
					Property: "start_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "SingleRerun",
			SortBy: []datastore.IndexColumn{
				{
					Property: "Status",
				},
				{
					Property: "create_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "LuciFailedBuild",
			SortBy: []datastore.IndexColumn{
				{
					Property: "project",
				},
				{
					Property: "bucket",
				},
				{
					Property: "builder",
				},
				{
					Property:   "end_time",
					Descending: true,
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "TestSingleRerun",
			SortBy: []datastore.IndexColumn{
				{
					Property: "nthsection_analysis_key",
				},
				{
					Property: "luci_build.create_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "CompileRerunBuild",
			SortBy: []datastore.IndexColumn{
				{
					Property: "project",
				},
				{
					Property: "status",
				},
				{
					Property: "create_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "TestSingleRerun",
			SortBy: []datastore.IndexColumn{
				{
					Property: "luci_build.project",
				},
				{
					Property: "luci_build.status",
				},
				{
					Property: "luci_build.create_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "TestSingleRerun",
			SortBy: []datastore.IndexColumn{
				{
					Property: "status",
				},
				{
					Property: "luci_build.create_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "TestFailureAnalysis",
			SortBy: []datastore.IndexColumn{
				{
					Property: "project",
				},
				{
					Property: "create_time",
				},
			},
		},
		&datastore.IndexDefinition{
			Kind: "TestFailureAnalysis",
			SortBy: []datastore.IndexColumn{
				{
					Property: "project",
				},
				{
					Property:   "create_time",
					Descending: true,
				},
			},
		},
	)
	datastore.GetTestable(c).CatchupIndexes()
}
