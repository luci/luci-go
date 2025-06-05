// Copyright 2020 The LUCI Authors.
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

package poller

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGerritQueryString(t *testing.T) {
	t.Parallel()

	ftt.Run("gerritString works", t, func(t *ftt.Test) {
		qs := &QueryState{}

		t.Run("single project", func(t *ftt.Test) {
			qs.OrProjects = []string{"inf/ra"}
			assert.That(t, qs.gerritString(queryLimited), should.Equal(
				`status:NEW label:Commit-Queue>0 project:"inf/ra"`))
		})

		t.Run("many projects", func(t *ftt.Test) {
			qs.OrProjects = []string{"inf/ra", "second"}
			assert.That(t, qs.gerritString(queryLimited), should.Equal(
				`status:NEW label:Commit-Queue>0 (project:"inf/ra" OR project:"second")`))
		})

		t.Run("shared prefix", func(t *ftt.Test) {
			qs.CommonProjectPrefix = "shared"
			assert.That(t, qs.gerritString(queryLimited), should.Equal(
				`status:NEW label:Commit-Queue>0 projects:"shared"`))
		})

		t.Run("unlimited", func(t *ftt.Test) {
			qs.CommonProjectPrefix = "shared"
			assert.That(t, qs.gerritString(queryAll), should.Equal(
				`projects:"shared"`))
		})
	})
}
