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
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCatFileBatchBlob(t *testing.T) {
	t.Parallel()

	commit := "e0d8ad8ebb113bc1eb10be628cd57e3a7684df4f"
	file := "unrelated_file"

	repo := mkRepo(t, fmt.Sprintf("%s:%s", commit, file))

	dat, err := repo.batchProc.catFileBlob(context.Background(), commit, file)
	assert.NoErr(t, err)
	assert.That(t, dat, should.Match([]byte("This file is completely unrelated, and is outside of subdir.\n")))
}
