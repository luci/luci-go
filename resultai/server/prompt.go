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
	"embed"
	"fmt"
	"strings"
	"text/template"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultai/llm"
	pb "go.chromium.org/luci/resultai/proto/v1"
)

//go:embed prompts/*.md
var promptTemplates embed.FS

type PromptServer struct {
	pb.UnimplementedPromptServer
	LlmClient llm.Client
}

func (s *PromptServer) ExecutePrompt(ctx context.Context, req *pb.ExecutePromptRequest) (resp *pb.ExecutePromptResponse, err error) {
	// Extract prompt template name from resource name
	// Format: promptTemplates/{prompt_template}
	parts := strings.Split(req.Name, "/")
	if len(parts) != 2 || parts[0] != "promptTemplates" {
		return nil, status.Errorf(codes.InvalidArgument, "invalid prompt template name format, expected: promptTemplates/{template_name}")
	}
	templateName := parts[1]

	// Load prompt template
	promptContent, err := s.loadPromptTemplate(templateName)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "failed to load prompt template: %s", err)
	}

	// Process template with parameters
	processedPrompt, err := s.processTemplate(promptContent, req.PromptParams)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to process template: %s", err)
	}

	// Generate content using LLM client
	output, err := s.LlmClient.GenerateContent(ctx, processedPrompt)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate content: %s", err)
	}

	return &pb.ExecutePromptResponse{
		PromptOutput: output,
	}, nil
}

func (s *PromptServer) loadPromptTemplate(templateName string) (string, error) {
	filePath := fmt.Sprintf("prompts/%s.md", templateName)
	content, err := promptTemplates.ReadFile(filePath)
	if err != nil {
		return "", errors.Fmt("template file not found: %s", templateName)
	}
	return string(content), nil
}

func (s *PromptServer) processTemplate(templateContent string, params map[string]string) (string, error) {
	tmpl, err := template.New("prompt").Parse(templateContent)
	if err != nil {
		return "", errors.Fmt("failed to parse template: %w", err)
	}

	var result strings.Builder
	err = tmpl.Execute(&result, params)
	if err != nil {
		return "", errors.Fmt("failed to execute template: %w", err)
	}

	return result.String(), nil
}
