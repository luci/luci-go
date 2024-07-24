// Copyright 2019 The LUCI Authors.
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

package git

import (
	"context"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"testing"
)

func TestProjectContext(t *testing.T) {
	t.Parallel()

	ftt.Run("Context annotation works", t, func(t *ftt.Test) {
		ctx := context.Background()
		_, ok := ProjectFromContext(ctx)
		assert.Loosely(t, ok, should.BeFalse)

		WithProject(ctx, "")
		_, ok = ProjectFromContext(ctx)
		assert.Loosely(t, ok, should.BeFalse)

		ctx = WithProject(ctx, "luci-project")
		project, ok := ProjectFromContext(ctx)
		assert.Loosely(t, project, should.Match("luci-project"))
		assert.Loosely(t, ok, should.BeTrue)
	})
}
