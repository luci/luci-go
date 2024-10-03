// Copyright 2024 The LUCI Authors.
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

package buganizer

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestBuganizerClient(t *testing.T) {
	ctx := context.Background()
	ftt.Run("Buganizer client", t, func(t *ftt.Test) {
		t.Run("no value in context means no buganizer client", func(t *ftt.Test) {
			client, err := CreateBuganizerClient(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, client, should.BeNil)
		})
		t.Run("empty value in context means no buganizer client", func(t *ftt.Test) {
			ctx = context.WithValue(ctx, &BuganizerClientModeKey, "")
			client, err := CreateBuganizerClient(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, client, should.BeNil)
		})
		t.Run("`disable` mode doesn't create any client", func(t *ftt.Test) {
			ctx = context.WithValue(ctx, &BuganizerClientModeKey, ModeDisable)
			client, err := CreateBuganizerClient(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, client, should.BeNil)
		})
	})
}
