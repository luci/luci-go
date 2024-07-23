// Copyright 2015 The LUCI Authors.
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

package memory

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/info"
)

func TestMustNamespace(t *testing.T) {
	ftt.Run("Testable interface works", t, func(t *ftt.Test) {
		c := UseWithAppID(context.Background(), "dev~app-id")

		// Default value.
		assert.Loosely(t, info.AppID(c), should.Equal("app-id"))
		assert.Loosely(t, info.FullyQualifiedAppID(c), should.Equal("dev~app-id"))
		assert.Loosely(t, info.RequestID(c), should.Equal("test-request-id"))
		sa, err := info.ServiceAccount(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, sa, should.Equal("gae_service_account@example.com"))

		// Setting to "override" applies to initial context.
		c = info.GetTestable(c).SetRequestID("override")
		assert.Loosely(t, info.RequestID(c), should.Equal("override"))

		// Derive inner context, "override" applies.
		c = info.MustNamespace(c, "valid_namespace_name")
		assert.Loosely(t, info.RequestID(c), should.Equal("override"))
	})
}
