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
	"slices"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestOrder(t *testing.T) {
	t.Parallel()

	repo := mkRepo(t)

	// git log output from test repo
	commitsActualOrder := []string{
		"9c350ff8eeacef414a4baa5d68098e61b2d51a51",
		"ba5f5f958d0c29033c262df7a8507aa9df47f337",
		"73bd1be998e9effd633fd2fc4428f8081b231fb6",
		"90eb749377df38ad4a3d4622ca083cafac45364c",
		"e0d8ad8ebb113bc1eb10be628cd57e3a7684df4f",
		"7b1085476b595ec17c51ffb664766efa2d3b2a28",
		"2ee3f86c1b7f56cf8f9341b7d03064645ec99913",
		"0e63e40753dca5f413b7462add8566559d8083b5",
	}

	// give it all the commits in alphanumeric order
	ordered, err := repo.Order(context.Background(), "refs/heads/main",
		slices.Sorted(slices.Values(commitsActualOrder)))
	assert.NoErr(t, err)

	// reverse order - repo.Order returns in oldest-to-newest, and git log is in
	// newest-to-oldest
	slices.Reverse(ordered)
	assert.That(t, ordered, should.Match(commitsActualOrder))
}
