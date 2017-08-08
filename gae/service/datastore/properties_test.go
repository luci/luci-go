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

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/gae/service/blobstore"
)

type myint int
type mybool bool
type mystring string
type myfloat float32

func TestProperties(t *testing.T) {
	t.Parallel()

	Convey("Test Property", t, func() {
		Convey("Construction", func() {
			Convey("empty", func() {
				pv := Property{}
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTNull")
			})
			Convey("set", func() {
				pv := MkPropertyNI(100)
				So(pv.Value(), ShouldHaveSameTypeAs, int64(100))
				So(pv.Value(), ShouldEqual, 100)
				So(pv.IndexSetting(), ShouldEqual, NoIndex)
				So(pv.Type().String(), ShouldEqual, "PTInt")

				So(pv.SetValue(nil, ShouldIndex), ShouldBeNil)
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTNull")
			})
			Convey("derived types", func() {
				Convey("int", func() {
					pv := MkProperty(19)
					So(pv.Value(), ShouldHaveSameTypeAs, int64(19))
					So(pv.Value(), ShouldEqual, 19)
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTInt")
				})
				Convey("int32", func() {
					pv := MkProperty(int32(32))
					So(pv.Value(), ShouldHaveSameTypeAs, int64(32))
					So(pv.Value(), ShouldEqual, 32)
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTInt")
				})
				Convey("uint32", func() {
					pv := MkProperty(uint32(32))
					So(pv.Value(), ShouldHaveSameTypeAs, int64(32))
					So(pv.Value(), ShouldEqual, 32)
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTInt")
				})
				Convey("byte", func() {
					pv := MkProperty(byte(32))
					So(pv.Value(), ShouldHaveSameTypeAs, int64(32))
					So(pv.Value(), ShouldEqual, 32)
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTInt")
				})
				Convey("bool (true)", func() {
					pv := MkProperty(mybool(true))
					So(pv.Value(), ShouldBeTrue)
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTBool")
				})
				Convey("string", func() {
					pv := MkProperty(mystring("sup"))
					So(pv.Value(), ShouldEqual, "sup")
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTString")
				})
				Convey("blobstore.Key is distinquished", func() {
					pv := MkProperty(blobstore.Key("sup"))
					So(pv.Value(), ShouldEqual, blobstore.Key("sup"))
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTBlobKey")
				})
				Convey("datastore Key is distinguished", func() {
					k := MkKeyContext("appid", "ns").MakeKey("kind", "1")
					pv := MkProperty(k)
					So(pv.Value(), ShouldHaveSameTypeAs, k)
					So(pv.Value().(*Key).Equal(k), ShouldBeTrue)
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTKey")

					pv = MkProperty((*Key)(nil))
					So(pv.Value(), ShouldBeNil)
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTNull")
				})
				Convey("float", func() {
					pv := Property{}
					So(pv.SetValue(myfloat(19.7), ShouldIndex), ShouldBeNil)
					So(pv.Value(), ShouldHaveSameTypeAs, float64(19.7))
					So(pv.Value(), ShouldEqual, float32(19.7))
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTFloat")
				})
			})
			Convey("bad type", func() {
				pv := Property{}
				err := pv.SetValue(complex(100, 29), ShouldIndex)
				So(err.Error(), ShouldContainSubstring, "has bad type complex")
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTNull")
			})
			Convey("invalid GeoPoint", func() {
				pv := Property{}
				err := pv.SetValue(GeoPoint{-1000, 0}, ShouldIndex)
				So(err.Error(), ShouldContainSubstring, "invalid GeoPoint value")
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTNull")
			})
			Convey("invalid time", func() {
				pv := Property{}
				loc, err := time.LoadLocation("America/Los_Angeles")
				So(err, ShouldBeNil)
				t := time.Date(1970, 1, 1, 0, 0, 0, 0, loc)

				err = pv.SetValue(t, ShouldIndex)
				So(err.Error(), ShouldContainSubstring, "time value has wrong Location")

				err = pv.SetValue(time.Unix(math.MaxInt64, 0).UTC(), ShouldIndex)
				So(err.Error(), ShouldContainSubstring, "time value out of range")
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTNull")
			})
			Convey("time gets rounded", func() {
				pv := Property{}
				now := time.Now().In(time.UTC)
				now = now.Round(time.Microsecond).Add(time.Nanosecond * 313)
				So(pv.SetValue(now, ShouldIndex), ShouldBeNil)
				So(pv.Value(), ShouldHappenBefore, now)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTTime")
			})
			Convey("zero time", func() {
				now := time.Time{}
				So(now.IsZero(), ShouldBeTrue)

				pv := Property{}
				So(pv.SetValue(now, ShouldIndex), ShouldBeNil)
				So(pv.Value(), ShouldResemble, now)
				v, err := pv.Project(PTInt)
				So(err, ShouldBeNil)
				So(v, ShouldEqual, 0)

				So(pv.SetValue(0, ShouldIndex), ShouldBeNil)
				So(pv.Value(), ShouldEqual, 0)
				v, err = pv.Project(PTTime)
				So(err, ShouldBeNil)
				So(v.(time.Time).IsZero(), ShouldBeTrue)
			})
			Convey("[]byte allows IndexSetting", func() {
				pv := Property{}
				So(pv.SetValue([]byte("hello"), ShouldIndex), ShouldBeNil)
				So(pv.Value(), ShouldResemble, []byte("hello"))
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTBytes")
			})
		})

		Convey("Comparison", func() {
			Convey(`A []byte property should equal a string property with the same value.`, func() {
				a := MkProperty([]byte("ohaithere"))
				b := MkProperty("ohaithere")
				So(a.Equal(&b), ShouldBeTrue)
			})
		})
	})
}

func TestDSPropertyMapImpl(t *testing.T) {
	t.Parallel()

	Convey("PropertyMap load/save err conditions", t, func() {
		Convey("empty", func() {
			pm := PropertyMap{}
			err := pm.Load(PropertyMap{"hello": Property{}})
			So(err, ShouldBeNil)
			So(pm, ShouldResemble, PropertyMap{"hello": Property{}})

			npm, _ := pm.Save(false)
			So(npm, ShouldResemble, pm)
		})
		Convey("meta", func() {
			Convey("working", func() {
				pm := PropertyMap{"": MkProperty("trap!")}
				_, ok := pm.GetMeta("foo")
				So(ok, ShouldBeFalse)

				So(pm.SetMeta("foo", 100), ShouldBeTrue)

				v, ok := pm.GetMeta("foo")
				So(ok, ShouldBeTrue)
				So(v, ShouldEqual, 100)

				So(GetMetaDefault(pm, "foo", 100), ShouldEqual, 100)

				So(GetMetaDefault(pm, "bar", 100), ShouldEqual, 100)

				npm, err := pm.Save(false)
				So(err, ShouldBeNil)
				So(len(npm), ShouldEqual, 0)
			})

			Convey("too many values picks the first one", func() {
				pm := PropertyMap{
					"$thing": PropertySlice{MkProperty(100), MkProperty(200)},
				}
				v, ok := pm.GetMeta("thing")
				So(ok, ShouldBeTrue)
				So(v, ShouldEqual, 100)
			})

			Convey("errors", func() {

				Convey("weird value", func() {
					pm := PropertyMap{}
					So(pm.SetMeta("sup", complex(100, 20)), ShouldBeFalse)
				})
			})
		})
	})
}

func TestByteSequences(t *testing.T) {
	t.Parallel()

	conversions := []struct {
		desc string
		conv func(v string) (byteSequence, interface{})
	}{
		{"string", func(v string) (byteSequence, interface{}) { return stringByteSequence(v), v }},
		{"[]byte", func(v string) (byteSequence, interface{}) { return bytesByteSequence(v), []byte(v) }},
	}

	testCases := map[string][]struct {
		assertion func(interface{}, ...interface{}) string
		cmpS      string
	}{
		"": {
			{ShouldEqual, ""},
			{ShouldBeLessThan, "foo"},
		},
		"bar": {
			{ShouldEqual, "bar"},
			{ShouldBeGreaterThan, "ba"},
		},
		"ba": {
			{ShouldBeLessThan, "bar"},
			{ShouldBeLessThan, "z"},
		},
		"foo": {
			{ShouldBeGreaterThan, ""},
		},
		"bz": {
			{ShouldBeGreaterThan, "bar"},
		},
		"qux": {
			{ShouldBeGreaterThan, "bar"},
		},
	}

	keys := make([]string, 0, len(testCases))
	for k := range testCases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	Convey(`When testing byte sequences`, t, func() {
		for _, s := range keys {
			for _, c := range conversions {
				Convey(fmt.Sprintf(`A %s sequence with test data %q`, c.desc, s), func() {
					bs, effectiveValue := c.conv(s)

					Convey(`Basic stuff works.`, func() {
						So(bs.len(), ShouldEqual, len(s))
						for i, c := range s {
							So(bs.get(i), ShouldEqual, c)
						}
						So(bs.value(), ShouldResemble, effectiveValue)
						So(bs.string(), ShouldEqual, s)
						So(bs.bytes(), ShouldResemble, []byte(s))
					})

					// Test comparison with other byteSequence types.
					for _, tc := range testCases[s] {
						for _, c := range conversions {
							Convey(fmt.Sprintf(`Compares properly with %s %q`, c.desc, tc.cmpS), func() {
								cmpBS, _ := c.conv(tc.cmpS)
								So(cmpByteSequence(bs, cmpBS), tc.assertion, 0)
							})
						}
					}
				})
			}
		}
	})
}
