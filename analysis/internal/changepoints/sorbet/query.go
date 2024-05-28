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
	"fmt"
	"math"

	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"
	"google.golang.org/grpc/codes"
)

const (
	// The maximum number of CL suspects to analyse.
	MaxSuspectCLs = 50
)

// GenerateClient represents the interface used to generate analysis from a prompt.
type GenerateClient interface {
	Generate(ctx context.Context, prompt string) (GenerateResponse, error)
}

// Analyzer provides analysis on the possible culprits of a changepoint.
type Analyzer struct {
	client GenerateClient
}

// NewAnalyzer initialises a new Analyzer.
func NewAnalyzer(client GenerateClient) *Analyzer {
	return &Analyzer{
		client: client,
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
		return AnalysisResponse{}, err
	}
	logging.Debugf(ctx, "The changepoint matched to the request has a nominal range of [%d, %d] and 99th percentile range of [%d, %d].",
		cp.NominalEndPreviousSegment, cp.NominalStartNextSegment, cp.StartLowerBound99th, cp.StartUpperBound99th)

	start, end := identifySearchBounds(cp, MaxSuspectCLs)
	logging.Debugf(ctx, "The search bounds were identified to be commit positions [%d, %d] (%d positions).", start, end, end-start+1)

	// TODO: Fetch more data relevant to the analysis and include it in the prompt.
	prompt := fmt.Sprintf("Your job is to help the software engineer try to identify which code change caused a test to start failing. "+
		"The software project is %s.\n\n"+
		"The failing test is %q.\n\n", request.Project, request.TestID)

	result, err := a.client.Generate(ctx, prompt)
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

// Changepoint
type changepoint struct {
	StartLowerBound99th       int64
	StartUpperBound99th       int64
	NominalStartNextSegment   int64
	NominalEndPreviousSegment int64
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
	}

	return result, nil
}

// identifySearchBounds recommends a range of commit positions to search
// for the culprit, assuming the search is bounded to at most maxSuspects CLs.
// The range returned is inclusive.
func identifySearchBounds(c changepoint, maxSuspects int) (start, end int64) {
	start = c.StartLowerBound99th
	end = c.StartUpperBound99th

	if int(end-start+1) <= maxSuspects {
		// 99% confidence range is satisfactory.
		return start, end
	}

	// There are too many suspects.
	// Focus on the nominal range only (accurate for deterministic regressions only).
	start = c.NominalEndPreviousSegment + 1
	end = c.NominalStartNextSegment

	if int(end-start+1) > maxSuspects {
		// Still too large. Look at the 50 prior to the start of the
		// new behaviour.
		start = end - 50
		if start < 1 {
			start = 1
		}
	}

	// If there are available slots for suspects, expand to fill the number of
	// available suspects, remaining constrained by the 99% confidence
	// interval (N.B. that range is often assymetric)
	var alternate bool
	for int(end-start+1) <= maxSuspects {
		canAddToStart := start > c.StartLowerBound99th
		canAddToEnd := end < c.StartUpperBound99th
		if canAddToStart && canAddToEnd {
			if alternate {
				start--
			} else {
				end++
			}
			alternate = !alternate
		} else if canAddToStart {
			start--
		} else if canAddToEnd {
			end++
		} else {
			// We only entered this path because the 99th range was
			// too large in and of itself.
			panic("should be unreachable")
		}
	}
	return start, end
}
