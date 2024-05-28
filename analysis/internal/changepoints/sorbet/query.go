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

	"go.chromium.org/luci/common/errors"
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
	TestID string
}

type AnalysisResponse struct {
	// The response.
	Response string
	// The prompt used to generate the response.
	Prompt string
}

// Analyze generates an analysis of the possible culprits for a given changepoint.
func (a *Analyzer) Analyze(ctx context.Context, request AnalysisRequest) (AnalysisResponse, error) {
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
