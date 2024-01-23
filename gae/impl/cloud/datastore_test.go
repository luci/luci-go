// Copyright 2016 The LUCI Authors.
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

package cloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/datastore"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"

	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"

	. "github.com/smartystreets/goconvey/convey"
)

func mkProperties(index bool, forceMulti bool, vals ...any) ds.PropertyData {
	indexSetting := ds.ShouldIndex
	if !index {
		indexSetting = ds.NoIndex
	}

	if len(vals) == 1 && !forceMulti {
		var prop ds.Property
		prop.SetValue(vals[0], indexSetting)
		return prop
	}

	result := make(ds.PropertySlice, len(vals))
	for i, v := range vals {
		result[i].SetValue(v, indexSetting)
	}
	return result
}

func mkp(vals ...any) ds.PropertyData   { return mkProperties(true, false, vals...) }
func mkpNI(vals ...any) ds.PropertyData { return mkProperties(false, false, vals...) }

// shouldBeUntypedNil asserts that actual is nil, with a nil type pointer
// For example: https://play.golang.org/p/qN25iAYQw5Z
func shouldBeUntypedNil(actual any, _ ...any) string {
	if actual == nil {
		return ""
	}
	return fmt.Sprintf(`Expected: (%T, %v)
Actual:   (%T, %v)`, nil, nil, actual, actual)
}

func TestBoundDatastore(t *testing.T) {
	t.Parallel()

	Convey("boundDatastore", t, func() {
		kc := ds.KeyContext{
			AppID:     "app-id",
			Namespace: "ns",
		}

		Convey("*datastore.Entity", func() {
			ent := &datastore.Entity{
				Key: &datastore.Key{
					ID:        1,
					Kind:      "Kind",
					Namespace: "ns",
					Parent: &datastore.Key{
						Name:      "p",
						Kind:      "Parent",
						Namespace: "ns",
					},
				},
				Properties: []datastore.Property{
					{
						Name:  "bool",
						Value: true,
					},
					{
						Name: "entity",
						Value: &datastore.Entity{
							Properties: []datastore.Property{
								{
									Name:    "[]byte",
									NoIndex: true,
									Value:   []byte("byte"),
								},
								{
									Name:    "[]interface",
									NoIndex: true,
									Value:   []any{"interface"},
								},
								{
									Name:    "geopoint",
									NoIndex: true,
									Value: datastore.GeoPoint{
										Lat: 1,
										Lng: 1,
									},
								},
								{
									Name:  "indexed",
									Value: "hi",
								},
							},
						},
					},
					{
						Name:  "float64",
						Value: 1.0,
					},
					{
						Name:  "int64",
						Value: int64(1),
					},
					{
						Name: "key",
						Value: &datastore.Key{
							ID:        2,
							Kind:      "kind",
							Namespace: "ns",
						},
					},
					{
						Name:  "string",
						Value: "string",
					},
					{
						Name:  "time",
						Value: ds.RoundTime(testclock.TestRecentTimeUTC),
					},
					{
						Name:    "unindexed_entity",
						NoIndex: true,
						Value: &datastore.Entity{
							Properties: []datastore.Property{
								{
									Name:    "field",
									NoIndex: true,
									Value:   "ho",
								},
							},
						},
					},
				},
			}

			parent := kc.NewKey("Parent", "p", 0, nil)
			key := kc.NewKey("Kind", "", 1, parent)

			pm := ds.PropertyMap{
				"$key":    ds.MkPropertyNI(key),
				"$parent": ds.MkPropertyNI(parent),
				"$kind":   ds.MkPropertyNI("Kind"),
				"$id":     ds.MkPropertyNI(1),
				"bool":    ds.MkProperty(true),
				"entity": ds.MkProperty(ds.PropertyMap{
					"[]byte": ds.MkPropertyNI([]byte("byte")),
					"[]interface": ds.PropertySlice{
						ds.MkPropertyNI("interface"),
					},
					"geopoint": ds.MkPropertyNI(ds.GeoPoint{Lat: 1, Lng: 1}),
					"indexed":  ds.MkProperty("hi"),
				}),
				"unindexed_entity": ds.MkPropertyNI(ds.PropertyMap{
					"field": ds.MkPropertyNI("ho"),
				}),
				"float64": ds.MkProperty(1.0),
				"int64":   ds.MkProperty(int64(1)),
				"key":     ds.MkProperty(kc.NewKey("kind", "", 2, nil)),
				"string":  ds.MkProperty("string"),
				"time":    ds.MkProperty(ds.RoundTime(testclock.TestRecentTimeUTC)),
			}

			Convey("gaeEntityToNative", func() {
				So(gaeEntityToNative(kc, pm), ShouldResemble, ent)
			})

			Convey("nativeEntityToGAE", func() {
				So(nativeEntityToGAE(kc, ent), ShouldResemble, pm)
			})

			Convey("gaeEntityToNative, nativeEntityToGAE", func() {
				So(nativeEntityToGAE(kc, gaeEntityToNative(kc, pm)), ShouldResemble, pm)
			})

			Convey("nativeEntityToGAE, gaeEntityToNative", func() {
				So(gaeEntityToNative(kc, nativeEntityToGAE(kc, ent)), ShouldResemble, ent)
			})
		})
	})
}

// TestDatastore tests the cloud datastore implementation.
//
// Run the emulator:
// $ gcloud beta emulators datastore start --use-firestore-in-datastore-mode
//
// Export the DATASTORE_EMULATOR_HOST environment variable, which the above
// command printed.
//
// If the emulator environment is not detected, this test will be skipped.
func TestDatastore(t *testing.T) {
	t.Parallel()

	// See if an emulator is running. If no emulator is running, we will skip this
	// test suite.
	emulatorHost := os.Getenv("DATASTORE_EMULATOR_HOST")
	if emulatorHost == "" {
		t.Skip("No emulator detected (DATASTORE_EMULATOR_HOST). Skipping test suite.")
		return
	}

	Convey(fmt.Sprintf(`A cloud installation using datastore emulator %q`, emulatorHost), t, func() {
		c := context.Background()
		client, err := datastore.NewClient(c, "luci-gae-test")
		So(err, ShouldBeNil)
		defer client.Close()

		testTime := ds.RoundTime(time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC))
		_ = testTime

		cfg := ConfigLite{ProjectID: "luci-gae-test", DS: client}
		c = cfg.Use(c)

		Convey(`Supports namespaces`, func() {
			namespaces := []string{"foo", "bar", "baz"}

			// Clear all used entities from all namespaces.
			for _, ns := range namespaces {
				nsCtx := info.MustNamespace(c, ns)

				keys := make([]*ds.Key, len(namespaces))
				for i := range keys {
					keys[i] = ds.MakeKey(nsCtx, "Test", i+1)
				}
				So(errors.Filter(ds.Delete(nsCtx, keys), ds.ErrNoSuchEntity), ShouldBeNil)
			}

			// Put one entity per namespace.
			for i, ns := range namespaces {
				nsCtx := info.MustNamespace(c, ns)

				pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp(i + 1), "Value": mkp(i)}
				So(ds.Put(nsCtx, pmap), ShouldBeNil)
			}

			// Make sure that entity only exists in that namespace.
			for _, ns := range namespaces {
				nsCtx := info.MustNamespace(c, ns)

				for i := range namespaces {
					pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp(i + 1)}
					err := ds.Get(nsCtx, pmap)

					if namespaces[i] == ns {
						So(err, ShouldBeNil)
					} else {
						So(err, ShouldEqual, ds.ErrNoSuchEntity)
					}
				}
			}
		})

		Convey(`In a clean random testing namespace`, func() {
			// Enter a namespace for this round of tests.
			randNamespace := make([]byte, 32)
			if _, err := rand.Read(randNamespace); err != nil {
				panic(err)
			}
			c = info.MustNamespace(c, fmt.Sprintf("testing-%s", hex.EncodeToString(randNamespace)))

			// Execute a kindless query to clear the namespace.
			q := ds.NewQuery("").KeysOnly(true)
			var allKeys []*ds.Key
			So(ds.GetAll(c, q, &allKeys), ShouldBeNil)
			So(ds.Delete(c, allKeys), ShouldBeNil)

			Convey(`Can allocate an ID range`, func() {
				var keys []*ds.Key
				keys = append(keys, ds.NewIncompleteKeys(c, 10, "Bar", ds.MakeKey(c, "Foo", 12))...)
				keys = append(keys, ds.NewIncompleteKeys(c, 10, "Baz", ds.MakeKey(c, "Foo", 12))...)

				seen := map[string]struct{}{}
				So(ds.AllocateIDs(c, keys), ShouldBeNil)
				for _, k := range keys {
					So(k.IsIncomplete(), ShouldBeFalse)
					seen[k.String()] = struct{}{}
				}

				So(ds.AllocateIDs(c, keys), ShouldBeNil)
				for _, k := range keys {
					So(k.IsIncomplete(), ShouldBeFalse)

					_, ok := seen[k.String()]
					So(ok, ShouldBeFalse)
				}
			})

			Convey(`Can get, put, and delete entities`, func() {
				// Put: "foo", "bar", "baz".
				put := []ds.PropertyMap{
					{"$kind": mkp("test"), "$id": mkp("foo"), "Value": mkp(1337)},
					{"$kind": mkp("test"), "$id": mkp("bar"), "Value": mkp(42)},
					{"$kind": mkp("test"), "$id": mkp("baz"), "Value": mkp(0xd065)},
				}
				So(ds.Put(c, put), ShouldBeNil)

				// Delete: "bar".
				So(ds.Delete(c, ds.MakeKey(c, "test", "bar")), ShouldBeNil)

				// Get: "foo", "bar", "baz"
				get := []ds.PropertyMap{
					{"$kind": mkp("test"), "$id": mkp("foo")},
					{"$kind": mkp("test"), "$id": mkp("bar")},
					{"$kind": mkp("test"), "$id": mkp("baz")},
				}

				err := ds.Get(c, get)
				So(err, ShouldHaveSameTypeAs, errors.MultiError(nil))

				merr := err.(errors.MultiError)
				So(len(merr), ShouldEqual, 3)
				So(merr[0], ShouldBeNil)
				So(merr[1], ShouldEqual, ds.ErrNoSuchEntity)
				So(merr[2], ShouldBeNil)

				// put[1] will not be retrieved (delete)
				put[1] = get[1]
				So(get, ShouldResemble, put)
			})

			Convey(`Can put and get all supported entity fields.`, func() {
				put := ds.PropertyMap{
					"$id":   mkpNI("foo"),
					"$kind": mkpNI("FooType"),

					"Number":    mkp(1337),
					"String":    mkpNI("hello"),
					"Bytes":     mkp([]byte("world")),
					"Time":      mkp(testTime),
					"Float":     mkpNI(3.14),
					"Key":       mkp(ds.MakeKey(c, "Parent", "ParentID", "Child", 1337)),
					"Null":      mkp(nil),
					"NullSlice": mkp(nil, nil),

					"ComplexSlice": mkp(1337, "string", []byte("bytes"), testTime, float32(3.14),
						float64(2.71), true, nil, ds.MakeKey(c, "SomeKey", "SomeID")),

					"Single":      mkp("single"),
					"SingleSlice": mkProperties(true, true, "single"), // Force a single "multi" value.
					"EmptySlice":  ds.PropertySlice(nil),

					"SingleEntity": mkp(ds.PropertyMap{
						"$id":     mkpNI("inner"),
						"$kind":   mkpNI("Inner"),
						"$key":    mkpNI(ds.MakeKey(c, "Inner", "inner")),
						"$parent": mkpNI(nil),
						"prop":    mkp(1),
						"deeper":  mkp(ds.PropertyMap{"deep": mkp(123)}),
					}),
					"SliceOfEntities": mkp(
						ds.PropertyMap{"prop": mkp(2)},
						ds.PropertyMap{"prop": mkp(3)},
					),
				}
				So(ds.Put(c, put), ShouldBeNil)

				get := ds.PropertyMap{
					"$id":   mkpNI("foo"),
					"$kind": mkpNI("FooType"),
				}
				So(ds.Get(c, get), ShouldBeNil)
				So(get, ShouldResemble, put)
			})

			Convey(`Can Get empty []byte slice as nil`, func() {
				put := ds.PropertyMap{
					"$id":   mkpNI("foo"),
					"$kind": mkpNI("FooType"),
					"Empty": mkp([]byte(nil)),
					"Nilly": mkp([]byte{}),
				}
				get := ds.PropertyMap{
					"$id":   put["$id"],
					"$kind": put["$kind"],
				}
				exp := put.Clone()
				exp["Nilly"] = mkp([]byte(nil))

				So(ds.Put(c, put), ShouldBeNil)
				So(ds.Get(c, get), ShouldBeNil)
				So(get, ShouldResemble, exp)
			})

			Convey(`With several entities installed`, func() {
				So(ds.Put(c, []ds.PropertyMap{
					{"$kind": mkp("Test"), "$id": mkp("foo"), "FooBar": mkp(true)},
					{"$kind": mkp("Test"), "$id": mkp("bar"), "FooBar": mkp(true)},
					{"$kind": mkp("Test"), "$id": mkp("baz")},
					{"$kind": mkp("Test"), "$id": mkp("qux")},
					{"$kind": mkp("Test"), "$id": mkp("quux"), "$parent": mkp(ds.MakeKey(c, "Test", "baz"))},
					{"$kind": mkp("Test"), "$id": mkp("quuz"), "$parent": mkp(ds.MakeKey(c, "Test", "baz"))},
				}), ShouldBeNil)

				withAllMeta := func(pm ds.PropertyMap) ds.PropertyMap {
					prop := pm["$key"].(ds.Property)
					key := prop.Value().(*ds.Key)
					pm["$id"] = mkpNI(key.StringID())
					pm["$kind"] = mkpNI(key.Kind())
					pm["$parent"] = mkpNI(key.Parent())
					return pm
				}

				q := ds.NewQuery("Test")

				Convey(`Can query for entities with FooBar == true.`, func() {
					var results []ds.PropertyMap
					q = q.Eq("FooBar", true)
					So(ds.GetAll(c, q, &results), ShouldBeNil)

					So(results, ShouldResemble, []ds.PropertyMap{
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "bar")), "FooBar": mkp(true)}),
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "foo")), "FooBar": mkp(true)}),
					})
				})

				Convey(`Can query for entities whose __key__ > "baz".`, func() {
					var results []ds.PropertyMap
					q = q.Gt("__key__", ds.MakeKey(c, "Test", "baz"))
					So(ds.GetAll(c, q, &results), ShouldBeNil)

					So(results, ShouldResemble, []ds.PropertyMap{
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "baz", "Test", "quux"))}),
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "baz", "Test", "quuz"))}),
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "foo")), "FooBar": mkp(true)}),
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "qux"))}),
					})
				})

				Convey(`Can query for entities whose ancestor is "baz".`, func() {
					var results []ds.PropertyMap
					q := ds.NewQuery("Test").Ancestor(ds.MakeKey(c, "Test", "baz"))
					So(ds.GetAll(c, q, &results), ShouldBeNil)

					So(results, ShouldResemble, []ds.PropertyMap{
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "baz"))}),
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "baz", "Test", "quux"))}),
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "baz", "Test", "quuz"))}),
					})
				})

				Convey(`Can transactionally get and put.`, func() {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("qux")}
						if err := ds.Get(c, pmap); err != nil {
							return err
						}

						pmap["ExtraField"] = mkp("Present!")
						return ds.Put(c, pmap)
					}, nil)
					So(err, ShouldBeNil)

					pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("qux")}
					err = ds.RunInTransaction(c, func(c context.Context) error {
						return ds.Get(c, pmap)
					}, nil)
					So(err, ShouldBeNil)
					So(pmap, ShouldResemble, ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("qux"), "ExtraField": mkp("Present!")})
				})

				Convey(`Can fail in a transaction with no effect.`, func() {
					testError := errors.New("test error")

					noTxnPM := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("no txn")}
					err := ds.RunInTransaction(c, func(c context.Context) error {
						So(ds.CurrentTransaction(c), ShouldNotBeNil)

						pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("quux")}
						if err := ds.Put(c, pmap); err != nil {
							return err
						}

						// Put an entity outside of the transaction so we can confirm that
						// it was added even when the transaction fails.
						if err := ds.Put(ds.WithoutTransaction(c), noTxnPM); err != nil {
							return err
						}
						return testError
					}, nil)
					So(err, ShouldEqual, testError)

					// Confirm that noTxnPM was added.
					So(ds.CurrentTransaction(c), shouldBeUntypedNil)
					So(ds.Get(c, noTxnPM), ShouldBeNil)

					pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("quux")}
					err = ds.RunInTransaction(c, func(c context.Context) error {
						return ds.Get(c, pmap)
					}, nil)
					So(err, ShouldEqual, ds.ErrNoSuchEntity)
				})
			})
		})
	})
}
