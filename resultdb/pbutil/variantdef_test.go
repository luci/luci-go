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

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

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
	t.Parallel()

	Convey(`Conversion to pair strings works`, t, func() {
		def := &pb.VariantDef{Def: map[string]string{
			"key/with/part/k3": "v3",
			"k1":               "v1",
			"key/k2":           "v2",
		}}
		So(VariantDefToStrings(def), ShouldResemble, []string{
			"k1:v1", "key/k2:v2", "key/with/part/k3:v3",
		})
	})

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

		Convey(`for empty list returns nil`, func() {
			def, err := VariantDefFromStrings([]string{})
			So(def, ShouldBeNil)
			So(err, ShouldBeNil)
		})
	})

	Convey(`Key sorting works`, t, func() {
		def := &pb.VariantDef{Def: map[string]string{
			"k2": "v2",
			"k3": "v3",
			"k1": "v1",
		}}

		So(SortedVariantDefKeys(def), ShouldResemble, []string{"k1", "k2", "k3"})
	})
}

func TestValidateTestVariant(t *testing.T) {
	Convey(`TestValidateTestVariant`, t, func() {
		Convey(`empty`, func() {
			tv := &pb.TestVariant{}
			err := ValidateTestVariant(tv)
			So(err, ShouldErrLike, "test_path: unspecified")
		})

		Convey(`NUL in test path`, func() {
			tv := &pb.TestVariant{TestPath: "\x01"}
			err := ValidateTestVariant(tv)
			So(err, ShouldErrLike, "test_path: does not match")
		})

		Convey(`invalid variant def`, func() {
			tv := &pb.TestVariant{
				TestPath: "a",
				Variant: &pb.VariantDef{
					Def: map[string]string{"": ""},
				},
			}
			err := ValidateTestVariant(tv)
			So(err, ShouldErrLike, `variant: "":"": key: does not match`)
		})
	})
}
