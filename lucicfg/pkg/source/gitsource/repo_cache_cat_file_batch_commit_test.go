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

package gitsource

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCommitParse(t *testing.T) {
	ctx := context.Background()

	run := func(message []string, expect commitObj) func(t *testing.T) {
		return func(t *testing.T) {
			actual := commitObj{}
			assert.NoErr(t, actual.parse(ctx, message))
			assert.That(t, actual, should.Match(expect))
		}
	}

	t.Run("basic", run([]string{
		"tree 87e5668936da787a6ff47774f61aa7053e9f5144",
		"parent 02d24d062e5e98992724f0c70780ebbf48c8b963",
		"author Guy Fleegman <fleegman@example.com> 1740713039 -0800",
		"committer Chromium LUCI CQ <chromium-scoped@luci-project-accounts.iam.gserviceaccount.com> 1740713039 -0800",
		"",
		"[Vestibulum] Lorem ipsum dolor sit amet.",
		"",
		"Morbi facilisis sit amet ipsum vel porttitor.",
		"Curabitur condimentum consequat lectus fermentum tempus.",
		"",
		"Bug: 1234567890",
		"Change-Id: Ifc6bc4017e98e6d8f99ac3950a1c1d5b8b909f33",
		"Reviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/123456",
		"Commit-Queue: Guy Fleegman <fleegman@example.com>",
		"Reviewed-by: Ace Codegal <ace@example.com>",
		"Reviewed-by: Binny Truli <binny@example.org>",
		"Cr-Commit-Position: refs/heads/main@{#123456}",
	}, commitObj{
		Tree: "87e5668936da787a6ff47774f61aa7053e9f5144",
		Parents: []string{
			"02d24d062e5e98992724f0c70780ebbf48c8b963",
		},
		Author:        "Guy Fleegman <fleegman@example.com>",
		AuthorTime:    time.Unix(1740713039, 0).In(time.FixedZone("-800", -800)),
		Committer:     "Chromium LUCI CQ <chromium-scoped@luci-project-accounts.iam.gserviceaccount.com>",
		CommitterTime: time.Unix(1740713039, 0).In(time.FixedZone("-800", -800)),
		MessageLines: []string{
			"[Vestibulum] Lorem ipsum dolor sit amet.",
			"",
			"Morbi facilisis sit amet ipsum vel porttitor.",
			"Curabitur condimentum consequat lectus fermentum tempus.",
		},
		Trailers: map[string][]string{
			"Bug":                {"1234567890"},
			"Change-Id":          {"Ifc6bc4017e98e6d8f99ac3950a1c1d5b8b909f33"},
			"Reviewed-on":        {"https://chromium-review.googlesource.com/c/chromium/src/+/123456"},
			"Commit-Queue":       {"Guy Fleegman <fleegman@example.com>"},
			"Reviewed-by":        {"Ace Codegal <ace@example.com>", "Binny Truli <binny@example.org>"},
			"Cr-Commit-Position": {"refs/heads/main@{#123456}"},
		},
	}))
}

func TestCommitParseActual(t *testing.T) {
	t.Parallel()

	commit := "9c350ff8eeacef414a4baa5d68098e61b2d51a51"

	repo := mkRepo(t, commit)

	cmt, err := repo.batchProc.catFileCommit(context.Background(), commit)
	assert.NoErr(t, err)

	assert.That(t, cmt.Author, should.Equal("Exemplar Exemplaris <exemplar@example.com>"))
	assert.That(t, cmt.Committer, should.Equal("Exemplar Exemplaris <exemplar@example.com>"))
	assert.That(t, cmt.MessageLines, should.Match([]string{
		"Modify unrelated_file",
		"",
		"This is a commit message body.",
	}))
	assert.That(t, cmt.Trailers, should.Match(map[string][]string{
		"Other-Footer": {"Cool beans!"},
		"Weird-Footer": {"100", "200"},
	}))
}
