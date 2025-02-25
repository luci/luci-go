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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateVariant(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateVariant`, t, func(t *ftt.Test) {
		t.Run(`empty`, func(t *ftt.Test) {
			err := ValidateVariant(Variant())
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`invalid`, func(t *ftt.Test) {
			err := ValidateVariant(Variant("1", "b"))
			assert.Loosely(t, err, should.ErrLike(`key: does not match`))
		})
	})
}

func TestVariantUtils(t *testing.T) {
	t.Parallel()

	ftt.Run(`Conversion to pair strings works`, t, func(t *ftt.Test) {
		v := Variant(
			"key/with/part/k3", "v3",
			"k1", "v1",
			"key/k2", "v2",
		)
		assert.Loosely(t, VariantToStrings(v), should.Match([]string{
			"k1:v1", "key/k2:v2", "key/with/part/k3:v3",
		}))
	})

	ftt.Run(`Conversion from pair strings works`, t, func(t *ftt.Test) {
		t.Run(`for valid pairs`, func(t *ftt.Test) {
			vr, err := VariantFromStrings([]string{"k1:v1", "key/k2:v2", "key/with/part/k3:v3"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, vr, should.Match(Variant(
				"k1", "v1",
				"key/k2", "v2",
				"key/with/part/k3", "v3",
			)))
		})

		t.Run(`for empty list returns nil`, func(t *ftt.Test) {
			vr, err := VariantFromStrings([]string{})
			assert.Loosely(t, vr, should.BeNil)
			assert.Loosely(t, err, should.BeNil)
		})
	})

	ftt.Run(`Key sorting works`, t, func(t *ftt.Test) {
		vr := Variant(
			"k2", "v2",
			"k3", "v3",
			"k1", "v1",
		)
		assert.Loosely(t, SortedVariantKeys(vr), should.Match([]string{"k1", "k2", "k3"}))
	})

	ftt.Run(`VariantToStringPairs`, t, func(t *ftt.Test) {
		pairs := []string{
			"k1", "v1",
			"k2", "v2",
			"k3", "v3",
		}
		vr := Variant(pairs...)
		assert.Loosely(t, VariantToStringPairs(vr), should.Match(StringPairs(pairs...)))
	})

	ftt.Run(`CombineVariant`, t, func(t *ftt.Test) {
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
		assert.Loosely(t, CombineVariant(baseVariant, additionalVariant), should.Match(expectedVariant))
	})

	ftt.Run(`CombineVariant with nil variant`, t, func(t *ftt.Test) {
		var baseVariant *pb.Variant
		var additionalVariant *pb.Variant
		var expectedVariant *pb.Variant
		v1, v2 := Variant("key/with/part/k3", "v3"), Variant("key/with/part/k4", "v4")

		// (nil, nil)
		baseVariant, additionalVariant, expectedVariant = nil, nil, nil
		assert.Loosely(t, CombineVariant(baseVariant, additionalVariant), should.Match(expectedVariant))

		// (variant, nil)
		baseVariant, additionalVariant, expectedVariant = v1, nil, v1
		assert.Loosely(t, CombineVariant(baseVariant, additionalVariant), should.Match(expectedVariant))

		// (nil, variant)
		baseVariant, additionalVariant, expectedVariant = nil, v2, v2
		assert.Loosely(t, CombineVariant(baseVariant, additionalVariant), should.Match(expectedVariant))
	})
}

func TestVariantToJSON(t *testing.T) {
	t.Parallel()
	ftt.Run(`VariantToJSON`, t, func(t *ftt.Test) {
		t.Run(`empty`, func(t *ftt.Test) {
			result, err := VariantToJSON(nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal("{}"))
		})
		t.Run(`non-empty`, func(t *ftt.Test) {
			variant := &pb.Variant{
				Def: map[string]string{
					"builder":           "linux-rel",
					"os":                "Ubuntu-18.04",
					"pathological-case": "\000\001\n\r\f",
				},
			}
			result, err := VariantToJSON(variant)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal(`{"builder":"linux-rel","os":"Ubuntu-18.04","pathological-case":"\u0000\u0001\n\r\f"}`))
		})
	})
}
