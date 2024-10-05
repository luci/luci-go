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

package rotationproxy

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGetOnCallEmails(t *testing.T) {
	ctx := context.Background()
	ctx = MockedRotationProxyClientContext(ctx, map[string]string{
		"oncallator:chrome-build-sheriff": `{"emails":["jdoe@example.com", "esmith@example.com"],"updated_unix_timestamp":1669331526}`,
	})

	ftt.Run("Chromium arborists are returned", t, func(t *ftt.Test) {
		emails, err := GetOnCallEmails(ctx, "chromium/src")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, emails, should.Resemble([]string{"jdoe@example.com", "esmith@example.com"}))
	})

	ftt.Run("unknown project", t, func(t *ftt.Test) {
		_, err := GetOnCallEmails(ctx, "infra/infra")
		assert.Loosely(t, err, should.ErrLike("could not get on-call rotation for project"))
		assert.Loosely(t, err, should.ErrLike("infra/infra"))
	})
}
