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
	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"

	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(ds.Property{}))
}

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

	ftt.Run("boundDatastore", t, func(t *ftt.Test) {
		kc := ds.KeyContext{
			AppID:     "app-id",
			Namespace: "ns",
		}

		t.Run("*datastore.Entity", func(t *ftt.Test) {
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

			t.Run("gaeEntityToNative", func(t *ftt.Test) {
				assert.Loosely(t, gaeEntityToNative(kc, pm), should.Match(ent))
			})

			t.Run("nativeEntityToGAE", func(t *ftt.Test) {
				assert.Loosely(t, nativeEntityToGAE(kc, ent), should.Match(pm))
			})

			t.Run("gaeEntityToNative, nativeEntityToGAE", func(t *ftt.Test) {
				assert.Loosely(t, nativeEntityToGAE(kc, gaeEntityToNative(kc, pm)), should.Match(pm))
			})

			t.Run("nativeEntityToGAE, gaeEntityToNative", func(t *ftt.Test) {
				assert.Loosely(t, gaeEntityToNative(kc, nativeEntityToGAE(kc, ent)), should.Match(ent))
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

	ftt.Run(fmt.Sprintf(`A cloud installation using datastore emulator %q`, emulatorHost), t, func(t *ftt.Test) {
		c := context.Background()
		client, err := datastore.NewClient(c, "luci-gae-test")
		assert.Loosely(t, err, should.BeNil)
		defer client.Close()

		testTime := ds.RoundTime(time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC))
		_ = testTime

		cfg := ConfigLite{ProjectID: "luci-gae-test", DS: client}
		c = cfg.Use(c)

		t.Run(`Supports namespaces`, func(t *ftt.Test) {
			namespaces := []string{"foo", "bar", "baz"}

			// Clear all used entities from all namespaces.
			for _, ns := range namespaces {
				nsCtx := info.MustNamespace(c, ns)

				keys := make([]*ds.Key, len(namespaces))
				for i := range keys {
					keys[i] = ds.MakeKey(nsCtx, "Test", i+1)
				}
				assert.Loosely(t, errors.Filter(ds.Delete(nsCtx, keys), ds.ErrNoSuchEntity), should.BeNil)
			}

			// Put one entity per namespace.
			for i, ns := range namespaces {
				nsCtx := info.MustNamespace(c, ns)

				pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp(i + 1), "Value": mkp(i)}
				assert.Loosely(t, ds.Put(nsCtx, pmap), should.BeNil)
			}

			// Make sure that entity only exists in that namespace.
			for _, ns := range namespaces {
				nsCtx := info.MustNamespace(c, ns)

				for i := range namespaces {
					pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp(i + 1)}
					err := ds.Get(nsCtx, pmap)

					if namespaces[i] == ns {
						assert.Loosely(t, err, should.BeNil)
					} else {
						assert.Loosely(t, err, should.Equal(ds.ErrNoSuchEntity))
					}
				}
			}
		})

		t.Run(`In a clean random testing namespace`, func(t *ftt.Test) {
			// Enter a namespace for this round of tests.
			randNamespace := make([]byte, 32)
			if _, err := rand.Read(randNamespace); err != nil {
				panic(err)
			}
			c = info.MustNamespace(c, fmt.Sprintf("testing-%s", hex.EncodeToString(randNamespace)))

			// Execute a kindless query to clear the namespace.
			q := ds.NewQuery("").KeysOnly(true)
			var allKeys []*ds.Key
			assert.Loosely(t, ds.GetAll(c, q, &allKeys), should.BeNil)
			assert.Loosely(t, ds.Delete(c, allKeys), should.BeNil)

			t.Run(`Can allocate an ID range`, func(t *ftt.Test) {
				var keys []*ds.Key
				keys = append(keys, ds.NewIncompleteKeys(c, 10, "Bar", ds.MakeKey(c, "Foo", 12))...)
				keys = append(keys, ds.NewIncompleteKeys(c, 10, "Baz", ds.MakeKey(c, "Foo", 12))...)

				seen := map[string]struct{}{}
				assert.Loosely(t, ds.AllocateIDs(c, keys), should.BeNil)
				for _, k := range keys {
					assert.Loosely(t, k.IsIncomplete(), should.BeFalse)
					seen[k.String()] = struct{}{}
				}

				assert.Loosely(t, ds.AllocateIDs(c, keys), should.BeNil)
				for _, k := range keys {
					assert.Loosely(t, k.IsIncomplete(), should.BeFalse)

					_, ok := seen[k.String()]
					assert.Loosely(t, ok, should.BeFalse)
				}
			})

			t.Run(`Can allocate IDs in Put`, func(t *ftt.Test) {
				type Ent struct {
					ID  int64 `gae:"$id"`
					Val int
				}

				// A mix of entities with and without IDs.
				ents := []*Ent{
					{Val: 100},
					{Val: 101},
					{Val: 102, ID: 1111},
					{Val: 103},
					{Val: 104, ID: 2222},
					{Val: 105},
				}

				getVal := func(c context.Context, id int64) int {
					ent := &Ent{ID: id}
					if err := ds.Get(c, ent); err != nil {
						return 0
					}
					return ent.Val
				}

				t.Run(`Outside of txn`, func(t *ftt.Test) {
					assert.NoErr(t, ds.Put(c, ents))
					for _, ent := range ents {
						assert.That(t, ent.ID, should.NotEqual(int64(0)))
						assert.That(t, getVal(c, ent.ID), should.Equal(ent.Val))
					}
				})

				t.Run(`In txn by default`, func(t *ftt.Test) {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						assert.NoErr(t, ds.Put(c, ents))
						for _, ent := range ents {
							assert.That(t, ent.ID, should.NotEqual(int64(0)))
						}
						return nil
					}, nil)
					assert.NoErr(t, err)
					for _, ent := range ents {
						assert.That(t, getVal(c, ent.ID), should.Equal(ent.Val))
					}
				})

				t.Run(`Delayed when AllocateIDsOnCommit`, func(t *ftt.Test) {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						assert.NoErr(t, ds.Put(c, ents))
						var ids []int64
						for _, ent := range ents {
							ids = append(ids, ent.ID)
						}
						assert.That(t, ids, should.Match([]int64{
							0, 0, 1111, 0, 2222, 0,
						}))
						return nil
					}, &ds.TransactionOptions{AllocateIDsOnCommit: true})
					assert.NoErr(t, err)

					// Actually stored them all with some unknown IDs.
					var stored []*Ent
					assert.NoErr(t, ds.GetAll(c, ds.NewQuery("Ent"), &stored))
					assert.Loosely(t, stored, should.HaveLength(len(ents)))
				})
			})

			t.Run(`Can get, put, and delete entities`, func(t *ftt.Test) {
				// Put: "foo", "bar", "baz".
				put := []ds.PropertyMap{
					{"$kind": mkp("test"), "$id": mkp("foo"), "Value": mkp(1337)},
					{"$kind": mkp("test"), "$id": mkp("bar"), "Value": mkp(42)},
					{"$kind": mkp("test"), "$id": mkp("baz"), "Value": mkp(0xd065)},
				}
				assert.Loosely(t, ds.Put(c, put), should.BeNil)

				// Delete: "bar".
				assert.Loosely(t, ds.Delete(c, ds.MakeKey(c, "test", "bar")), should.BeNil)

				// Get: "foo", "bar", "baz"
				get := []ds.PropertyMap{
					{"$kind": mkp("test"), "$id": mkp("foo")},
					{"$kind": mkp("test"), "$id": mkp("bar")},
					{"$kind": mkp("test"), "$id": mkp("baz")},
				}

				err := ds.Get(c, get)
				assert.Loosely(t, err, should.HaveType[errors.MultiError])

				merr := err.(errors.MultiError)
				assert.Loosely(t, len(merr), should.Equal(3))
				assert.Loosely(t, merr[0], should.BeNil)
				assert.Loosely(t, merr[1], should.Equal(ds.ErrNoSuchEntity))
				assert.Loosely(t, merr[2], should.BeNil)

				// put[1] will not be retrieved (delete)
				put[1] = get[1]
				assert.Loosely(t, get, should.Match(put))
			})

			t.Run(`Can put and get all supported entity fields.`, func(t *ftt.Test) {
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
				assert.Loosely(t, ds.Put(c, put), should.BeNil)

				get := ds.PropertyMap{
					"$id":   mkpNI("foo"),
					"$kind": mkpNI("FooType"),
				}
				assert.Loosely(t, ds.Get(c, get), should.BeNil)
				assert.Loosely(t, get, should.Match(put))
			})

			t.Run(`Can Get empty []byte slice as nil`, func(t *ftt.Test) {
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

				assert.Loosely(t, ds.Put(c, put), should.BeNil)
				assert.Loosely(t, ds.Get(c, get), should.BeNil)
				assert.Loosely(t, get, should.Match(exp))
			})

			t.Run(`With several entities installed`, func(t *ftt.Test) {
				assert.Loosely(t, ds.Put(c, []ds.PropertyMap{
					{"$kind": mkp("Test"), "$id": mkp("foo"), "FooBar": mkp(true)},
					{"$kind": mkp("Test"), "$id": mkp("bar"), "FooBar": mkp(true)},
					{"$kind": mkp("Test"), "$id": mkp("baz")},
					{"$kind": mkp("Test"), "$id": mkp("qux")},
					{"$kind": mkp("Test"), "$id": mkp("quux"), "$parent": mkp(ds.MakeKey(c, "Test", "baz"))},
					{"$kind": mkp("Test"), "$id": mkp("quuz"), "$parent": mkp(ds.MakeKey(c, "Test", "baz"))},
					// Entities for checking IN query.
					{"$kind": mkp("AAA"), "$id": mkp("e1"), "Slice": mkp("a", "b")},
					{"$kind": mkp("AAA"), "$id": mkp("e2"), "Slice": mkp("a", "c")},
				}), should.BeNil)

				withAllMeta := func(pm ds.PropertyMap) ds.PropertyMap {
					prop := pm["$key"].(ds.Property)
					key := prop.Value().(*ds.Key)
					pm["$id"] = mkpNI(key.StringID())
					pm["$kind"] = mkpNI(key.Kind())
					pm["$parent"] = mkpNI(key.Parent())
					return pm
				}

				q := ds.NewQuery("Test")

				t.Run(`Can query for entities with FooBar == true.`, func(t *ftt.Test) {
					var results []ds.PropertyMap
					q = q.Eq("FooBar", true)
					assert.Loosely(t, ds.GetAll(c, q, &results), should.BeNil)

					assert.Loosely(t, results, should.Match([]ds.PropertyMap{
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "bar")), "FooBar": mkp(true)}),
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "foo")), "FooBar": mkp(true)}),
					}))
				})

				t.Run(`Can query for entities whose __key__ > "baz".`, func(t *ftt.Test) {
					var results []ds.PropertyMap
					q = q.Gt("__key__", ds.MakeKey(c, "Test", "baz"))
					assert.Loosely(t, ds.GetAll(c, q, &results), should.BeNil)

					assert.Loosely(t, results, should.Match([]ds.PropertyMap{
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "baz", "Test", "quux"))}),
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "baz", "Test", "quuz"))}),
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "foo")), "FooBar": mkp(true)}),
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "qux"))}),
					}))
				})

				t.Run(`Can query for entities whose ancestor is "baz".`, func(t *ftt.Test) {
					var results []ds.PropertyMap
					q := ds.NewQuery("Test").Ancestor(ds.MakeKey(c, "Test", "baz"))
					assert.Loosely(t, ds.GetAll(c, q, &results), should.BeNil)

					assert.Loosely(t, results, should.Match([]ds.PropertyMap{
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "baz"))}),
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "baz", "Test", "quux"))}),
						withAllMeta(ds.PropertyMap{"$key": mkpNI(ds.MakeKey(c, "Test", "baz", "Test", "quuz"))}),
					}))
				})

				t.Run(`Can use IN in queries`, func(t *ftt.Test) {
					// TODO(vadimsh): Unfortunately Cloud Datastore emulator doesn't
					// support IN queries, see https://cloud.google.com/datastore/docs/tools/datastore-emulator#known_issues
					t.Skip("Cloud Datastore emulator doesn't support IN queries")

					var results []*ds.Key
					q := ds.NewQuery("AAA").In("Slice", "b", "c").KeysOnly(true)
					assert.Loosely(t, ds.GetAll(c, q, &results), should.BeNil)
					assert.Loosely(t, results, should.Match([]*ds.Key{
						ds.MakeKey(c, "AAA", "e1"),
						ds.MakeKey(c, "AAA", "e2"),
					}))
				})

				t.Run(`Can transactionally get and put.`, func(t *ftt.Test) {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("qux")}
						if err := ds.Get(c, pmap); err != nil {
							return err
						}

						pmap["ExtraField"] = mkp("Present!")
						return ds.Put(c, pmap)
					}, nil)
					assert.Loosely(t, err, should.BeNil)

					pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("qux")}
					err = ds.RunInTransaction(c, func(c context.Context) error {
						return ds.Get(c, pmap)
					}, nil)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, pmap, should.Match(ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("qux"), "ExtraField": mkp("Present!")}))
				})

				t.Run(`Can fail in a transaction with no effect.`, func(t *ftt.Test) {
					testError := errors.New("test error")

					noTxnPM := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("no txn")}
					err := ds.RunInTransaction(c, func(c context.Context) error {
						assert.Loosely(t, ds.CurrentTransaction(c), should.NotBeNil)

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
					assert.Loosely(t, err, should.Equal(testError))

					// Confirm that noTxnPM was added.
					assert.Loosely(t, ds.CurrentTransaction(c), convey.Adapt(shouldBeUntypedNil)())
					assert.Loosely(t, ds.Get(c, noTxnPM), should.BeNil)

					pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("quux")}
					err = ds.RunInTransaction(c, func(c context.Context) error {
						return ds.Get(c, pmap)
					}, nil)
					assert.Loosely(t, err, should.Equal(ds.ErrNoSuchEntity))
				})
			})
		})
	})
}
