// Copyright 2015 The LUCI Authors.
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

package datastore

import (
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/blobstore"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(Property{}))
}

type mybool bool
type mystring string
type myfloat float32

func TestProperties(t *testing.T) {
	t.Parallel()

	ftt.Run("Test Property", t, func(t *ftt.Test) {
		t.Run("Construction", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				pv := Property{}
				assert.Loosely(t, pv.Value(), should.BeNil)
				assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
				assert.Loosely(t, pv.Type().String(), should.Equal("PTNull"))
			})
			t.Run("set", func(t *ftt.Test) {
				pv := MkPropertyNI(100)
				assert.Loosely(t, pv.Value(), should.HaveType[int64])
				assert.Loosely(t, pv.Value(), should.Equal(100))
				assert.Loosely(t, pv.IndexSetting(), should.Equal(NoIndex))
				assert.Loosely(t, pv.Type().String(), should.Equal("PTInt"))

				assert.Loosely(t, pv.SetValue(nil, ShouldIndex), should.BeNil)
				assert.Loosely(t, pv.Value(), should.BeNil)
				assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
				assert.Loosely(t, pv.Type().String(), should.Equal("PTNull"))
			})
			t.Run("derived types", func(t *ftt.Test) {
				t.Run("int", func(t *ftt.Test) {
					pv := MkProperty(19)
					assert.Loosely(t, pv.Value(), should.HaveType[int64])
					assert.Loosely(t, pv.Value(), should.Equal(19))
					assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
					assert.Loosely(t, pv.Type().String(), should.Equal("PTInt"))
				})
				t.Run("int32", func(t *ftt.Test) {
					pv := MkProperty(int32(32))
					assert.Loosely(t, pv.Value(), should.HaveType[int64])
					assert.Loosely(t, pv.Value(), should.Equal(32))
					assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
					assert.Loosely(t, pv.Type().String(), should.Equal("PTInt"))
				})
				t.Run("uint32", func(t *ftt.Test) {
					pv := MkProperty(uint32(32))
					assert.Loosely(t, pv.Value(), should.HaveType[int64])
					assert.Loosely(t, pv.Value(), should.Equal(32))
					assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
					assert.Loosely(t, pv.Type().String(), should.Equal("PTInt"))
				})
				t.Run("byte", func(t *ftt.Test) {
					pv := MkProperty(byte(32))
					assert.Loosely(t, pv.Value(), should.HaveType[int64])
					assert.Loosely(t, pv.Value(), should.Equal(32))
					assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
					assert.Loosely(t, pv.Type().String(), should.Equal("PTInt"))
				})
				t.Run("bool (true)", func(t *ftt.Test) {
					pv := MkProperty(mybool(true))
					assert.Loosely(t, pv.Value(), should.BeTrue)
					assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
					assert.Loosely(t, pv.Type().String(), should.Equal("PTBool"))
				})
				t.Run("string", func(t *ftt.Test) {
					pv := MkProperty(mystring("sup"))
					assert.Loosely(t, pv.Value(), should.Equal("sup"))
					assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
					assert.Loosely(t, pv.Type().String(), should.Equal("PTString"))
				})
				t.Run("blobstore.Key is distinquished", func(t *ftt.Test) {
					pv := MkProperty(blobstore.Key("sup"))
					assert.Loosely(t, pv.Value(), should.Equal(blobstore.Key("sup")))
					assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
					assert.Loosely(t, pv.Type().String(), should.Equal("PTBlobKey"))
				})
				t.Run("datastore Key is distinguished", func(t *ftt.Test) {
					k := MkKeyContext("appid", "ns").MakeKey("kind", "1")
					pv := MkProperty(k)
					assert.Loosely(t, pv.Value(), should.HaveType[*Key])
					assert.Loosely(t, pv.Value().(*Key).Equal(k), should.BeTrue)
					assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
					assert.Loosely(t, pv.Type().String(), should.Equal("PTKey"))

					pv = MkProperty((*Key)(nil))
					assert.Loosely(t, pv.Value(), should.BeNil)
					assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
					assert.Loosely(t, pv.Type().String(), should.Equal("PTNull"))
				})
				t.Run("float", func(t *ftt.Test) {
					pv := Property{}
					assert.Loosely(t, pv.SetValue(myfloat(19.7), ShouldIndex), should.BeNil)
					assert.Loosely(t, pv.Value(), should.HaveType[float64])
					assert.Loosely(t, pv.Value(), should.AlmostEqual(19.7, 0.000001))
					assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
					assert.Loosely(t, pv.Type().String(), should.Equal("PTFloat"))
				})
				t.Run("property map", func(t *ftt.Test) {
					pv := Property{}
					assert.Loosely(t, pv.SetValue(PropertyMap{
						"key": Property{
							indexSetting: true,
							propType:     PTString,
							value:        stringByteSequence("val"),
						},
						"nested": Property{
							indexSetting: false,
							propType:     PTPropertyMap,
							value: PropertyMap{
								"key": Property{
									indexSetting: false,
									propType:     PTInt,
									value:        1,
								},
							},
						},
					}, NoIndex), should.BeNil)
					assert.Loosely(t, pv.Value(), should.Resemble(PropertyMap{
						"key": Property{
							indexSetting: true,
							propType:     PTString,
							value:        stringByteSequence("val"),
						},
						"nested": Property{
							indexSetting: false,
							propType:     PTPropertyMap,
							value: PropertyMap{
								"key": Property{
									indexSetting: false,
									propType:     PTInt,
									value:        1,
								},
							},
						},
					}))
					assert.Loosely(t, pv.IndexSetting(), should.Equal(NoIndex))
					assert.Loosely(t, pv.Type().String(), should.Equal("PTPropertyMap"))
				})
			})
			t.Run("bad type", func(t *ftt.Test) {
				pv := Property{}
				err := pv.SetValue(complex(100, 29), ShouldIndex)
				assert.Loosely(t, err.Error(), should.ContainSubstring("has bad type complex"))
				assert.Loosely(t, pv.Value(), should.BeNil)
				assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
				assert.Loosely(t, pv.Type().String(), should.Equal("PTNull"))
			})
			t.Run("invalid GeoPoint", func(t *ftt.Test) {
				pv := Property{}
				err := pv.SetValue(GeoPoint{-1000, 0}, ShouldIndex)
				assert.Loosely(t, err.Error(), should.ContainSubstring("invalid GeoPoint value"))
				assert.Loosely(t, pv.Value(), should.BeNil)
				assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
				assert.Loosely(t, pv.Type().String(), should.Equal("PTNull"))
			})
			t.Run("invalid time", func(t *ftt.Test) {
				pv := Property{}
				loc, err := time.LoadLocation("America/Los_Angeles")
				assert.Loosely(t, err, should.BeNil)
				ts := time.Date(1970, 1, 1, 0, 0, 0, 0, loc)

				err = pv.SetValue(ts, ShouldIndex)
				assert.Loosely(t, err.Error(), should.ContainSubstring("time value has wrong Location"))

				err = pv.SetValue(time.Unix(math.MaxInt64, 0).UTC(), ShouldIndex)
				assert.Loosely(t, err.Error(), should.ContainSubstring("time value out of range"))
				assert.Loosely(t, pv.Value(), should.BeNil)
				assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
				assert.Loosely(t, pv.Type().String(), should.Equal("PTNull"))
			})
			t.Run("time gets rounded", func(t *ftt.Test) {
				pv := Property{}
				now := time.Now().In(time.UTC)
				now = now.Round(time.Microsecond).Add(time.Nanosecond * 313)
				assert.Loosely(t, pv.SetValue(now, ShouldIndex), should.BeNil)
				assert.Loosely(t, pv.Value(), should.HappenBefore(now))
				assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
				assert.Loosely(t, pv.Type().String(), should.Equal("PTTime"))
			})
			t.Run("zero time", func(t *ftt.Test) {
				now := time.Time{}
				assert.Loosely(t, now.IsZero(), should.BeTrue)

				pv := Property{}
				assert.Loosely(t, pv.SetValue(now, ShouldIndex), should.BeNil)
				assert.Loosely(t, pv.Value(), should.Resemble(now))
				v, err := pv.Project(PTInt)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, v, should.BeZero)

				assert.Loosely(t, pv.SetValue(0, ShouldIndex), should.BeNil)
				assert.Loosely(t, pv.Value(), should.BeZero)
				v, err = pv.Project(PTTime)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, v.(time.Time).IsZero(), should.BeTrue)
			})
			t.Run("[]byte allows IndexSetting", func(t *ftt.Test) {
				pv := Property{}
				assert.Loosely(t, pv.SetValue([]byte("hello"), ShouldIndex), should.BeNil)
				assert.Loosely(t, pv.Value(), should.Resemble([]byte("hello")))
				assert.Loosely(t, pv.IndexSetting(), should.Equal(ShouldIndex))
				assert.Loosely(t, pv.Type().String(), should.Equal("PTBytes"))
			})
		})

		t.Run("Comparison", func(t *ftt.Test) {
			t.Run(`A []byte property should equal a string property with the same value.`, func(t *ftt.Test) {
				a := MkProperty([]byte("ohaithere"))
				b := MkProperty("ohaithere")
				assert.Loosely(t, a.Equal(&b), should.BeTrue)
			})
		})
	})
}

func TestDSPropertyMapImpl(t *testing.T) {
	t.Parallel()

	ftt.Run("Test PropertyMap", t, func(t *ftt.Test) {
		t.Run("load/save err conditions", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				pm := PropertyMap{}
				err := pm.Load(PropertyMap{"hello": Property{}})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pm, should.Resemble(PropertyMap{"hello": Property{}}))

				npm, _ := pm.Save(false)
				assert.Loosely(t, npm, should.Resemble(pm))
			})
			t.Run("meta", func(t *ftt.Test) {
				t.Run("working", func(t *ftt.Test) {
					pm := PropertyMap{"": MkProperty("trap!")}
					_, ok := pm.GetMeta("foo")
					assert.Loosely(t, ok, should.BeFalse)

					assert.Loosely(t, pm.SetMeta("foo", 100), should.BeTrue)

					v, ok := pm.GetMeta("foo")
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, v, should.Equal(100))

					assert.Loosely(t, GetMetaDefault(pm, "foo", 100), should.Equal(100))

					assert.Loosely(t, GetMetaDefault(pm, "bar", 100), should.Equal(100))

					npm, err := pm.Save(false)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(npm), should.BeZero)
				})

				t.Run("too many values picks the first one", func(t *ftt.Test) {
					pm := PropertyMap{
						"$thing": PropertySlice{MkProperty(100), MkProperty(200)},
					}
					v, ok := pm.GetMeta("thing")
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, v, should.Equal(100))
				})

				t.Run("errors", func(t *ftt.Test) {

					t.Run("weird value", func(t *ftt.Test) {
						pm := PropertyMap{}
						assert.Loosely(t, pm.SetMeta("sup", complex(100, 20)), should.BeFalse)
					})
				})
			})
		})
		t.Run("disable indexing on entire map", func(t *ftt.Test) {
			pm := PropertyMap{
				"single": MkProperty("foo"),
				"slice":  PropertySlice{MkProperty(100), MkProperty(200)},
			}
			pm.TurnOffIdx()
			assert.Loosely(t, pm["single"].Slice()[0].indexSetting, should.Equal(NoIndex))
			assert.Loosely(t, pm["slice"].Slice()[0].indexSetting, should.Equal(NoIndex))
			assert.Loosely(t, pm["slice"].Slice()[1].indexSetting, should.Equal(NoIndex))
		})
	})
}

func TestByteSequences(t *testing.T) {
	t.Parallel()

	conversions := []struct {
		desc string
		conv func(v string) (byteSequence, any)
	}{
		{"string", func(v string) (byteSequence, any) { return stringByteSequence(v), v }},
		{"[]byte", func(v string) (byteSequence, any) { return bytesByteSequence(v), []byte(v) }},
	}

	testCases := map[string][]struct {
		assertion func(expect int) comparison.Func[int]
		cmpS      string
	}{
		"": {
			{should.Equal[int], ""},
			{should.BeLessThan[int], "foo"},
		},
		"bar": {
			{should.Equal[int], "bar"},
			{should.BeGreaterThan[int], "ba"},
		},
		"ba": {
			{should.BeLessThan[int], "bar"},
			{should.BeLessThan[int], "z"},
		},
		"foo": {
			{should.BeGreaterThan[int], ""},
		},
		"bz": {
			{should.BeGreaterThan[int], "bar"},
		},
		"qux": {
			{should.BeGreaterThan[int], "bar"},
		},
	}

	keys := make([]string, 0, len(testCases))
	for k := range testCases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ftt.Run(`When testing byte sequences`, t, func(t *ftt.Test) {
		for _, s := range keys {
			for _, c := range conversions {
				t.Run(fmt.Sprintf(`A %s sequence with test data %q`, c.desc, s), func(t *ftt.Test) {
					bs, effectiveValue := c.conv(s)

					t.Run(`Basic stuff works.`, func(t *ftt.Test) {
						assert.Loosely(t, bs.len(), should.Equal(len(s)))
						for i, c := range s {
							assert.Loosely(t, bs.get(i), should.Equal(c))
						}
						assert.Loosely(t, bs.value(), should.Resemble(effectiveValue))
						assert.Loosely(t, bs.string(), should.Equal(s))
						assert.Loosely(t, bs.bytes(), should.Resemble([]byte(s)))
					})

					// Test comparison with other byteSequence types.
					for _, tc := range testCases[s] {
						for _, c := range conversions {
							t.Run(fmt.Sprintf(`Compares properly with %s %q`, c.desc, tc.cmpS), func(t *ftt.Test) {
								cmpBS, _ := c.conv(tc.cmpS)
								assert.Loosely(t, cmpByteSequence(bs, cmpBS), tc.assertion(0))
							})
						}
					}
				})
			}
		}
	})
}
