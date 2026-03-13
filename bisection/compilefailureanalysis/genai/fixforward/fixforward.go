// Copyright 2026 The LUCI Authors.
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

package fixforward

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"google.golang.org/genai"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/llm"
	"go.chromium.org/luci/bisection/model"
)

type LLMResponse struct {
	Files []struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	} `json:"files"`
	Message string `json:"message"`
}

// Response schema for structured LLM output
var fixforwardSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"files": {
			Type: genai.TypeArray,
			Items: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"path": {
						Type:        genai.TypeString,
						Description: "The relative path to the modified file",
					},
					"content": {
						Type:        genai.TypeString,
						Description: "The COMPLETE valid compilable content of the file. No placeholders.",
					},
				},
			},
		},
		"message": {
			Type:        genai.TypeString,
			Description: "The commit message for the fix",
		},
	},
}

var promptTemplate = `You are an experienced Software engineer. A build failed and we identified the culprit CL. A revert of this CL is not possible or desirable, so we want to generate a fix forward CL.
Please generate the fix CL to fix the compile failure.

Failure:
%s

Culprit CL:
%s

%s
`

// GenerateFixforwardCL attempts to generate a fixforward CL for a given compile failure.
func GenerateFixforwardCL(ctx context.Context, genaiClient llm.Client, gerritClient *gerrit.Client, cfa *model.CompileFailureAnalysis, culpritCommit string, failureLog string, repoUrl string, revertURL string) error {
	// 1. Fetch culprit changelog
	changelog, err := gitiles.GetChangeLogsForSingleRevision(ctx, repoUrl, culpritCommit)
	if err != nil {
		return errors.Annotate(err, "failed to get changelog")
	}

	// 2. Fetch file contents for modified files
	var filesInfo string
	for _, diff := range changelog.ChangeLogDiffs {
		if diff.Type == model.ChangeType_DELETE {
			continue
		}
		path := diff.NewPath
		content, err := gitiles.DownloadFile(ctx, repoUrl, culpritCommit, path)
		if err != nil {
			logging.Warningf(ctx, "failed to download file %s: %v", path, err)
			continue
		}
		if len(content) > 500000 {
			logging.Warningf(ctx, "file %s exceeds 500KB size limit, skipping file content generation for LLM prompt", path)
			continue
		}
		filesInfo += fmt.Sprintf("\nFile: %s\n```\n%s\n```\n", path, content)
	}

	if filesInfo == "" {
		return errors.Reason("no modified files within size limit found for culprit %s", culpritCommit)
	}

	// 3. Construct prompt
	clInfo := fmt.Sprintf("commit %s\nAuthor: %s\nMessage:\n%s", changelog.Commit, changelog.Author.Email, changelog.Message)
	prompt := fmt.Sprintf(promptTemplate, failureLog, clInfo, filesInfo)

	logging.Infof(ctx, "Sending prompt to LLM: %s", prompt)

	// 4. Call LLM using Schema
	respText, err := genaiClient.GenerateContentWithSchema(ctx, prompt, fixforwardSchema)
	if err != nil {
		return errors.Annotate(err, "LLM generation failed")
	}

	// 5. Parse LLM response
	respText = strings.TrimSpace(respText)
	respText = strings.TrimPrefix(respText, "```json")
	respText = strings.TrimPrefix(respText, "```")
	respText = strings.TrimSuffix(respText, "```")
	respText = strings.TrimSpace(respText)

	var llmParsed LLMResponse
	if err := json.Unmarshal([]byte(respText), &llmParsed); err != nil {
		return errors.Annotate(err, "failed to parse LLM JSON response")
	}

	if len(llmParsed.Files) == 0 {
		return errors.Reason("LLM generated no file changes for culprit %s", culpritCommit)
	}

	// 6. Create Gerrit CL
	project := "chromium/src"
	if u, err := url.Parse(repoUrl); err == nil && u.Path != "" {
		project = strings.TrimPrefix(u.Path, "/")
	}

	subject := fmt.Sprintf("[experiment fixforward] Fix for culprit %s", culpritCommit[:8])
	if revertURL != "" {
		subject += fmt.Sprintf(" (Reverted in %s)", revertURL)
	}
	subject += fmt.Sprintf("\n\n%s", llmParsed.Message)

	createReq := &gerritpb.CreateChangeRequest{
		Project: project,
		Ref:     "refs/heads/main",
		Subject: subject,
	}
	change, err := gerritClient.CreateChange(ctx, createReq)
	if err != nil {
		return errors.Annotate(err, "failed to create change")
	}

	// 7. Apply edits
	for _, f := range llmParsed.Files {
		editReq := &gerritpb.ChangeEditFileContentRequest{
			Number:   change.Number,
			Project:  project,
			FilePath: f.Path,
			Content:  []byte(f.Content),
		}
		if err := gerritClient.ChangeEditFileContent(ctx, editReq); err != nil {
			logging.Errorf(ctx, "Failed to edit file %s on CL %d: %v", f.Path, change.Number, err)
		}
	}

	// 8. Publish edit
	publishReq := &gerritpb.ChangeEditPublishRequest{
		Number:  change.Number,
		Project: project,
	}
	if err := gerritClient.ChangeEditPublish(ctx, publishReq); err != nil {
		return errors.Annotate(err, "failed to publish edit")
	}

	// 9. Send for review to jiameil@google.com
	_, err = gerritClient.SendForReview(ctx, change, "Please review this GenAI fixforward CL.", []string{"jiameil@google.com"}, nil)
	if err != nil {
		return errors.Annotate(err, "failed to send for review")
	}

	return nil
}
