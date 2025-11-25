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

package producersystems

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	configpb "go.chromium.org/luci/resultdb/proto/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestProducerSystem(t *testing.T) {
	t.Parallel()

	ftt.Run("ProducerSystem", t, func(t *ftt.Test) {
		cfg := &configpb.ProducerSystem{
			System:           "atp",
			NamePattern:      `^runs/(?P<run_id>[0-9]+)$`,
			DataRealmPattern: `^prod|test$`,
			UrlTemplate:      "https://atp.com/runs/${run_id}",
			UrlTemplateByDataRealm: map[string]string{
				"test": "https://atp-test.com/runs/${run_id}",
			},
		}
		ps, err := NewProducerSystem(cfg)
		assert.Loosely(t, err, should.BeNil)

		t.Run("Validate", func(t *ftt.Test) {
			t.Run("Valid", func(t *ftt.Test) {
				err := ps.Validate(&pb.ProducerResource{
					System:    "atp",
					Name:      "runs/123",
					DataRealm: "prod",
				})
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("Invalid system", func(t *ftt.Test) {
				err := ps.Validate(&pb.ProducerResource{
					System:    "other",
					Name:      "runs/123",
					DataRealm: "prod",
				})
				assert.Loosely(t, err, should.ErrLike(`system: expected "atp", got "other"`))
			})

			t.Run("Invalid name", func(t *ftt.Test) {
				err := ps.Validate(&pb.ProducerResource{
					System:    "atp",
					Name:      "invalid",
					DataRealm: "prod",
				})
				assert.Loosely(t, err, should.ErrLike(`name: does not match pattern "^runs/(?P<run_id>[0-9]+)$"`))
			})

			t.Run("Invalid data_realm", func(t *ftt.Test) {
				err := ps.Validate(&pb.ProducerResource{
					System:    "atp",
					Name:      "runs/123",
					DataRealm: "other",
				})
				assert.Loosely(t, err, should.ErrLike(`data_realm: does not match pattern "^prod|test$"`))
			})
		})

		t.Run("GenerateURL", func(t *ftt.Test) {
			t.Run("Default template", func(t *ftt.Test) {
				url, err := ps.GenerateURL(&pb.ProducerResource{
					System:    "atp",
					Name:      "runs/123",
					DataRealm: "prod",
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, url, should.Equal("https://atp.com/runs/123"))
			})

			t.Run("Data realm override", func(t *ftt.Test) {
				url, err := ps.GenerateURL(&pb.ProducerResource{
					System:    "atp",
					Name:      "runs/123",
					DataRealm: "test",
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, url, should.Equal("https://atp-test.com/runs/123"))
			})
		})
	})
}
