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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"

	"cloud.google.com/go/datastore"
	"go.chromium.org/luci/common/errors"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func mkProperties(index bool, forceMulti bool, vals ...interface{}) ds.PropertyData {
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

func mkp(vals ...interface{}) ds.PropertyData   { return mkProperties(true, false, vals...) }
func mkpNI(vals ...interface{}) ds.PropertyData { return mkProperties(false, false, vals...) }

// TestDatastore tests the cloud datastore implementation.
//
// This test uses the gcloud datastore emulator. Like the Go datastore package,
// the emulator must use the gRPC interface. At the time of writing, the
// emulator included with the "gcloud" tool is an older emulator that does NOT
// support gRPC.
//
// Download the emulator linked here:
// https://code.google.com/p/google-cloud-sdk/issues/detail?id=719#c3
//
// Run it in "--testing" mode, which removes random consistency failures and
// runs in-memory:
// $ ./gcd.sh start --testing
//
// Export the DATASTORE_EMULATOR_HOST environment variable. By default:
// $ export DATASTORE_EMULATOR_HOST=localhost:8080
//
// If the emulator environment is not detected, this test will be skipped.
func TestDatastore(t *testing.T) {
	t.Parallel()

	// See if an emulator is running. If no emulator is running, we will skip this
	// test suite.
	emulatorHost := os.Getenv("DATASTORE_EMULATOR_HOST")
	if emulatorHost == "" {
		t.Logf("No emulator detected (DATASTORE_EMULATOR_HOST). Skipping test suite.")
		return
	}

	Convey(fmt.Sprintf(`A cloud installation using datastore emulator %q`, emulatorHost), t, func() {
		c := context.Background()
		client, err := datastore.NewClient(c, "luci-gae-test")
		So(err, ShouldBeNil)
		defer client.Close()

		testTime := ds.RoundTime(time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC))
		_ = testTime

		cfg := Config{DS: client}
		c = cfg.Use(c, nil)

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
				delete(put[0], "$key")
				delete(put[1], "$key")
				delete(put[2], "$key")

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
				}
				So(ds.Put(c, put), ShouldBeNil)
				delete(put, "$key")

				get := ds.PropertyMap{
					"$id":   mkpNI("foo"),
					"$kind": mkpNI("FooType"),
				}
				So(ds.Get(c, get), ShouldBeNil)
				So(get, ShouldResemble, put)
			})

			Convey(`With several entities installed`, func() {
				So(ds.Put(c, []ds.PropertyMap{
					{"$kind": mkp("Test"), "$id": mkp("foo"), "FooBar": mkp(true)},
					{"$kind": mkp("Test"), "$id": mkp("bar"), "FooBar": mkp(true)},
					{"$kind": mkp("Test"), "$id": mkp("baz")},
					{"$kind": mkp("Test"), "$id": mkp("qux")},
				}), ShouldBeNil)

				q := ds.NewQuery("Test")

				Convey(`Can query for entities with FooBar == true.`, func() {
					var results []ds.PropertyMap
					q = q.Eq("FooBar", true)
					So(ds.GetAll(c, q, &results), ShouldBeNil)

					So(results, ShouldResemble, []ds.PropertyMap{
						{"$key": mkpNI(ds.MakeKey(c, "Test", "bar")), "FooBar": mkp(true)},
						{"$key": mkpNI(ds.MakeKey(c, "Test", "foo")), "FooBar": mkp(true)},
					})
				})

				Convey(`Can query for entities whose __key__ > "baz".`, func() {
					var results []ds.PropertyMap
					q = q.Gt("__key__", ds.MakeKey(c, "Test", "baz"))
					So(ds.GetAll(c, q, &results), ShouldBeNil)

					So(results, ShouldResemble, []ds.PropertyMap{
						{"$key": mkpNI(ds.MakeKey(c, "Test", "foo")), "FooBar": mkp(true)},
						{"$key": mkpNI(ds.MakeKey(c, "Test", "qux"))},
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
					So(ds.CurrentTransaction(c), ShouldBeNil)
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
