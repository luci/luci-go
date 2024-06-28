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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateVariant(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateVariant`, t, func() {
		Convey(`empty`, func() {
			err := ValidateVariant(Variant())
			So(err, ShouldBeNil)
		})

		Convey(`invalid`, func() {
			err := ValidateVariant(Variant("1", "b"))
			So(err, ShouldErrLike, `key: does not match`)
		})
	})
}

func TestVariantUtils(t *testing.T) {
	t.Parallel()

	Convey(`Conversion to pair strings works`, t, func() {
		v := Variant(
			"key/with/part/k3", "v3",
			"k1", "v1",
			"key/k2", "v2",
		)
		So(VariantToStrings(v), ShouldResemble, []string{
			"k1:v1", "key/k2:v2", "key/with/part/k3:v3",
		})
	})

	Convey(`Conversion from pair strings works`, t, func() {
		Convey(`for valid pairs`, func() {
			vr, err := VariantFromStrings([]string{"k1:v1", "key/k2:v2", "key/with/part/k3:v3"})
			So(err, ShouldBeNil)
			So(vr, ShouldResembleProto, Variant(
				"k1", "v1",
				"key/k2", "v2",
				"key/with/part/k3", "v3",
			))
		})

		Convey(`for empty list returns nil`, func() {
			vr, err := VariantFromStrings([]string{})
			So(vr, ShouldBeNil)
			So(err, ShouldBeNil)
		})
	})

	Convey(`Key sorting works`, t, func() {
		vr := Variant(
			"k2", "v2",
			"k3", "v3",
			"k1", "v1",
		)
		So(SortedVariantKeys(vr), ShouldResemble, []string{"k1", "k2", "k3"})
	})

	Convey(`VariantToStringPairs`, t, func() {
		pairs := []string{
			"k1", "v1",
			"k2", "v2",
			"k3", "v3",
		}
		vr := Variant(pairs...)
		So(VariantToStringPairs(vr), ShouldResemble, StringPairs(pairs...))
	})

	Convey(`CombineVariant`, t, func() {
		baseVariant := Variant(
			"key/with/part/k3", "v3",
		)
		additionalVariant := Variant(
			// Overwrite the value for the duplicate key.
			"key/with/part/k3", "v4",
			"k1", "v1",
			"key/k2", "v2",
		)
		expectedVariant := Variant(
			"key/with/part/k3", "v4",
			"k1", "v1",
			"key/k2", "v2",
		)
		So(CombineVariant(baseVariant, additionalVariant), ShouldResemble, expectedVariant)
	})

	Convey(`CombineVariant with nil variant`, t, func() {
		var baseVariant *pb.Variant
		var additionalVariant *pb.Variant
		var expectedVariant *pb.Variant
		v1, v2 := Variant("key/with/part/k3", "v3"), Variant("key/with/part/k4", "v4")

		// (nil, nil)
		baseVariant, additionalVariant, expectedVariant = nil, nil, nil
		So(CombineVariant(baseVariant, additionalVariant), ShouldResemble, expectedVariant)

		// (variant, nil)
		baseVariant, additionalVariant, expectedVariant = v1, nil, v1
		So(CombineVariant(baseVariant, additionalVariant), ShouldResemble, expectedVariant)

		// (nil, variant)
		baseVariant, additionalVariant, expectedVariant = nil, v2, v2
		So(CombineVariant(baseVariant, additionalVariant), ShouldResemble, expectedVariant)
	})
}

func TestVariantToJSON(t *testing.T) {
	t.Parallel()
	Convey(`VariantToJSON`, t, func() {
		Convey(`empty`, func() {
			result, err := VariantToJSON(nil)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "{}")
		})
		Convey(`non-empty`, func() {
			variant := &pb.Variant{
				Def: map[string]string{
					"builder":           "linux-rel",
					"os":                "Ubuntu-18.04",
					"pathological-case": "\000\001\n\r\f",
				},
			}
			result, err := VariantToJSON(variant)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, `{"builder":"linux-rel","os":"Ubuntu-18.04","pathological-case":"\u0000\u0001\n\r\f"}`)
		})
	})
}
