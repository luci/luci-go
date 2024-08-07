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

package model

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestBotInfoQuery(t *testing.T) {
	t.Parallel()

	ftt.Run("Filters", t, func(t *ftt.Test) {
		q := FilterBotsByState(BotInfoQuery(), StateFilter{
			Quarantined:   apipb.NullableBool_TRUE,
			InMaintenance: apipb.NullableBool_TRUE,
			IsDead:        apipb.NullableBool_TRUE,
			IsBusy:        apipb.NullableBool_TRUE,
		})

		dims, err := NewFilter([]*apipb.StringPair{
			{Key: "k1", Value: "v1|v2"},
			{Key: "k2", Value: "v1|v2"},
			{Key: "k3", Value: "v1"},
		})
		assert.Loosely(t, err, should.BeNil)

		qs := FilterBotsByDimensions(q, SplitOptimally, dims)
		assert.Loosely(t, qs, should.HaveLength(2))

		q1, err := qs[0].Finalize()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, q1.GQL(), should.Equal(
			"SELECT * FROM `BotInfo` "+
				"WHERE `composite` = 1 AND `composite` = 4 AND "+
				"`composite` = 64 AND `composite` = 256 AND "+
				"`dimensions_flat` = \"k1:v1\" AND `dimensions_flat` = \"k3:v1\" AND "+
				"`dimensions_flat` IN ARRAY(\"k2:v1\", \"k2:v2\") ORDER BY `__key__`"))

		q2, err := qs[1].Finalize()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, q2.GQL(), should.Equal(
			"SELECT * FROM `BotInfo` "+
				"WHERE `composite` = 1 AND `composite` = 4 AND "+
				"`composite` = 64 AND `composite` = 256 AND "+
				"`dimensions_flat` = \"k1:v2\" AND `dimensions_flat` = \"k3:v1\" AND "+
				"`dimensions_flat` IN ARRAY(\"k2:v1\", \"k2:v2\") ORDER BY `__key__`"))
	})
}

func TestBotEvent(t *testing.T) {
	t.Parallel()

	ftt.Run("QuarantineMessage", t, func(t *ftt.Test) {
		event := func(state string) *BotEvent {
			return &BotEvent{
				BotCommon: BotCommon{
					State: []byte(state),
				},
			}
		}

		assert.Loosely(t, event(`{"quarantined": "yes", "blah": 1}`).QuarantineMessage(), should.Equal("yes"))
		assert.Loosely(t, event(`{"quarantined": true}`).QuarantineMessage(), should.Equal("true"))
		assert.Loosely(t, event(`{"quarantined": 0}`).QuarantineMessage(), should.Equal("true"))
		assert.Loosely(t, event(`{"quarantined": false}`).QuarantineMessage(), should.BeEmpty)
		assert.Loosely(t, event(`{"quarantined": null}`).QuarantineMessage(), should.BeEmpty)
		assert.Loosely(t, event(`{}`).QuarantineMessage(), should.BeEmpty)
		assert.Loosely(t, event(``).QuarantineMessage(), should.BeEmpty)
		assert.Loosely(t, event(`broken`).QuarantineMessage(), should.BeEmpty)
		assert.Loosely(t, event(`[]`).QuarantineMessage(), should.BeEmpty)
	})
}
