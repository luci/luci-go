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

	"google.golang.org/genai"

	"go.chromium.org/luci/common/errors"
)

type Client interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}

type clientImpl struct {
	client *genai.Client
}

func NewClient(ctx context.Context, cloudProject string) (Client, error) {
	location := "us-central1" // Default location for Vertex AI
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:  cloudProject,
		Location: location,
		Backend:  genai.BackendVertexAI,
	})
	if err != nil {
		return nil, errors.Fmt("failed to create genai client: %w", err)
	}

	return &clientImpl{
		client: client,
	}, nil
}

func (c *clientImpl) GenerateContent(ctx context.Context, prompt string) (string, error) {
	if c == nil {
		return "", errors.New("GenAI client is nil")
	}

	contents := []*genai.Content{
		genai.NewContentFromText(prompt, genai.RoleUser),
	}

	// Generate content
	response, err := c.client.Models.GenerateContent(ctx, "gemini-2.5-pro", contents, nil)
	if err != nil {
		return "", errors.Fmt("failed to generate content: %w", err)
	}

	return response.Text(), nil
}
