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

	apipb "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBotInfoQuery(t *testing.T) {
	t.Parallel()

	Convey("Filters", t, func() {
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
		So(err, ShouldBeNil)

		qs := FilterBotsByDimensions(q, SplitOptimally, dims)
		So(qs, ShouldHaveLength, 2)

		q1, err := qs[0].Finalize()
		So(err, ShouldBeNil)
		So(q1.GQL(), ShouldEqual,
			"SELECT * FROM `BotInfo` "+
				"WHERE `composite` = 1 AND `composite` = 4 AND "+
				"`composite` = 64 AND `composite` = 256 AND "+
				"`dimensions_flat` = \"k1:v1\" AND `dimensions_flat` = \"k3:v1\" AND "+
				"`dimensions_flat` IN ARRAY(\"k2:v1\", \"k2:v2\") ORDER BY `__key__`")

		q2, err := qs[1].Finalize()
		So(err, ShouldBeNil)
		So(q2.GQL(), ShouldEqual,
			"SELECT * FROM `BotInfo` "+
				"WHERE `composite` = 1 AND `composite` = 4 AND "+
				"`composite` = 64 AND `composite` = 256 AND "+
				"`dimensions_flat` = \"k1:v2\" AND `dimensions_flat` = \"k3:v1\" AND "+
				"`dimensions_flat` IN ARRAY(\"k2:v1\", \"k2:v2\") ORDER BY `__key__`")
	})
}
