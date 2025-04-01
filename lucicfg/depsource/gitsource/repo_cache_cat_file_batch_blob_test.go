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

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCatFileBatchBlob(t *testing.T) {
	t.Parallel()

	repo := mkRepo(t)

	dat, err := repo.batchProcLazy.catFileBlob(context.Background(), "e0d8ad8ebb113bc1eb10be628cd57e3a7684df4f", "unrelated_file")
	assert.NoErr(t, err)
	assert.That(t, dat, should.Match([]byte("This file is completely unrelated, and is outside of subdir.\n")))
}
