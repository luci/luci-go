// Copyright 2026 The LUCI Authors.
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

package base

import (
	"context"
	"testing"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

func TestNewAnalysisClients(t *testing.T) {
	ftt.Run(`NewAnalysisClients`, t, func(t *ftt.Test) {
		ctx := context.Background()
		af := &AuthFlags{DefaultOpts: auth.Options{SecretsDir: t.TempDir()}}

		// Without Parse(), NewAnalysisClients returns an error.
		_, _, _, err := af.NewAnalysisClients(ctx, chromeinfra.AnalysisHost)
		assert.Loosely(t, err, should.NotBeNil)

		// After Parse(), NewAnalysisClients returns all three pRPC clients.
		assert.Loosely(t, af.Parse(), should.BeNil)
		rulesClient, clustersClient, testHistoryClient, err := af.NewAnalysisClients(ctx, chromeinfra.AnalysisHost)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, rulesClient, should.NotBeNil)
		assert.Loosely(t, clustersClient, should.NotBeNil)
		assert.Loosely(t, testHistoryClient, should.NotBeNil)
	})
}
