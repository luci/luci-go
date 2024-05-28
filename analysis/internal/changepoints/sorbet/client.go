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

// Package sorbet implements analysis for test changepoint culprits.
package sorbet

import (
	"context"
	"strings"

	gai "cloud.google.com/go/vertexai/genai"

	"go.chromium.org/luci/common/errors"
)

type Client struct {
	client *gai.Client
}

func NewClient(ctx context.Context, gcpProject string) (*Client, error) {
	location := "us-central1" // Fall back to inferred location or default (us-central1).
	client, err := gai.NewClient(ctx, gcpProject, location)
	if err != nil {
		return nil, errors.Annotate(err, "create sorbet client").Err()
	}
	return &Client{client: client}, nil
}

type GenerateResponse struct {
	// The generated response.
	Candidate string
	// If the response is blocked, the reason why. Set if Candidate is empty.
	BlockReason string
}

// Generate generates a response for the given prompt.
func (c *Client) Generate(ctx context.Context, prompt string) (GenerateResponse, error) {
	m := c.client.GenerativeModel("gemini-1.5-pro")
	m.SetCandidateCount(1)

	response, err := m.GenerateContent(ctx, gai.Text(prompt))
	if err != nil {
		return GenerateResponse{}, errors.Annotate(err, "generate content").Err()
	}

	if len(response.Candidates) == 0 {
		// No candidates could be generated for content reasons.
		return GenerateResponse{BlockReason: response.PromptFeedback.BlockReasonMessage}, nil
	}

	// Concatenate the result parts.
	var result strings.Builder
	for _, part := range response.Candidates[0].Content.Parts {
		text, ok := part.(gai.Text)
		if !ok {
			continue
		}
		if result.Len() > 0 {
			result.WriteRune('\n')
		}
		result.WriteString(string(text))
	}
	return GenerateResponse{Candidate: result.String()}, nil
}
