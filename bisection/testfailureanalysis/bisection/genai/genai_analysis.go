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

package genai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"google.golang.org/genai"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/culpritverification/task"
	"go.chromium.org/luci/bisection/internal/resultdb"
	"go.chromium.org/luci/bisection/llm"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/testfailureanalysis"
	"go.chromium.org/luci/bisection/util/changelogutil"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

const (
	promptTemplate = `You are an experienced software engineer analyzing test failures, below are the details

Test failure information:
Test ID: %s
Test failure summary: %s

Blamelist: %s

Instructions:
Analyze the test failure and identify the TOP 3 most likely culprit commits from the blamelist that caused this test to start failing.
For each suspect, provide the commit ID, a confidence score (from 0 to 10, where 10 is most confident), and a short justification (less than 512 characters).
If there are fewer than 3 commits in the blamelist, just provide all of them ranked by confidence.
`
)

// Response schema for structured output
var responseSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"suspects": {
			Type: genai.TypeArray,
			Items: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"commit_id": {
						Type:        genai.TypeString,
						Description: "The commit ID/hash",
					},
					"confidence_score": {
						Type:        genai.TypeInteger,
						Description: "Confidence score from 0 to 10",
					},
					"justification": {
						Type:        genai.TypeString,
						Description: "Justification for why this commit is suspected (max 512 characters)",
					},
				},
				Required: []string{"commit_id", "confidence_score", "justification"},
			},
		},
	},
	Required: []string{"suspects"},
}

// StructuredResponse represents the JSON response from GenAI with structured output
type StructuredResponse struct {
	Suspects []SuspectInfo `json:"suspects"`
}

func Analyze(ctx context.Context, tfa *model.TestFailureAnalysis, client llm.Client, rdbClient resultdb.Client) (reterr error) {
	logging.Infof(ctx, "Starting GenAI analysis for test failure %d", tfa.ID)

	// Create the GenAI analysis entity
	genaiAnalysis := &model.TestGenAIAnalysis{
		ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		StartTime:         clock.Now(ctx),
		Status:            pb.AnalysisStatus_RUNNING,
		RunStatus:         pb.AnalysisRunStatus_STARTED,
	}

	if err := datastore.Put(ctx, genaiAnalysis); err != nil {
		return errors.Fmt("failed to save GenAI analysis to datastore: %w", err)
	}

	defer func() {
		if reterr != nil {
			if err := testfailureanalysis.UpdateGenAIAnalysisStatus(ctx, genaiAnalysis, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED); err != nil {
				logging.Errorf(ctx, "Failed to update GenAI analysis status to ERROR: %v", err)
			}
		}
	}()

	// Get primary test failure
	primaryFailure, err := datastoreutil.GetPrimaryTestFailure(ctx, tfa)
	if err != nil {
		return errors.Fmt("failed to get primary test failure: %w", err)
	}

	// Construct failure summary with artifact content
	failureSummary := constructFailureSummary(ctx, rdbClient, primaryFailure)

	// Get Blamelist - construct regression range from primary failure
	gitiles := primaryFailure.Ref.GetGitiles()
	if gitiles == nil {
		return errors.New("no gitiles info in primary failure")
	}

	regressionRange := &pb.RegressionRange{
		LastPassed: &buildbucketpb.GitilesCommit{
			Host:    gitiles.Host,
			Project: gitiles.Project,
			Ref:     gitiles.Ref,
			Id:      tfa.StartCommitHash,
		},
		FirstFailed: &buildbucketpb.GitilesCommit{
			Host:    gitiles.Host,
			Project: gitiles.Project,
			Ref:     gitiles.Ref,
			Id:      tfa.EndCommitHash,
		},
	}

	// Get changelogs for the regression range
	changelogs, err := changelogutil.GetChangeLogs(ctx, regressionRange, false)
	if err != nil {
		return errors.Fmt("failed to get changelogs: %w", err)
	}

	if len(changelogs) == 0 {
		return errors.New("no changelogs found in regression range")
	}

	// Prepare blamelist from changelogs
	blamelist, err := prepareBlamelist(changelogs)
	if err != nil {
		return errors.Fmt("failed to prepare blamelist: %w", err)
	}

	// Construct the prompt
	prompt := constructPrompt(primaryFailure.TestID, failureSummary, blamelist)

	// Call GenAI API with structured output
	rawResponse, err := client.GenerateContentWithSchema(ctx, prompt, responseSchema)
	if err != nil {
		return errors.Fmt("failed to call GenAI: %w", err)
	}

	// Parse the structured JSON response
	var response StructuredResponse
	if err := json.Unmarshal([]byte(rawResponse), &response); err != nil {
		return errors.Fmt("failed to parse GenAI JSON response: %w", err)
	}

	suspects := response.Suspects

	// Validate results, at least one suspect, at most 3
	if len(suspects) == 0 {
		return errors.New("no suspects found in GenAI response")
	}
	if len(suspects) > 3 {
		logging.Warningf(ctx, "GenAI returned %d suspects, truncating to 3", len(suspects))
		suspects = suspects[:3]
	}

	logging.Infof(ctx, "GenAI analysis identified %d suspects", len(suspects))

	// Save the suspect entities and update GenAI analysis
	suspectsModel, err := saveSuspectsAndUpdateGenAIAnalysis(ctx, genaiAnalysis, suspects, changelogs, gitiles)
	if err != nil {
		return errors.Fmt("failed to save suspects: %w", err)
	}

	// Schedule Test Failure verification for all GenAI suspects
	for _, suspect := range suspectsModel {
		if err := task.ScheduleTestFailureTask(ctx, tfa.ID, suspect.Id, datastore.KeyForObj(ctx, tfa).Encode()); err != nil {
			// Non-critical, just log the error
			logging.Errorf(ctx, "Failed to schedule culprit verification task for suspect %d: %v", suspect.Id, err)
		}
	}

	return nil
}

// constructPrompt constructs the prompt for GenAI analysis from the test ID, failure summary, and blamelist.
func constructPrompt(testID, failureSummary, blamelist string) string {
	return fmt.Sprintf(promptTemplate, testID, failureSummary, blamelist)
}

// prepareBlamelist formats changeLogs into a readable blamelist for the LLM
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

// SuspectInfo holds the parsed information for a single suspect from GenAI response
type SuspectInfo struct {
	CommitID      string `json:"commit_id"`
	Score         int    `json:"confidence_score"`
	Justification string `json:"justification"`
}

// saveSuspectsAndUpdateGenAIAnalysis creates and saves all suspect entities, then updates the GenAI analysis status.
// Uses a transaction to ensure atomicity - either all suspects are saved and analysis is updated, or nothing is saved.
// Returns the list of created suspects for verification scheduling.
func saveSuspectsAndUpdateGenAIAnalysis(ctx context.Context, genaiAnalysis *model.TestGenAIAnalysis, suspects []SuspectInfo, changelogs []*model.ChangeLog, gitiles *pb.GitilesRef) ([]*model.Suspect, error) {
	var suspectsModel []*model.Suspect

	// Build all suspect entities first (no I/O)
	for _, suspectInfo := range suspects {
		// Find the changelog for this suspect to extract review info and commit time
		var reviewUrl, reviewTitle string
		var commitTime time.Time
		for _, cl := range changelogs {
			if cl.Commit == suspectInfo.CommitID {
				if url, err := cl.GetReviewUrl(); err == nil {
					reviewUrl = url
				}
				if title, err := cl.GetReviewTitle(); err == nil {
					reviewTitle = title
				}
				if ct, err := cl.GetCommitTime(); err == nil {
					commitTime = ct.AsTime()
				}
				break
			}
		}

		suspect := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(ctx, genaiAnalysis),
			Type:           model.SuspectType_GenAI,
			GitilesCommit: buildbucketpb.GitilesCommit{
				Host:    gitiles.Host,
				Project: gitiles.Project,
				Ref:     gitiles.Ref,
				Id:      suspectInfo.CommitID,
			},
			ReviewUrl:          reviewUrl,
			ReviewTitle:        reviewTitle,
			Score:              suspectInfo.Score,
			Justification:      suspectInfo.Justification,
			VerificationStatus: model.SuspectVerificationStatus_Unverified,
			AnalysisType:       pb.AnalysisType_TEST_FAILURE_ANALYSIS,
			CommitTime:         commitTime,
		}

		suspectsModel = append(suspectsModel, suspect)
	}

	// Save all suspects and analysis in a single transaction for atomicity
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Read the latest version from datastore to avoid race conditions
		if err := datastore.Get(ctx, genaiAnalysis); err != nil {
			return errors.Fmt("failed to get GenAI analysis: %w", err)
		}

		// Update analysis status
		genaiAnalysis.Status = pb.AnalysisStatus_SUSPECTFOUND
		genaiAnalysis.RunStatus = pb.AnalysisRunStatus_ENDED
		genaiAnalysis.EndTime = clock.Now(ctx)

		// Batch put all suspects
		if err := datastore.Put(ctx, suspectsModel); err != nil {
			return errors.Fmt("failed to save suspects: %w", err)
		}

		// Update the analysis entity
		if err := datastore.Put(ctx, genaiAnalysis); err != nil {
			return errors.Fmt("failed to update GenAI analysis: %w", err)
		}

		return nil
	}, nil)

	if err != nil {
		return nil, err
	}

	// Log after successful save
	for _, suspect := range suspectsModel {
		logging.Infof(ctx, "Created suspect: %s with score %d", suspect.GitilesCommit.Id, suspect.Score)
	}

	return suspectsModel, nil
}

// parseArtifactReferences parses <text-artifact artifact-id="..."/> tags from HTML summary.
func parseArtifactReferences(summaryHTML string) []string {
	// Regex to match <text-artifact artifact-id="artifact_name" [inv-level]>
	re := regexp.MustCompile(`<text-artifact\s+artifact-id="([^"]+)"`)
	matches := re.FindAllStringSubmatch(summaryHTML, -1)

	var artifactIDs []string
	for _, match := range matches {
		if len(match) > 1 {
			artifactIDs = append(artifactIDs, match[1])
		}
	}

	return artifactIDs
}

// constructFailureSummary constructs a detailed failure summary by combining
// direct fields and artifact content.
func constructFailureSummary(
	ctx context.Context,
	rdbClient resultdb.Client,
	primaryFailure *model.TestFailure,
) string {
	var summaryBuilder strings.Builder

	// Start with direct fields (always available)
	summaryBuilder.WriteString(fmt.Sprintf("Failure Kind: %s\n\n", primaryFailure.FailureKind))

	if primaryFailure.PrimaryErrorMessage != "" {
		summaryBuilder.WriteString("Primary Error Message:\n")
		summaryBuilder.WriteString(primaryFailure.PrimaryErrorMessage)
		summaryBuilder.WriteString("\n\n")
	}

	if primaryFailure.FirstErrorTrace != "" {
		summaryBuilder.WriteString("Stack Trace:\n")
		summaryBuilder.WriteString(primaryFailure.FirstErrorTrace)
		summaryBuilder.WriteString("\n\n")
	}

	// If we have invocation and result IDs, try to fetch full artifact content
	if rdbClient != nil && primaryFailure.InvocationID != "" && primaryFailure.ResultID != "" && primaryFailure.SummaryHTML != "" {
		// Parse artifact references from summary HTML
		artifactIDs := parseArtifactReferences(primaryFailure.SummaryHTML)

		if len(artifactIDs) > 0 {
			logging.Infof(ctx, "Found %d artifact references in summary_html", len(artifactIDs))

			// Fetch specific artifact contents using GetArtifact
			artifactContents, err := rdbClient.FetchSpecificArtifacts(ctx, primaryFailure.InvocationID, primaryFailure.TestID, primaryFailure.ResultID, artifactIDs)
			if err != nil {
				logging.Warningf(ctx, "Failed to fetch artifacts (using fallback): %v", err)
			} else if len(artifactContents) > 0 {
				// Append artifact contents
				summaryBuilder.WriteString("Full Artifact Content:\n")
				summaryBuilder.WriteString("========================\n\n")

				for _, artifactID := range artifactIDs {
					if content, ok := artifactContents[artifactID]; ok {
						summaryBuilder.WriteString(fmt.Sprintf("Artifact: %s\n", artifactID))
						summaryBuilder.WriteString("---\n")
						summaryBuilder.WriteString(content)
						summaryBuilder.WriteString("\n\n")
					}
				}
			}
		}
	}

	return summaryBuilder.String()
}
