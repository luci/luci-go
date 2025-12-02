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

package masking

import (
	"regexp"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/producersystems"
	"go.chromium.org/luci/resultdb/internal/workunits"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestProducerResourceURL(t *testing.T) {
	t.Parallel()
	ftt.Run("TestProducerResourceURL", t, func(t *ftt.Test) {
		cfg := &config.CompiledServiceConfig{
			ProducerSystems: map[string]*producersystems.ProducerSystem{
				"test-system": {
					System:           "test-system",
					NamePattern:      regexp.MustCompile(`^test-run/(?P<name>.+)$`),
					DataRealmPattern: regexp.MustCompile(`^test-realm$`),
					URLTemplate:      "http://test-url.com/runs/${name}",
				},
			},
		}

		t.Run("ProducerResource", func(t *ftt.Test) {
			pr := &resultpb.ProducerResource{
				System:    "test-system",
				Name:      "test-run/run-name",
				DataRealm: "test-realm",
			}

			t.Run("Producer system with configuration", func(t *ftt.Test) {
				assert.That(t, producerResourceURL(pr, cfg), should.Equal("http://test-url.com/runs/run-name"))
			})

			t.Run("Producer system without configuration", func(t *ftt.Test) {
				pr.System = "unknown-system"
				assert.That(t, producerResourceURL(pr, cfg), should.Equal(""))
			})

			t.Run("ProducerResource with non-parsable name", func(t *ftt.Test) {
				pr.Name = "old-name-format/run-name"
				assert.That(t, producerResourceURL(pr, cfg), should.Equal(""))
			})
		})
	})
}

func TestWorkUnitETag(t *testing.T) {
	t.Parallel()
	ftt.Run("TestWorkUnitETag", t, func(t *ftt.Test) {
		lastUpdatedTime := time.Date(2025, 4, 26, 1, 2, 3, 4000, time.UTC)
		wu := &workunits.WorkUnitRow{LastUpdated: lastUpdatedTime}

		t.Run("LimitedAccess BasicView", func(t *ftt.Test) {
			etag := WorkUnitETag(wu, permissions.LimitedAccess, resultpb.WorkUnitView_WORK_UNIT_VIEW_BASIC)
			assert.That(t, etag, should.Equal(`W/"+l/2025-04-26T01:02:03.000004Z"`))
		})

		t.Run("FullAccess FullView", func(t *ftt.Test) {
			etag := WorkUnitETag(wu, permissions.FullAccess, resultpb.WorkUnitView_WORK_UNIT_VIEW_FULL)
			assert.That(t, etag, should.Equal(`W/"+f/2025-04-26T01:02:03.000004Z"`))
		})

		t.Run("LimitedAccess FullView", func(t *ftt.Test) {
			etag := WorkUnitETag(wu, permissions.LimitedAccess, resultpb.WorkUnitView_WORK_UNIT_VIEW_FULL)
			assert.That(t, etag, should.Equal(`W/"+l+f/2025-04-26T01:02:03.000004Z"`))
		})

		t.Run("FullAccess BasicView", func(t *ftt.Test) {
			etag := WorkUnitETag(wu, permissions.FullAccess, resultpb.WorkUnitView_WORK_UNIT_VIEW_BASIC)
			assert.That(t, etag, should.Equal(`W/"/2025-04-26T01:02:03.000004Z"`))
		})

		t.Run("FullAccess UnspecifiedView", func(t *ftt.Test) {
			etag := WorkUnitETag(wu, permissions.FullAccess, resultpb.WorkUnitView_WORK_UNIT_VIEW_UNSPECIFIED)
			assert.That(t, etag, should.Equal(`W/"/2025-04-26T01:02:03.000004Z"`))
		})

		t.Run("NoAccess panics", func(t *ftt.Test) {
			assert.That(t, func() {
				WorkUnitETag(wu, permissions.NoAccess, resultpb.WorkUnitView_WORK_UNIT_VIEW_BASIC)
			}, should.Panic)
		})
	})

	ftt.Run("TestParseWorkUnitETag", t, func(t *ftt.Test) {
		t.Run("valid etag", func(t *ftt.Test) {
			etag := `W/"+l/2025-04-26T01:02:03.000004Z"`
			lastUpdated, err := ParseWorkUnitETag(etag)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, lastUpdated, should.Equal("2025-04-26T01:02:03.000004Z"))
		})

		t.Run("malformed etag", func(t *ftt.Test) {
			etag := `W/"2025-04-26T01:02:03.000004Z"` // Missing access/view filter
			_, err := ParseWorkUnitETag(etag)
			assert.Loosely(t, err, should.ErrLike("malformated etag"))
		})
	})

	ftt.Run("TestIsWorkUnitETagMatch", t, func(t *ftt.Test) {
		lastUpdatedTime := time.Date(2025, 4, 26, 1, 2, 3, 4000, time.UTC)
		wu := &workunits.WorkUnitRow{LastUpdated: lastUpdatedTime}

		t.Run("round-trip match", func(t *ftt.Test) {
			etag := WorkUnitETag(wu, permissions.FullAccess, resultpb.WorkUnitView_WORK_UNIT_VIEW_BASIC)

			match, err := IsWorkUnitETagMatch(wu, etag)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, match, should.BeTrue)
		})

		t.Run("not match", func(t *ftt.Test) {
			etag := WorkUnitETag(wu, permissions.FullAccess, resultpb.WorkUnitView_WORK_UNIT_VIEW_BASIC)

			wu.LastUpdated = wu.LastUpdated.Add(time.Second)
			match, err := IsWorkUnitETagMatch(wu, etag)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, match, should.BeFalse)
		})

		t.Run("malformed", func(t *ftt.Test) {
			etag := `W/"+a/2025-04-26T01:02:03.000004Z"`

			_, err := IsWorkUnitETagMatch(wu, etag)
			assert.Loosely(t, err, should.ErrLike("malformated etag"))
		})
	})
}
