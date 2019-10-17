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

package pbutil

import (
	"testing"

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateVariantDef(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateVariantDef`, t, func() {
		Convey(`empty`, func() {
			err := ValidateVariantDef(&pb.VariantDef{})
			So(err, ShouldBeNil)
		})

		Convey(`invalid`, func() {
			err := ValidateVariantDef(&pb.VariantDef{
				Def: map[string]string{
					"1": "b",
				},
			})
			So(err, ShouldErrLike, `key: does not match`)
		})
	})
}

func TestVariantDefUtils(t *testing.T) {
	Convey(`Conversion from pair strings works`, t, func() {
		Convey(`for valid pairs`, func() {
			def, err := VariantDefFromStrings([]string{"k1:v1", "key/k2:v2", "key/with/part/k3:v3"})
			So(err, ShouldBeNil)
			So(def, ShouldResembleProto, &pb.VariantDef{Def: map[string]string{
				"k1":               "v1",
				"key/k2":           "v2",
				"key/with/part/k3": "v3",
			}})
		})
	})
}
