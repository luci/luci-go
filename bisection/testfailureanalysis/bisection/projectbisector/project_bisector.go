// Copyright 2023 The LUCI Authors.
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

// Package projectbisector declare the interface that each individual
// project bisector needs to implement.
package projectbisector

import (
	"context"

	bbpb "go.chromium.org/luci/buildbucket/proto"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/analysis"
)

type RerunOption struct {
	FullRun bool
	BotID   string
}

type ProjectBisector interface {
	// Prepares data for bisection. This may involve populating models with data.
	Prepare(ctx context.Context, tfa *model.TestFailureAnalysis, luciAnalysis analysis.AnalysisClient) error
	// TriggerRerun triggers a rerun build bucket build on a specific commit.
	TriggerRerun(ctx context.Context, tfa *model.TestFailureAnalysis, tfs []*model.TestFailure, gitilesCommit *bbpb.GitilesCommit, option RerunOption) (*bbpb.Build, error)
}
