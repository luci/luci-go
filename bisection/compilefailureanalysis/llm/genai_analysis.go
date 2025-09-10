// Copyright 2025 The LUCI Authors.
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

// Package genai provides LLM-powered analysis for compile failure bisection.
package llm

import (
	"context"
	"fmt"
	"strings"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/compilefailureanalysis/compilelog"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/changelogutil"
)

const (
	promptTemplate = `You are an experienced Software engineer who is looking into the build failures and below are the details.

Failure: %s

Blamelist: %s

Find the culprit CL from the log range which caused the compile failure, just let me know the Commit ID without any other information.`
)

// Analyze performs LLM-powered analysis to identify the culprit CL
func Analyze(c context.Context, client Client, cfa *model.CompileFailureAnalysis, regressionRange *pb.RegressionRange, compileLogs *model.CompileLogs) (*model.CompileGenAIAnalysis, error) {
	logging.Infof(c, "Starting GenAI analysis for compile failure")

	// Create the GenAI analysis record
	genaiAnalysis := &model.CompileGenAIAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
		StartTime:      clock.Now(c),
		Status:         pb.AnalysisStatus_RUNNING,
		RunStatus:      pb.AnalysisRunStatus_STARTED,
	}

	// Save the initial analysis record to datastore
	if err := datastore.Put(c, genaiAnalysis); err != nil {
		return nil, errors.Fmt("failed to save GenAI analysis to datastore: %w", err)
	}

	// Get changelogs for the regression range
	changelogs, err := changelogutil.GetChangeLogs(c, regressionRange, false)
	if err != nil {
		setStatusError(c, genaiAnalysis)
		return genaiAnalysis, errors.Fmt("failed to get changelogs: %w", err)
	}

	// Prepare blamelist from changelogs
	blamelist, err := prepareBlamelist(changelogs)
	if err != nil {
		setStatusError(c, genaiAnalysis)
		return genaiAnalysis, errors.Fmt("failed to prepare blamelist: %w", err)
	}

	// Gets compile logs from logdog
	if compileLogs == nil {
		compileLogs, err = compilelog.GetCompileLogs(c, cfa.FirstFailedBuildId)
		if err != nil {
			setStatusError(c, genaiAnalysis)
			return genaiAnalysis, errors.Fmt("failed getting compile log: %w", err)
		}
	}

	// Prepare stack trace from compile logs
	stackTrace, err := prepareStackTrace(compileLogs)
	if err != nil {
		setStatusError(c, genaiAnalysis)
		return genaiAnalysis, errors.Fmt("failed to prepare stack trace: %w", err)
	}

	// Generate prompt from template
	prompt := fmt.Sprintf(promptTemplate, stackTrace, blamelist)

	// Call genai model
	suspectCommitID, err := client.GenerateContent(c, prompt)

	if err != nil {
		setStatusError(c, genaiAnalysis)
		return genaiAnalysis, errors.Fmt("failed to call GenAI: %w", err)
	}
	// Process the raw string, the commit should be a valid ID to GitlesCommit

	logging.Infof(c, "GenAI analysis identified culprit CL: %s", suspectCommitID)

	// Find the changelog for the suspect commit to extract review info
	var reviewUrl, reviewTitle string
	for _, cl := range changelogs {
		if cl.Commit == suspectCommitID {
			if url, err := cl.GetReviewUrl(); err == nil {
				reviewUrl = url
			}
			if title, err := cl.GetReviewTitle(); err == nil {
				reviewTitle = title
			}
			break
		}
	}

	// Save suspect to datastore
	suspect := &model.Suspect{
		ParentAnalysis: datastore.KeyForObj(c, genaiAnalysis),
		ReviewUrl:      reviewUrl,
		ReviewTitle:    reviewTitle,
		Score:          30, // Default HighConfidence for PriorityCulpritVerification
		GitilesCommit: buildbucketpb.GitilesCommit{
			Host:    regressionRange.LastPassed.Host,
			Project: regressionRange.LastPassed.Project,
			Ref:     regressionRange.LastPassed.Ref,
			Id:      suspectCommitID,
		},
		VerificationStatus: model.SuspectVerificationStatus_Unverified,
		Type:               model.SuspectType_GenAI,
		AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
	}
	datastore.Put(c, suspect)

	// Update analysis record with completion status
	genaiAnalysis.Status = pb.AnalysisStatus_FOUND
	genaiAnalysis.RunStatus = pb.AnalysisRunStatus_ENDED
	genaiAnalysis.EndTime = clock.Now(c)

	if err := datastore.Put(c, genaiAnalysis); err != nil {
		return genaiAnalysis, errors.Fmt("failed to update GenAI analysis in datastore: %w", err)
	}

	return genaiAnalysis, nil
}

// prepareStackTrace extracts and formats compile failure information
func prepareStackTrace(compileLogs *model.CompileLogs) (string, error) {
	if compileLogs == nil {
		return "", errors.New("compile logs are nil")
	}

	var stackTraceBuilder strings.Builder

	// Add failure summary log failures if available
	if compileLogs.FailureSummaryLog != "" {
		stackTraceBuilder.WriteString("Failure Summary Log:\n")
		stackTraceBuilder.WriteString(compileLogs.FailureSummaryLog)
		stackTraceBuilder.WriteString("\n\n")
	}

	result := stackTraceBuilder.String()
	if result == "" {
		return "", errors.New("no compile failure summary available")
	}

	return result, nil
}

// prepareBlamelist formats changeLogs into a readable blamelist
func prepareBlamelist(changeLogs []*model.ChangeLog) (string, error) {
	if len(changeLogs) == 0 {
		return "", errors.New("no changelogs available")
	}

	var blamelistBuilder strings.Builder
	blamelistBuilder.WriteString("Commits in regression range (newest to oldest):\n\n")

	for i, cl := range changeLogs {
		blamelistBuilder.WriteString(fmt.Sprintf("CL %d:\n", i+1))
		blamelistBuilder.WriteString(fmt.Sprintf("  Commit ID: %s\n", cl.Commit))
		blamelistBuilder.WriteString(fmt.Sprintf("  Time: %s\n", cl.Author.Time))

		// Extract and format commit message title
		title, err := cl.GetReviewTitle()
		if err != nil {
			// Fallback to first line of message
			lines := strings.Split(cl.Message, "\n")
			if len(lines) > 0 {
				title = lines[0]
			}
		}
		blamelistBuilder.WriteString(fmt.Sprintf("  Message: %s\n", title))

		// Add changed files
		if len(cl.ChangeLogDiffs) > 0 {
			blamelistBuilder.WriteString("  Changed files:\n")
			for _, diff := range cl.ChangeLogDiffs {
				switch diff.Type {
				case model.ChangeType_ADD:
					blamelistBuilder.WriteString(fmt.Sprintf("    ADD: %s\n", diff.NewPath))
				case model.ChangeType_MODIFY:
					blamelistBuilder.WriteString(fmt.Sprintf("    MODIFY: %s\n", diff.NewPath))
				case model.ChangeType_DELETE:
					blamelistBuilder.WriteString(fmt.Sprintf("    DELETE: %s\n", diff.OldPath))
				case model.ChangeType_RENAME:
					blamelistBuilder.WriteString(fmt.Sprintf("    RENAME: %s -> %s\n", diff.OldPath, diff.NewPath))
				case model.ChangeType_COPY:
					blamelistBuilder.WriteString(fmt.Sprintf("    COPY: %s -> %s\n", diff.OldPath, diff.NewPath))
				}
			}
		}
		blamelistBuilder.WriteString("\n")
	}

	return blamelistBuilder.String(), nil
}

// setStatusError sets the analysis status to ERROR and saves it to datastore
func setStatusError(c context.Context, genaiAnalysis *model.CompileGenAIAnalysis) {
	genaiAnalysis.Status = pb.AnalysisStatus_ERROR
	genaiAnalysis.RunStatus = pb.AnalysisRunStatus_ENDED
	genaiAnalysis.EndTime = clock.Now(c)
	datastore.Put(c, genaiAnalysis)
}
