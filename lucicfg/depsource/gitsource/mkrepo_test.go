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

	"go.chromium.org/luci/common/git/testrepo"
	"go.chromium.org/luci/common/testing/truth/assert"
)

func mkRepo(t testing.TB) string {
	ret, err := testrepo.ToGit(context.Background(), "testdata", t.TempDir(), false)
	assert.NoErr(t, err)
	return ret
}
