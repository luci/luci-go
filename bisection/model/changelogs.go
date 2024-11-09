// Copyright 2022 The LUCI Authors.
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

package model

import (
	"fmt"
	"regexp"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
)

// ChangeLog represents the changes of a revision
type ChangeLog struct {
	Commit         string          `json:"commit"`
	Tree           string          `json:"tree"`
	Parents        []string        `json:"parents"`
	Author         ChangeLogActor  `json:"author"`
	Committer      ChangeLogActor  `json:"committer"`
	Message        string          `json:"message"`
	ChangeLogDiffs []ChangeLogDiff `json:"tree_diff"`
}

type ChangeLogActor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Time  string `json:"time"`
}

type ChangeLogDiff struct {
	Type    ChangeType `json:"type"`
	OldID   string     `json:"old_id"`
	OldMode int        `json:"old_mode"`
	OldPath string     `json:"old_path"`
	NewID   string     `json:"new_id"`
	NewMode int        `json:"new_mode"`
	NewPath string     `json:"new_path"`
}

type ChangeType string

const (
	ChangeType_ADD    = "add"
	ChangeType_MODIFY = "modify"
	ChangeType_COPY   = "copy"
	ChangeType_RENAME = "rename"
	ChangeType_DELETE = "delete"
)

// ChangeLogResponse represents the response from gitiles for changelog.
type ChangeLogResponse struct {
	Log  []*ChangeLog `json:"log"`
	Next string       `json:"next"` // From next revision
}

// GetReviewUrl returns the review URL of the changelog.
func (cl *ChangeLog) GetReviewUrl() (string, error) {
	pattern := regexp.MustCompile("\\nReviewed-on: (https://.+)\\n")
	matches := pattern.FindStringSubmatch(cl.Message)
	if matches == nil {
		return "", fmt.Errorf("Could not find review CL URL. Message: %s", cl.Message)
	}
	return matches[1], nil
}

// GetReviewTitle returns the review title from the changelog.
func (cl *ChangeLog) GetReviewTitle() (string, error) {
	pattern := regexp.MustCompile("\\A([^\\n]+)\\n")
	matches := pattern.FindStringSubmatch(cl.Message)
	if matches == nil {
		return "", fmt.Errorf("Could not find review title. Message: %s", cl.Message)
	}
	return matches[1], nil
}

func (cl *ChangeLog) GetCommitTime() (*timestamppb.Timestamp, error) {
	timeStr := cl.Committer.Time
	layout := "Mon Jan 02 15:04:05 2006"
	parsedTime, err := time.ParseInLocation(layout, timeStr, time.UTC)
	if err != nil {
		return nil, errors.Annotate(err, "parse time %s", timeStr).Err()
	}
	return timestamppb.New(parsedTime), nil
}
