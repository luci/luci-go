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

package revertculprit

import (
	"context"
	"testing"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestHasIrrevertibleAuthor(t *testing.T) {
	ctx := context.Background()

	ftt.Run("HasIrrevertibleAuthor", t, func(t *ftt.Test) {
		change := &gerritpb.ChangeInfo{
			Project:         "chromium/test/src",
			Number:          234567,
			CurrentRevision: "deadbeef",
			Revisions: map[string]*gerritpb.RevisionInfo{
				"deadbeef": {
					Number: 1,
					Kind:   gerritpb.RevisionInfo_REWORK,
					Uploader: &gerritpb.AccountInfo{
						AccountId:       1000096,
						Name:            "John Doe",
						Email:           "jdoe@example.com",
						SecondaryEmails: []string{"johndoe@chromium.org"},
						Username:        "jdoe",
					},
					Ref:         "refs/changes/123",
					Description: "first upload",
					Files: map[string]*gerritpb.FileInfo{
						"go/to/file.go": {
							LinesInserted: 32,
							LinesDeleted:  44,
							SizeDelta:     -567,
							Size:          11984,
						},
					},
					Commit: &gerritpb.CommitInfo{
						Id:      "",
						Message: "Title.\n\nBody is here.\n\nNOAUTOREVERT=true\n\nChange-Id: I100deadbeef",
						Parents: []*gerritpb.CommitInfo_Parent{
							{Id: "deadbeef00"},
						},
					},
				},
			},
		}

		t.Run("author is revertible", func(t *ftt.Test) {
			change.Revisions["deadbeef"].Commit.Author = &gerritpb.GitPersonInfo{
				Name:  "John Doe",
				Email: "jdoe@example.com",
			}

			cannotRevert, err := HasIrrevertibleAuthor(ctx, change)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cannotRevert, should.Equal(false))
		})

		t.Run("author is irrevertible with exact match", func(t *ftt.Test) {
			change.Revisions["deadbeef"].Commit.Author = &gerritpb.GitPersonInfo{
				Name:  "ChromeOS Commit Bot",
				Email: "chromeos-commit-bot@chromium.org",
			}

			cannotRevert, err := HasIrrevertibleAuthor(ctx, change)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cannotRevert, should.Equal(true))
		})

		t.Run("author is irrevertible with pattern match", func(t *ftt.Test) {
			change.Revisions["deadbeef"].Commit.Author = &gerritpb.GitPersonInfo{
				Name:  "Example Service Account",
				Email: "examplechromiumtest-autoroll@skia-buildbots.iam.gserviceaccount.com",
			}

			cannotRevert, err := HasIrrevertibleAuthor(ctx, change)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cannotRevert, should.Equal(true))
		})

		t.Run("author is irrevertible with pattern match extended", func(t *ftt.Test) {
			change.Revisions["deadbeef"].Commit.Author = &gerritpb.GitPersonInfo{
				Name:  "Another Example Service Account",
				Email: "chromium-autoroll@skia-corp.google.com.iam.gserviceaccount.com",
			}

			cannotRevert, err := HasIrrevertibleAuthor(ctx, change)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cannotRevert, should.Equal(true))
		})
	})
}
