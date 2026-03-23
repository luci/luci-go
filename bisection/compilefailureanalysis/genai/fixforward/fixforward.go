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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

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
		Path  string `json:"path"`
		Edits []struct {
			OldText string `json:"old_text"`
			NewText string `json:"new_text"`
		} `json:"edits"`
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
					"edits": {
						Type: genai.TypeArray,
						Items: &genai.Schema{
							Type: genai.TypeObject,
							Properties: map[string]*genai.Schema{
								"old_text": {
									Type:        genai.TypeString,
									Description: "The exact original text to be replaced. Must match exactly, including indentation.",
								},
								"new_text": {
									Type:        genai.TypeString,
									Description: "The new text that will replace the old text.",
								},
							},
						},
						Description: "A list of search/replace edits to apply to the file.",
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
Please generate the fix CL to fix the compile failure. You must output search/replace blocks for ONLY the exact snippets you wish to modify.

Failure:
%s

Culprit CL:
%s

%s
`

// GenerateFixforwardCL attempts to generate a fixforward CL for a given compile failure.
func GenerateFixforwardCL(ctx context.Context, genaiClient llm.Client, gerritClient *gerrit.Client, culpritCommit string, failureLog string, repoUrl string, revertURL string) error {
	// 1. Fetch culprit changelog
	changelog, err := gitiles.GetChangeLogsForSingleRevision(ctx, repoUrl, culpritCommit)
	if err != nil {
		return errors.Annotate(err, "failed to get changelog").Err()
	}

	// 2. Fetch file contents for modified files and neighbor files
	var filesInfo string
	originalFiles := make(map[string]string)
	directories := make(map[string]bool)
	totalInfoSize := 0
	const maxTotalSize = 500000 // 500KB total text size limit for the prompt payload

	for _, diff := range changelog.ChangeLogDiffs {
		if diff.Type == model.ChangeType_DELETE {
			continue
		}
		path := diff.NewPath

		// Record directory for later neighborhood fetch
		dirIdx := strings.LastIndex(path, "/")
		if dirIdx > 0 {
			directories[path[:dirIdx]] = true
		}

		content, err := gitiles.DownloadFile(ctx, repoUrl, culpritCommit, path)
		if err != nil {
			logging.Warningf(ctx, "failed to download file %s: %v", path, err)
			continue
		}
		if len(content) > maxTotalSize {
			logging.Warningf(ctx, "file %s exceeds 500KB size limit, skipping file content generation for LLM prompt", path)
			continue
		}
		originalFiles[path] = content
		fileStr := fmt.Sprintf("\nFile: %s\n```\n%s\n```\n", path, content)
		filesInfo += fileStr
		totalInfoSize += len(fileStr)
	}

	if filesInfo == "" {
		return errors.Reason("no modified files within size limit found for culprit %s", culpritCommit).Err()
	}

	// Fetch neighbor files to expand context
	if totalInfoSize < maxTotalSize {
		// Only check up to 3 directories to avoid massive bursts
		dirCount := 0
		for dir := range directories {
			if dirCount >= 3 {
				break
			}
			dirCount++

			files, err := gitiles.GetDirectoryTree(ctx, repoUrl, culpritCommit, dir)
			if err != nil {
				logging.Warningf(ctx, "failed to get directory tree %s: %v", dir, err)
				continue
			}

			// Append neighbor files in this directory
			for _, file := range files {
				fullPath := dir + "/" + file
				// Skip if we already fetched it
				if _, ok := originalFiles[fullPath]; ok {
					continue
				}

				// Stop if we hit our overall size cap
				if totalInfoSize >= maxTotalSize {
					break
				}

				content, err := gitiles.DownloadFile(ctx, repoUrl, culpritCommit, fullPath)
				if err != nil {
					continue
				}

				fileStr := fmt.Sprintf("\nFile: %s\n```\n%s\n```\n", fullPath, content)
				if totalInfoSize+len(fileStr) > maxTotalSize {
					continue // file too big for remaining cap
				}

				originalFiles[fullPath] = content
				filesInfo += fileStr
				totalInfoSize += len(fileStr)
			}
		}
	}

	// 3. Construct prompt
	clInfo := fmt.Sprintf("commit %s\nAuthor: %s\nMessage:\n%s", changelog.Commit, changelog.Author.Email, changelog.Message)
	prompt := fmt.Sprintf(promptTemplate, failureLog, clInfo, filesInfo)

	logging.Infof(ctx, "Sending raw prompt to LLM:\n%s", prompt)

	// 4. Call LLM using Schema
	var respText string
	for attempt := 1; attempt <= 3; attempt++ {
		respText, err = genaiClient.GenerateContentWithSchema(ctx, prompt, fixforwardSchema)
		if err == nil {
			break
		}
		if attempt < 3 && strings.Contains(err.Error(), "429") {
			waitTime := 60 * time.Second
			logging.Warningf(ctx, "LLM generation hit 429 quota (attempt %d/3). Retrying in %v...", attempt, waitTime)
			time.Sleep(waitTime)
			continue
		}
		break
	}
	if err != nil {
		return errors.Annotate(err, "LLM generation failed after retries").Err()
	}

	logging.Infof(ctx, "Received raw response from LLM:\n%s", respText)

	// 5. Parse LLM response
	respText = strings.TrimSpace(respText)
	respText = strings.TrimPrefix(respText, "```json")
	respText = strings.TrimPrefix(respText, "```")
	respText = strings.TrimSuffix(respText, "```")
	respText = strings.TrimSpace(respText)

	var llmParsed LLMResponse
	if err := json.Unmarshal([]byte(respText), &llmParsed); err != nil {
		return errors.Annotate(err, "failed to parse LLM JSON response").Err()
	}

	if len(llmParsed.Files) == 0 {
		return errors.Reason("LLM generated no file changes for culprit %s", culpritCommit).Err()
	}

	// 6. Create Gerrit CL
	project := "chromium/src"
	if u, err := url.Parse(repoUrl); err == nil && u.Path != "" {
		project = strings.TrimPrefix(u.Path, "/")
	}

	subject := fmt.Sprintf("[experiment fixforward] Fix for culprit %s", culpritCommit[:8])
	if revertURL != "" {
		subject += fmt.Sprintf("\n\nThis change is a fixforward for culprit %s, which was reverted in %s.", culpritCommit[:8], revertURL)
	}

	// Generate a deterministic Change-Id to satisfy Gerrit server hooks
	b := make([]byte, 20)
	if _, err := rand.Read(b); err == nil {
		changeId := "I" + hex.EncodeToString(b)
		subject += fmt.Sprintf("\n\nChange-Id: %s", changeId)
	}

	createReq := &gerritpb.CreateChangeRequest{
		Project: project,
		Ref:     "refs/heads/main",
		Subject: subject,
	}
	change, err := gerritClient.CreateChange(ctx, createReq)
	if err != nil {
		return errors.Annotate(err, "failed to create change").Err()
	}

	// 7. Apply edits
	for _, f := range llmParsed.Files {
		content, ok := originalFiles[f.Path]
		if !ok {
			logging.Warningf(ctx, "LLM returned edits for unknown file %s", f.Path)
			continue
		}
		for _, edit := range f.Edits {
			if strings.Contains(content, edit.OldText) {
				content = strings.Replace(content, edit.OldText, edit.NewText, 1)
			} else {
				logging.Warningf(ctx, "Could not find old_text in file %s to replace", f.Path)
			}
		}

		editReq := &gerritpb.ChangeEditFileContentRequest{
			Number:   change.Number,
			Project:  project,
			FilePath: f.Path,
			Content:  []byte(content),
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
		return errors.Annotate(err, "failed to publish edit").Err()
	}

	// 9. Send for review to jiameil@google.com
	reviewMessage := fmt.Sprintf("Please review this GenAI fixforward CL.\n\n%s", llmParsed.Message)
	_, err = gerritClient.SendForReview(ctx, change, reviewMessage, []string{"jiameil@google.com"}, nil)
	if err != nil {
		return errors.Annotate(err, "failed to send for review").Err()
	}

	return nil
}
