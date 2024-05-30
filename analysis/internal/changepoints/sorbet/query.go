// Copyright 2024 The LUCI Authors.
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

package sorbet

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/gitiles"
	"go.chromium.org/luci/analysis/internal/testverdicts"
)

const (
	// The maximum number of CL suspects to analyse.
	MaxSuspectCLs = 50
)

// GenerateClient represents the interface used to generate analysis from a prompt.
type GenerateClient interface {
	Generate(ctx context.Context, prompt string) (GenerateResponse, error)
}

// TestVerdictClient provides access to test verdicts.
type TestVerdictClient interface {
	ReadTestVerdictAfterPosition(ctx context.Context, options testverdicts.ReadVerdictAtOrAfterPositionOptions) (*testverdicts.SourceVerdict, error)
}

// Analyzer provides analysis on the possible culprits of a changepoint.
type Analyzer struct {
	generateClient    GenerateClient
	testVerdictClient TestVerdictClient
}

// NewAnalyzer initialises a new Analyzer.
func NewAnalyzer(gc GenerateClient, tvc TestVerdictClient) *Analyzer {
	return &Analyzer{
		generateClient:    gc,
		testVerdictClient: tvc,
	}
}

type AnalysisRequest struct {
	// The LUCI Project.
	Project string
	// The identifier of the test.
	TestID              string
	VariantHash         string
	RefHash             testvariantbranch.RefHash
	StartSourcePosition int64
	// The realms the user is allowed to query test results for in the project.
	AllowedRealms []string
}

type AnalysisResponse struct {
	// The response.
	Response string
	// The prompt used to generate the response.
	Prompt string
}

// Analyze generates an analysis of the possible culprits for a given changepoint.
// It returns appstatus-annotated errors for client errors.
func (a *Analyzer) Analyze(ctx context.Context, request AnalysisRequest) (AnalysisResponse, error) {
	cp, err := fetchChangepoint(ctx, request)
	if err != nil {
		return AnalysisResponse{}, errors.Annotate(err, "fetch changepoint").Err()
	}
	logging.Debugf(ctx, "The changepoint matched to the request has a nominal range of [%d, %d] and 99th percentile range of [%d, %d].",
		cp.NominalEndPreviousSegment, cp.NominalStartNextSegment, cp.StartLowerBound99th, cp.StartUpperBound99th)

	bounds := identifySearchBounds(cp, MaxSuspectCLs)
	logging.Debugf(ctx, "The search bounds were identified to be commit positions [%d, %d] (%d positions).", bounds.startPosition, bounds.endPosition, bounds.endPosition-bounds.startPosition+1)

	verdict, err := a.querySourceVerdictAtOrAfterEnd(ctx, request, bounds)
	if err != nil {
		return AnalysisResponse{}, errors.Annotate(err, "query verdict at or after end of searched range").Err()
	}
	logging.Debugf(ctx, "Retrieved source verdict at position %d.", verdict.Position)

	cls, err := queryChangelists(ctx, request, verdict, bounds)
	if err != nil {
		return AnalysisResponse{}, errors.Annotate(err, "query changelists").Err()
	}
	logging.Debugf(ctx, "Retrieved %d changelists.", len(cls))

	testCode, err := fetchCode(ctx, verdict)
	if err != nil {
		return AnalysisResponse{}, errors.Annotate(err, "query test code").Err()
	}
	logging.Debugf(ctx, "Test code retrieval complete.")

	prompt := preparePrompt(request, verdict, cls, testCode)

	logging.Debugf(ctx, "Generating AI response...")
	result, err := a.generateClient.Generate(ctx, prompt)
	if err != nil {
		return AnalysisResponse{}, errors.Annotate(err, "generate").Err()
	}
	response := AnalysisResponse{
		Response: result.Candidate,
		Prompt:   prompt,
	}
	if response.Response == "" {
		response.Response = fmt.Sprintf("The response was blocked: %s", result.BlockReason)
	}
	return response, nil
}

// changepoint specifies the location of a changepoint.
type changepoint struct {
	// The lower bound of the 99% confidence interval of the start source position.
	StartLowerBound99th int64
	// The upper bound of the 99% confidence interval of the start source position.
	StartUpperBound99th int64
	// The nominal start of the new (failing) segment. In case of deterministic
	// pass -> fail transitions, this is the source position of the first known failure.
	NominalStartNextSegment int64
	// The nominal end of the old (passing) segment. In case of deterministic
	// pass -> fail transitions, this is the source position of the last known pass.
	NominalEndPreviousSegment int64
	// The approximate wall-clock time the new failing segment started.
	ApproxStartHour time.Time
}

// fetchChangepoint fetches the changepoint closest to the given request.
func fetchChangepoint(ctx context.Context, request AnalysisRequest) (changepoint, error) {
	key := testvariantbranch.Key{
		Project:     request.Project,
		TestID:      request.TestID,
		VariantHash: request.VariantHash,
		RefHash:     request.RefHash,
	}
	rsp, err := testvariantbranch.Read(span.Single(ctx), []testvariantbranch.Key{key})
	if err != nil {
		return changepoint{}, errors.Annotate(err, "read").Err()
	}
	// Response will always have same length as request, but if not found the element
	// may be nil.
	tvb := rsp[0]
	if tvb == nil {
		return changepoint{}, appstatus.Errorf(codes.NotFound, "test variant branch not found")
	}

	var analyzer changepoints.Analyzer
	inputSegments := analyzer.Run(tvb)

	// TODO(meiring): It is a bit clumsy to use the BigQuery export segments here but
	// until another pending refactoring CL lands this appears to be the best option.
	segments := bqexporter.ToSegments(tvb, inputSegments)

	// Find the segment with the start position closest to request.StartSourcePosition.
	closestSegmentIndex := 0
	closestDistance := int64(math.MaxInt64)
	for i, segment := range segments {
		// Take the absolute value of the distance.
		distance := segment.StartPosition - request.StartSourcePosition
		if distance < 0 {
			distance = -distance
		}

		if distance < closestDistance {
			closestSegmentIndex = i
			closestDistance = distance
		}
	}
	if closestSegmentIndex >= len(segments)-1 {
		// The segment with the closest start position is the oldest segment.
		// This is because there is no changepoint to an earlier segment or
		// because it is beyond our retention period.
		return changepoint{}, appstatus.Errorf(codes.NotFound, "test variant branch changepoint not found")
	}
	nextSegment := segments[closestSegmentIndex]
	previousSegment := segments[closestSegmentIndex+1]

	result := changepoint{
		StartLowerBound99th:       nextSegment.StartPositionLowerBound_99Th,
		StartUpperBound99th:       nextSegment.StartPositionUpperBound_99Th,
		NominalStartNextSegment:   nextSegment.StartPosition,
		NominalEndPreviousSegment: previousSegment.EndPosition,
		ApproxStartHour:           nextSegment.StartHour.AsTime(),
	}

	return result, nil
}

type searchBounds struct {
	// The source position of the first suspected culprit CL.
	startPosition int64
	// The source position of the last suspected culprit CL.
	// endPosition > startPosition.
	endPosition int64
	// The minimum partition time to search.
	startTime time.Time
	// The maximum partition time to search.
	endTime time.Time
}

// identifySearchBounds recommends a range of commit positions to search
// for the culprit, assuming the search is bounded to at most maxSuspects CLs.
// The range returned is inclusive, where end >= start.
func identifySearchBounds(c changepoint, maxSuspects int) searchBounds {
	startPos := c.StartLowerBound99th
	endPos := c.StartUpperBound99th

	// Search +/- 7 days around the approximate time of the changepoint.
	// If the changepoint is so unclear that more than 7 days of CLs
	// are suspects, then good luck finding the culprit... in chromium
	// repo there would be thousands of commits to sift through.
	startTime := c.ApproxStartHour.Add(-7 * 24 * time.Hour)
	endTime := c.ApproxStartHour.Add(7 * 24 * time.Hour)

	if int(endPos-startPos+1) <= maxSuspects {
		// 99% confidence range is satisfactory.
		return searchBounds{
			startPosition: startPos,
			endPosition:   endPos,
			startTime:     startTime,
			endTime:       endTime,
		}
	}

	// There are too many suspects.
	// Focus on the nominal range only (accurate for deterministic regressions only).
	startPos = c.NominalEndPreviousSegment + 1
	endPos = c.NominalStartNextSegment

	if int(endPos-startPos+1) > maxSuspects {
		// Still too large. Look at the 50 prior to the start of the
		// new behaviour.
		startPos = endPos - 50
		if startPos < 1 {
			startPos = 1
		}
	}

	return searchBounds{
		startPosition: startPos,
		endPosition:   endPos,
		startTime:     startTime,
		endTime:       endTime,
	}
}

type changelist struct {
	Position int64
	Commit   *git.Commit
}

// querySourceVerdictAtOrAfterEnd queries details about the first source verdict
// at or after the end of the search bounds. By virtue of its position, this
// verdict should be within the new segment and therefore exhibit
// the failure reason(s) of that segment.
//
// The verdict can also be useful in querying logs from gitiles.
func (a *Analyzer) querySourceVerdictAtOrAfterEnd(ctx context.Context, req AnalysisRequest, bounds searchBounds) (*testverdicts.SourceVerdict, error) {
	if bounds.startPosition > bounds.endPosition {
		panic("bounds end position must be after the start position")
	}

	// Gitiles log requires us to supply a commit hash to fetch a commit log.
	// Ideally, if we know the commit hash of the requested endPosition, we can just
	// call gitiles with that, and page (backwards) through the commit history.
	//
	// However, as test verdicts are sparse, we may not have the commit hash for
	// the given endPosition. Therefore, we need to find the commit which
	// is the closest to or after the given endPosition.
	options := testverdicts.ReadVerdictAtOrAfterPositionOptions{
		Project:            req.Project,
		TestID:             req.TestID,
		VariantHash:        req.VariantHash,
		RefHash:            hex.EncodeToString([]byte(req.RefHash)),
		AtOrAfterPosition:  bounds.endPosition,
		PartitionTimeStart: bounds.startTime,
		PartitionTimeEnd:   bounds.endTime,
		AllowedRealms:      req.AllowedRealms,
	}
	closestAfterCommit, err := a.testVerdictClient.ReadTestVerdictAfterPosition(ctx, options)
	if err != nil {
		return nil, errors.Annotate(err, "read test verdict at or after position from BigQuery").Err()
	}
	if closestAfterCommit == nil {
		return nil, appstatus.Errorf(codes.NotFound, "no commit could be located at or after position %v in partition time range %s to %s", bounds.endPosition, bounds.startTime, bounds.endTime)
	}
	return closestAfterCommit, nil
}

func queryChangelists(ctx context.Context, req AnalysisRequest, closestAfterVerdict *testverdicts.SourceVerdict, bounds searchBounds) ([]changelist, error) {
	// Work out the number of commits to fetch from gitiles.
	requiredNumCommits := closestAfterVerdict.Position - bounds.startPosition + 1
	if requiredNumCommits > 10000 {
		// One gitiles log call returns at most 10000 commits.
		// It is unlikely that (pagesize + offset) will be greater than 10000, we just throw NotFound here if that happens.
		return nil, appstatus.Errorf(codes.NotFound, "cannot find relevant commits for position %v because nearest known commit hash is at position %v (>10000 positions away)", bounds.startPosition, closestAfterVerdict.Position)
	}
	ref := closestAfterVerdict.Ref
	// N.B. gitiles host is untrusted user input, but gitiles.NewClient validates we
	// are not connecting to an arbitrary host on the internet.
	// TODO: Supports open-source gitiles only. Investigate auth for general case.
	gitilesClient, err := gitiles.NewClient(ctx, ref.Gitiles.Host.String(), auth.NoAuth)
	if err != nil {
		return nil, errors.Annotate(err, "create gitiles client").Err()
	}
	logReq := &gitilespb.LogRequest{
		Project:    ref.Gitiles.Project.String(),
		Committish: closestAfterVerdict.CommitHash,
		PageSize:   int32(requiredNumCommits),
		TreeDiff:   true,
	}
	logRes, err := gitilesClient.Log(ctx, logReq)
	if err != nil {
		return nil, errors.Annotate(err, "gitiles log").Err()
	}
	// The response from gitiles contains commits from closestAfterVerdict.
	// Commit at the requested end position is at offset.
	offset := closestAfterVerdict.Position - bounds.endPosition
	logs := logRes.Log[offset:]

	var results []changelist
	for i, commit := range logs {
		results = append(results, changelist{
			Position: bounds.endPosition - int64(i),
			Commit:   commit,
		})
	}
	return results, nil
}

func preparePrompt(req AnalysisRequest, verdict *testverdicts.SourceVerdict, cls []changelist, testCode string) string {
	// TODO: Fetch more data relevant to the analysis and include it in the prompt.
	var prompt strings.Builder
	prompt.WriteString("You are a software engineer tasked with identifying the code change (commit) that caused a test to start failing.\n")
	prompt.WriteString("\n")
	prompt.WriteString(fmt.Sprintf("The software project is %q.\n", req.Project))
	prompt.WriteString(fmt.Sprintf("The failing test is %q.\n", req.TestID))
	prompt.WriteString(fmt.Sprintf("The failing test variant (test configuration) is %s.\n", verdict.Variant))
	if verdict.Results[0].PrimaryFailureReason.StringVal != "" {
		prompt.WriteString("The test is failing with the reason:\n")
		prompt.WriteString("```\n")
		prompt.WriteString(fmt.Sprintf("%s\n", verdict.Results[0].PrimaryFailureReason.StringVal))
		prompt.WriteString("```\n")
	}
	prompt.WriteString("\n")
	if verdict.TestLocation != nil && verdict.TestLocation.FileName != "" {
		prompt.WriteString(fmt.Sprintf("The location of the failing test in the source repository is %s.\n", verdict.TestLocation.FileName))
		if testCode != "" {
			prompt.WriteString("The content of that file is:\n")
			prompt.WriteString("```START OF FILE\n")
			prompt.WriteString(testCode)
			prompt.WriteString("```END OF FILE\n")
		}
	}
	prompt.WriteString("\n")
	prompt.WriteString("The culprit for the test failure is one of the following commits:\n")
	prompt.WriteString("\n")
	for _, cl := range cls {
		prompt.WriteString(fmt.Sprintf("Commit Position %d\n", cl.Position))
		prompt.WriteString("```\n")
		prompt.WriteString(cl.Commit.Message)
		prompt.WriteString("```\n")
		prompt.WriteString("Touched files:\n")
		for i, file := range cl.Commit.TreeDiff {
			if i > 100 {
				prompt.WriteString(fmt.Sprintf("(%v other files changed, details omitted for brevity)", len(cl.Commit.TreeDiff)-100))
				// Limit how many files we include per changelist
				break
			}
			prompt.WriteString(treeDiffDescription(file))
		}
		prompt.WriteString("")
		prompt.WriteString("\n")
		prompt.WriteString("\n")
	}
	prompt.WriteString("\n")
	prompt.WriteString("Go through each commit in order, and write out:\n")
	prompt.WriteString("- a description of the change\n")
	prompt.WriteString("- factors that may rule the change in and/or out from causing the test failure\n")
	prompt.WriteString("- your assessment of whether the change made the test failure appear\n")
	prompt.WriteString("\n")
	prompt.WriteString("Then, under a section heading for conclusions, identify the most likely commit(s).\n")
	prompt.WriteString("In your answer, please use markdown format and " +
		"link to commit position using the format [Commit Position 987654](https://crrev.com/987654).\n")
	prompt.WriteString("\n")
	prompt.WriteString("For example:\n")
	prompt.WriteString("## Commit Analysis\n")
	prompt.WriteString("**[Commit Position 987654](https://crrev.com/987654)**\n")
	prompt.WriteString("\n")
	prompt.WriteString("* **Description:** <description of change>\n")
	prompt.WriteString("* **Relevant parts:** <discussion of factors that may rule the change in or out from causing the test failure>\n")
	prompt.WriteString("* **Assessment:** <assessment>\n")
	prompt.WriteString("\n")
	prompt.WriteString("<more commits...>")
	prompt.WriteString("\n")
	prompt.WriteString("## Conclusions\n")
	prompt.WriteString("\n")
	prompt.WriteString("The following commit(s) have been flagged as potentially related to the test failure:\n")
	prompt.WriteString("\n")
	prompt.WriteString("**High Priority:**\n")
	prompt.WriteString("\n")
	prompt.WriteString("* **[Commit Position 123456](https://crrev.com/123456):** <reason>\n")
	prompt.WriteString("\n")
	prompt.WriteString("**Medium Priority:** (this section is optional and may be omitted if there are no medium priority commits)\n")
	prompt.WriteString("\n")
	prompt.WriteString("* **[Commit Position 234567](https://crrev.com/123456):** <reason>\n")
	return prompt.String()
}

func treeDiffDescription(diff *git.Commit_TreeDiff) string {
	switch diff.Type {
	case git.Commit_TreeDiff_ADD, git.Commit_TreeDiff_COPY:
		return fmt.Sprintf("+ %s (added)\n", diff.NewPath)
	case git.Commit_TreeDiff_MODIFY, git.Commit_TreeDiff_RENAME:
		return fmt.Sprintf("* %s (modified)\n", diff.NewPath)
	case git.Commit_TreeDiff_DELETE:
		return fmt.Sprintf("- %s (deleted)\n", diff.OldPath)
	default:
		panic(fmt.Sprintf("unexpected diff type: %s", diff.Type.String()))
	}
}

func fetchCode(ctx context.Context, verdict *testverdicts.SourceVerdict) (string, error) {
	ref := verdict.Ref
	gitilesClient, err := gitiles.NewClient(ctx, ref.Gitiles.Host.String(), auth.NoAuth)
	if err != nil {
		return "", errors.Annotate(err, "create gitiles client").Err()
	}
	path := strings.TrimLeft(verdict.TestLocation.FileName, "/")

	fileReq := &gitilespb.DownloadFileRequest{
		Project:    ref.Gitiles.Project.String(),
		Committish: verdict.CommitHash,
		Path:       path,
	}
	rsp, err := gitilesClient.DownloadFile(ctx, fileReq)
	if err != nil {
		// Downloading code is best effort only, it should not block analysis.
		logging.Debugf(ctx, "Unable to fetch file from gerrit: %s", path)
		return "", nil
	}
	return formatCodeFile(rsp.Contents), nil
}

func formatCodeFile(content string) string {
	lines := strings.Split(content, "\n")
	var result strings.Builder
	for i, line := range lines {
		result.WriteString(fmt.Sprintf("line %v: %s\n", i+1, line))
	}
	return result.String()
}
