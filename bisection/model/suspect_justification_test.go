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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSuspectJutification(t *testing.T) {
	t.Parallel()

	ftt.Run("SuspectJutification", t, func(t *ftt.Test) {
		justification := &SuspectJustification{}
		justification.AddItem(10, "a/b", "fileInLog", JustificationType_FAILURELOG)
		assert.Loosely(t, justification.GetScore(), should.Equal(10))
		justification.AddItem(2, "c/d", "fileInDependency1", JustificationType_DEPENDENCY)
		assert.Loosely(t, justification.GetScore(), should.Equal(12))
		justification.AddItem(8, "e/f", "fileInDependency2", JustificationType_DEPENDENCY)
		assert.Loosely(t, justification.GetScore(), should.Equal(19))
	})
}
