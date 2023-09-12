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
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/service/datastore"
)

func CreateBlamelist(nCommits int) *pb.BlameList {
	blamelist := &pb.BlameList{}
	for i := 0; i < nCommits; i++ {
		blamelist.Commits = append(blamelist.Commits, &pb.BlameListSingleCommit{
			Commit: fmt.Sprintf("commit%d", i),
		})
	}
	return blamelist
}

func CreateLUCIFailedBuild(c context.Context, id int64, project string) *model.LuciFailedBuild {
	fb := &model.LuciFailedBuild{
		Id: id,
		LuciBuild: model.LuciBuild{
			Project: project,
		},
	}
	So(datastore.Put(c, fb), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return fb
}

func CreateCompileFailure(c context.Context, fb *model.LuciFailedBuild) *model.CompileFailure {
	cf := &model.CompileFailure{
		Id:    fb.Id,
		Build: datastore.KeyForObj(c, fb),
	}
	So(datastore.Put(c, cf), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return cf
}

func CreateCompileFailureAnalysis(c context.Context, id int64, cf *model.CompileFailure) *model.CompileFailureAnalysis {
	cfa := &model.CompileFailureAnalysis{
		Id:             id,
		CompileFailure: datastore.KeyForObj(c, cf),
	}
	So(datastore.Put(c, cfa), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return cfa
}

func CreateCompileFailureAnalysisAnalysisChain(c context.Context, bbid int64, project string, analysisID int64) (*model.LuciFailedBuild, *model.CompileFailure, *model.CompileFailureAnalysis) {
	fb := CreateLUCIFailedBuild(c, bbid, project)
	cf := CreateCompileFailure(c, fb)
	cfa := CreateCompileFailureAnalysis(c, analysisID, cf)
	return fb, cf, cfa
}

func CreateHeuristicAnalysis(c context.Context, cfa *model.CompileFailureAnalysis) *model.CompileHeuristicAnalysis {
	ha := &model.CompileHeuristicAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
	}
	So(datastore.Put(c, ha), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return ha
}

func CreateNthSectionAnalysis(c context.Context, cfa *model.CompileFailureAnalysis) *model.CompileNthSectionAnalysis {
	nsa := &model.CompileNthSectionAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
	}
	So(datastore.Put(c, nsa), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return nsa
}

func CreateHeuristicSuspect(c context.Context, ha *model.CompileHeuristicAnalysis, status model.SuspectVerificationStatus) *model.Suspect {
	suspect := &model.Suspect{
		ParentAnalysis:     datastore.KeyForObj(c, ha),
		Type:               model.SuspectType_Heuristic,
		VerificationStatus: status,
	}
	So(datastore.Put(c, suspect), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return suspect
}

func CreateNthSectionSuspect(c context.Context, nsa *model.CompileNthSectionAnalysis) *model.Suspect {
	suspect := &model.Suspect{
		ParentAnalysis: datastore.KeyForObj(c, nsa),
		Type:           model.SuspectType_NthSection,
	}
	So(datastore.Put(c, suspect), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return suspect
}

type TestFailureCreationOption struct {
	ID          int64
	Project     string
	Variant     map[string]string
	IsPrimary   bool
	Analysis    *model.TestFailureAnalysis
	Ref         *pb.SourceRef
	TestID      string
	VariantHash string
}

func CreateTestFailure(ctx context.Context, option *TestFailureCreationOption) *model.TestFailure {
	id := int64(100)
	project := "chromium"
	variant := map[string]string{}
	isPrimary := false
	var analysisKey *datastore.Key = nil
	var ref *pb.SourceRef = nil
	var testID = ""
	var variantHash = ""

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
	}

	tf := &model.TestFailure{
		ID:      id,
		Project: project,
		Variant: &pb.Variant{
			Def: variant,
		},
		IsPrimary:   isPrimary,
		AnalysisKey: analysisKey,
		Ref:         ref,
		TestID:      testID,
		VariantHash: variantHash,
	}

	So(datastore.Put(ctx, tf), ShouldBeNil)
	datastore.GetTestable(ctx).CatchupIndexes()
	return tf
}

type TestFailureAnalysisCreationOption struct {
	ID              int64
	Project         string
	TestFailureKey  *datastore.Key
	StartCommitHash string
	EndCommitHash   string
	FailedBuildID   int64
	Priority        int32
	Status          pb.AnalysisStatus
	RunStatus       pb.AnalysisRunStatus
	CreateTime      time.Time
}

func CreateTestFailureAnalysis(ctx context.Context, option *TestFailureAnalysisCreationOption) *model.TestFailureAnalysis {
	id := int64(1000)
	project := "chromium"
	var tfKey *datastore.Key = nil
	startCommitHash := ""
	endCommitHash := ""
	failedBuildID := int64(8000)
	var priority int32
	var status pb.AnalysisStatus
	var runStatus pb.AnalysisRunStatus
	var createTime time.Time

	if option != nil {
		if option.ID != 0 {
			id = option.ID
		}
		if option.Project != "" {
			project = option.Project
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
	}

	tfa := &model.TestFailureAnalysis{
		ID:              id,
		Project:         project,
		TestFailure:     tfKey,
		StartCommitHash: startCommitHash,
		EndCommitHash:   endCommitHash,
		FailedBuildID:   failedBuildID,
		Priority:        int32(priority),
		Status:          status,
		RunStatus:       runStatus,
		CreateTime:      createTime,
	}
	So(datastore.Put(ctx, tfa), ShouldBeNil)
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
}

func CreateTestSingleRerun(ctx context.Context, option *TestSingleRerunCreationOption) *model.TestSingleRerun {
	id := int64(1000)
	status := pb.RerunStatus_RERUN_STATUS_UNSPECIFIED
	var analysisKey *datastore.Key
	var rerunType model.RerunBuildType
	testresult := model.RerunTestResults{}
	var nthSectionAnalysisKey *datastore.Key
	var culpritKey *datastore.Key
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
	}
	rerun := &model.TestSingleRerun{
		ID:                    id,
		Status:                status,
		AnalysisKey:           analysisKey,
		Type:                  rerunType,
		TestResults:           testresult,
		NthSectionAnalysisKey: nthSectionAnalysisKey,
		CulpritKey:            culpritKey,
	}
	So(datastore.Put(ctx, rerun), ShouldBeNil)
	datastore.GetTestable(ctx).CatchupIndexes()
	return rerun
}

type TestNthSectionAnalysisCreationOption struct {
	ID                int64
	ParentAnalysisKey *datastore.Key
	BlameList         *pb.BlameList
	Status          pb.AnalysisStatus
	RunStatus       pb.AnalysisRunStatus
}

func CreateTestNthSectionAnalysis(ctx context.Context, option *TestNthSectionAnalysisCreationOption) *model.TestNthSectionAnalysis {
	id := int64(1000)
	var parentAnalysis *datastore.Key = nil
	var blameList *pb.BlameList
	var status pb.AnalysisStatus
	var runStatus pb.AnalysisRunStatus


	if option != nil {
		if option.ID != 0 {
			id = option.ID
		}
		parentAnalysis = option.ParentAnalysisKey
		blameList = option.BlameList
		status = option.Status
		runStatus = option.RunStatus
	}
	nsa := &model.TestNthSectionAnalysis{
		ID:                id,
		ParentAnalysisKey: parentAnalysis,
		BlameList:         blameList,
		Status: status,
		RunStatus: runStatus,
	}
	So(datastore.Put(ctx, nsa), ShouldBeNil)
	datastore.GetTestable(ctx).CatchupIndexes()
	return nsa
}

type SuspectCreationOption struct {
	ID        int64
	ParentKey *datastore.Key
	CommitID  string
}

func CreateSuspect(ctx context.Context, option *SuspectCreationOption) *model.Suspect {
	var parentKey *datastore.Key
	id := int64(500)
	commitID := "1"
	if option != nil {
		if option.ID != 0 {
			id = option.ID
		}
		if option.CommitID != "" {
			commitID = option.CommitID
		}
		parentKey = option.ParentKey
	}
	suspect := &model.Suspect{
		Id:             id,
		ParentAnalysis: parentKey,
		GitilesCommit: bbpb.GitilesCommit{
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Id:      commitID,
		},
	}
	So(datastore.Put(ctx, suspect), ShouldBeNil)
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
	)
	datastore.GetTestable(c).CatchupIndexes()
}
