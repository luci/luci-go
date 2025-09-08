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

package server

import (
	"context"
	"errors"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultai/proto/v1"
)

// mockLLMClient implements llm.Client for testing
type mockLLMClient struct {
	responseText string
	err          error
}

func (m *mockLLMClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.responseText, nil
}

func TestExecutePrompt(t *testing.T) {
	t.Parallel()
	ftt.Run("ExecutePrompt", t, func(t *ftt.Test) {
		t.Run("Success", func(t *ftt.Test) {
			mockClient := &mockLLMClient{
				responseText: "AI generated response",
			}
			server := &PromptServer{LlmClient: mockClient}

			req := &pb.ExecutePromptRequest{
				Name: "promptTemplates/compilefailureanalysis",
				PromptParams: map[string]string{
					"Failure":   "compilation error in main.cpp",
					"Blamelist": "commit abc123, commit def456",
				},
			}

			resp, err := server.ExecutePrompt(context.Background(), req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.PromptOutput, should.Equal("AI generated response"))
		})

		t.Run("Invalid prompt name format", func(t *ftt.Test) {
			mockClient := &mockLLMClient{}
			server := &PromptServer{LlmClient: mockClient}

			req := &pb.ExecutePromptRequest{
				Name: "invalid/format/name",
			}

			resp, err := server.ExecutePrompt(context.Background(), req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, resp, should.BeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("invalid prompt template name format"))
		})

		t.Run("Template file not found", func(t *ftt.Test) {
			mockClient := &mockLLMClient{}
			server := &PromptServer{LlmClient: mockClient}

			req := &pb.ExecutePromptRequest{
				Name: "promptTemplates/nonexistent",
			}

			resp, err := server.ExecutePrompt(context.Background(), req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, resp, should.BeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("failed to load prompt template"))
		})

		t.Run("LLM client error", func(t *ftt.Test) {
			mockClient := &mockLLMClient{
				err: errors.New("mock LLM client error"),
			}
			server := &PromptServer{LlmClient: mockClient}

			req := &pb.ExecutePromptRequest{
				Name: "promptTemplates/compilefailureanalysis",
				PromptParams: map[string]string{
					"Failure":   "compilation error",
					"Blamelist": "commit abc123",
				},
			}

			resp, err := server.ExecutePrompt(context.Background(), req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, resp, should.BeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("failed to generate content"))
		})
	})
}

func TestProcessTemplate(t *testing.T) {
	t.Parallel()
	ftt.Run("processTemplate", t, func(t *ftt.Test) {
		server := &PromptServer{}

		t.Run("Success", func(t *ftt.Test) {
			template := "Hello {{.Name}}, error: {{.Error}}"
			params := map[string]string{
				"Name":  "John",
				"Error": "build failed",
			}

			result, err := server.processTemplate(template, params)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal("Hello John, error: build failed"))
		})

		t.Run("Invalid template syntax", func(t *ftt.Test) {
			template := "Hello {{.Name"
			params := map[string]string{"Name": "John"}

			result, err := server.processTemplate(template, params)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, result, should.BeEmpty)
			assert.Loosely(t, err.Error(), should.ContainSubstring("failed to parse template"))
		})

		t.Run("Missing parameter - should succeed with empty value", func(t *ftt.Test) {
			template := "Hello {{.Name}}, error: {{.MissingParam}}"
			params := map[string]string{"Name": "John"}

			result, err := server.processTemplate(template, params)
			// Go templates don't fail on missing keys by default, they just use zero value
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal("Hello John, error: <no value>"))
		})
	})
}

func TestLoadPromptTemplate(t *testing.T) {
	t.Parallel()
	ftt.Run("loadPromptTemplate", t, func(t *ftt.Test) {
		server := &PromptServer{}

		t.Run("Success with existing template", func(t *ftt.Test) {
			// Test with the actual compilefailureanalysis.md file
			content, err := server.loadPromptTemplate("compilefailureanalysis")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, content, should.ContainSubstring("Software engineer"))
			assert.Loosely(t, content, should.ContainSubstring("{{.Failure}}"))
			assert.Loosely(t, content, should.ContainSubstring("{{.Blamelist}}"))
		})

		t.Run("File not found", func(t *ftt.Test) {
			content, err := server.loadPromptTemplate("nonexistent")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, content, should.BeEmpty)
			assert.Loosely(t, err.Error(), should.ContainSubstring("template file not found"))
		})
	})
}
