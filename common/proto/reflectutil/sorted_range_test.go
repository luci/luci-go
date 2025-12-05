// Copyright 2022 The LUCI Authors.
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

package reflectutil

import (
	"sort"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type boolStrItem struct {
	k bool
	v string
}

type int64StrItem struct {
	k int64
	v string
}
type uint64StrItem struct {
	k uint64
	v string
}

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(boolStrItem{}))
	registry.RegisterCmpOption(cmp.AllowUnexported(int64StrItem{}))
	registry.RegisterCmpOption(cmp.AllowUnexported(uint64StrItem{}))
}

func TestMapRangeSorted(t *testing.T) {
	t.Parallel()

	ftt.Run(`MapRangeSorted`, t, func(t *ftt.Test) {
		t.Run(`empty`, func(t *ftt.Test) {
			msg := &TestMapMessage{}
			refl := msg.ProtoReflect()
			getField := func() protoreflect.Map { return refl.Get(refl.Descriptor().Fields().ByName("string_map")).Map() }
			for range MapRangeSorted(getField(), protoreflect.StringKind) {
				panic("callback called for empty map?")
			}
		})

		t.Run(`bool`, func(t *ftt.Test) {
			msg := &TestMapMessage{}
			refl := msg.ProtoReflect()
			getField := func() protoreflect.Map { return refl.Get(refl.Descriptor().Fields().ByName("bool_map")).Map() }

			t.Run(`true`, func(t *ftt.Test) {
				msg.BoolMap = map[bool]string{true: "hey"}
				results := []boolStrItem{}
				for k, v := range MapRangeSorted(getField(), protoreflect.BoolKind) {
					results = append(results, boolStrItem{k.Bool(), v.String()})
				}
				assert.That(t, results, should.Match([]boolStrItem{{true, "hey"}}))
			})
			t.Run(`false`, func(t *ftt.Test) {
				msg.BoolMap = map[bool]string{false: "hey"}
				results := []boolStrItem{}
				for k, v := range MapRangeSorted(getField(), protoreflect.BoolKind) {
					results = append(results, boolStrItem{k.Bool(), v.String()})
				}
				assert.That(t, results, should.Match([]boolStrItem{{false, "hey"}}))
			})
			t.Run(`both`, func(t *ftt.Test) {
				msg.BoolMap = map[bool]string{false: "hey", true: "norp"}
				results := []boolStrItem{}
				for k, v := range MapRangeSorted(getField(), protoreflect.BoolKind) {
					results = append(results, boolStrItem{k.Bool(), v.String()})
				}
				assert.That(t, results, should.Match([]boolStrItem{{false, "hey"}, {true, "norp"}}))
			})
		})

		t.Run(`int`, func(t *ftt.Test) {
			msg := &TestMapMessage{}
			refl := msg.ProtoReflect()
			get32Field := func() protoreflect.Map { return refl.Get(refl.Descriptor().Fields().ByName("int32_map")).Map() }
			get64Field := func() protoreflect.Map { return refl.Get(refl.Descriptor().Fields().ByName("int64_map")).Map() }

			t.Run(`int32`, func(t *ftt.Test) {
				msg.Int32Map = map[int32]string{}
				expect := []int64StrItem{}
				for i := range int64(100) {
					entry := int64StrItem{i, strconv.FormatInt(i, 10)}
					msg.Int32Map[int32(i)] = entry.v
					expect = append(expect, entry)
				}
				results := []int64StrItem{}
				for k, v := range MapRangeSorted(get32Field(), protoreflect.Int32Kind) {
					results = append(results, int64StrItem{k.Int(), v.String()})
				}
				assert.That(t, results, should.Match(expect))
			})

			t.Run(`int64`, func(t *ftt.Test) {
				msg.Int64Map = map[int64]string{}
				expect := []int64StrItem{}
				for i := range int64(100) {
					entry := int64StrItem{i, strconv.FormatInt(i, 10)}
					msg.Int64Map[i] = entry.v
					expect = append(expect, entry)
				}
				results := []int64StrItem{}
				for k, v := range MapRangeSorted(get64Field(), protoreflect.Int64Kind) {
					results = append(results, int64StrItem{k.Int(), v.String()})
				}
				assert.That(t, results, should.Match(expect))
			})
		})

		t.Run(`uint`, func(t *ftt.Test) {
			msg := &TestMapMessage{}
			refl := msg.ProtoReflect()
			get32Field := func() protoreflect.Map { return refl.Get(refl.Descriptor().Fields().ByName("uint32_map")).Map() }
			get64Field := func() protoreflect.Map { return refl.Get(refl.Descriptor().Fields().ByName("uint64_map")).Map() }

			t.Run(`uint32`, func(t *ftt.Test) {
				msg.Uint32Map = map[uint32]string{}
				expect := []uint64StrItem{}
				for i := range uint64(100) {
					entry := uint64StrItem{i, strconv.FormatUint(i, 10)}
					msg.Uint32Map[uint32(i)] = entry.v
					expect = append(expect, entry)
				}
				results := []uint64StrItem{}
				for k, v := range MapRangeSorted(get32Field(), protoreflect.Uint32Kind) {
					results = append(results, uint64StrItem{k.Uint(), v.String()})
				}
				assert.That(t, results, should.Match(expect))
			})

			t.Run(`uint64`, func(t *ftt.Test) {
				msg.Uint64Map = map[uint64]string{}
				expect := []uint64StrItem{}
				for i := range uint64(100) {
					entry := uint64StrItem{i, strconv.FormatUint(i, 10)}
					msg.Uint64Map[i] = entry.v
					expect = append(expect, entry)
				}
				results := []uint64StrItem{}
				for k, v := range MapRangeSorted(get64Field(), protoreflect.Uint64Kind) {
					results = append(results, uint64StrItem{k.Uint(), v.String()})
				}
				assert.That(t, results, should.Match(expect))
			})
		})

		t.Run(`string`, func(t *ftt.Test) {
			msg := &TestMapMessage{}
			refl := msg.ProtoReflect()
			getField := func() protoreflect.Map { return refl.Get(refl.Descriptor().Fields().ByName("string_map")).Map() }

			msg.StringMap = map[string]string{}
			expect := []string{}
			for i := range uint64(100) {
				entry := strconv.FormatUint(i, 10)
				msg.StringMap[entry] = entry
				expect = append(expect, entry)
			}
			// expect is sorted alphanumerically, not numerically
			sort.Strings(expect)

			results := []string{}
			for k, v := range MapRangeSorted(getField(), protoreflect.StringKind) {
				if k.String() != v.String() {
					panic("mismatched key/value")
				}
				results = append(results, v.String())
			}
			assert.That(t, results, should.Match(expect))
		})
	})
}
