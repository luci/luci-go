// Copyright 2019 The LUCI Authors.
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

package formats

import (
	"testing"

	"github.com/golang/protobuf/ptypes/timestamp"

	. "github.com/smartystreets/goconvey/convey"

	resultspb "go.chromium.org/luci/results/proto/v1"
)

func TestTimeConversion(t *testing.T) {
	Convey(`Works`, t, func() {
		Convey(`with whole seconds`, func() {
			ts := secondsToTimestamp(float64(1567740569))
			So(ts, ShouldResemble, &timestamp.Timestamp{Seconds: 1567740569})
		})

		Convey(`with fractions of seconds`, func() {
			ts := secondsToTimestamp(float64(1567740569.147629))
			So(ts.Seconds, ShouldEqual, 1567740569)
			So(ts.Nanos, ShouldAlmostEqual, 147629000, 100)
		})
	})
}

func TestVariantUtils(t *testing.T) {
	Convey(`Map conversion works`, t, func() {
		def := map[string]string{
			"k2": "v2",
			"k3": "v3",
			"k1": "v1",
		}

		Convey(`with presorted keys`, func() {
			varpb := getVariantFromDefMap(def, []string{"k1", "k2", "k3"})
			So(varpb, ShouldResemble, &resultspb.VariantDef{
				Def: map[string]string{
					"k1": "v1",
					"k2": "v2",
					"k3": "v3",
				},
				Digest: string([]byte{200, 131, 176, 65, 74, 65, 110, 8, 188, 253, 143, 179, 55, 140, 121, 192, 193, 199, 233, 91, 35, 178, 42, 240, 92, 156, 189, 85, 237, 163, 5, 89}),
			})
		})
		Convey(`without presorted keys`, func() {
			varpb := getVariantFromDefMap(def, []string{})
			So(varpb, ShouldResemble, &resultspb.VariantDef{
				Def: map[string]string{
					"k1": "v1",
					"k2": "v2",
					"k3": "v3",
				},
				Digest: string([]byte{200, 131, 176, 65, 74, 65, 110, 8, 188, 253, 143, 179, 55, 140, 121, 192, 193, 199, 233, 91, 35, 178, 42, 240, 92, 156, 189, 85, 237, 163, 5, 89}),
			})
		})
	})
}
