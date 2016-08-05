// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cloud

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"

	"cloud.google.com/go/datastore"
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func mkProperties(index bool, vals ...interface{}) []ds.Property {
	indexSetting := ds.ShouldIndex
	if !index {
		indexSetting = ds.NoIndex
	}

	result := make([]ds.Property, len(vals))
	for i, v := range vals {
		result[i].SetValue(v, indexSetting)
	}
	return result
}

func mkp(vals ...interface{}) []ds.Property   { return mkProperties(true, vals...) }
func mkpNI(vals ...interface{}) []ds.Property { return mkProperties(false, vals...) }

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
		t.Logf("No emulator detected. Skipping test suite.")
		return
	}

	Convey(fmt.Sprintf(`A cloud installation using datastore emulator %q`, emulatorHost), t, func() {
		c := context.Background()
		client, err := datastore.NewClient(c, "luci-gae-test")
		So(err, ShouldBeNil)
		defer client.Close()

		testTime := ds.RoundTime(time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC))

		c = Use(c, client)

		Convey(`Supports namespaces`, func() {
			namespaces := []string{"foo", "bar", "baz"}

			// Clear all used entities from all namespaces.
			for _, ns := range namespaces {
				nsCtx := info.Get(c).MustNamespace(ns)
				di := ds.Get(nsCtx)

				keys := make([]*ds.Key, len(namespaces))
				for i := range keys {
					keys[i] = di.MakeKey("Test", i+1)
				}
				So(errors.Filter(di.DeleteMulti(keys), ds.ErrNoSuchEntity), ShouldBeNil)
			}

			// Put one entity per namespace.
			for i, ns := range namespaces {
				nsCtx := info.Get(c).MustNamespace(ns)

				pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp(i + 1), "Value": mkp(i)}
				So(ds.Get(nsCtx).Put(pmap), ShouldBeNil)
			}

			// Make sure that entity only exists in that namespace.
			for _, ns := range namespaces {
				nsCtx := info.Get(c).MustNamespace(ns)

				for i := range namespaces {
					pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp(i + 1)}
					err := ds.Get(nsCtx).Get(pmap)

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
			c = info.Get(c).MustNamespace(fmt.Sprintf("testing-%s", hex.EncodeToString(randNamespace)))
			di := ds.Get(c)

			// Execute a kindless query to clear the namespace.
			q := ds.NewQuery("").KeysOnly(true)
			var allKeys []*ds.Key
			So(di.GetAll(q, &allKeys), ShouldBeNil)
			So(di.DeleteMulti(allKeys), ShouldBeNil)

			Convey(`Can allocate an ID range`, func() {
				var keys []*ds.Key
				keys = append(keys, di.NewIncompleteKeys(10, "Bar", di.MakeKey("Foo", 12))...)
				keys = append(keys, di.NewIncompleteKeys(10, "Baz", di.MakeKey("Foo", 12))...)

				seen := map[string]struct{}{}
				So(di.AllocateIDs(keys), ShouldBeNil)
				for _, k := range keys {
					So(k.IsIncomplete(), ShouldBeFalse)
					seen[k.String()] = struct{}{}
				}

				So(di.AllocateIDs(keys), ShouldBeNil)
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
				So(di.PutMulti(put), ShouldBeNil)
				delete(put[0], "$key")
				delete(put[1], "$key")
				delete(put[2], "$key")

				// Delete: "bar".
				So(di.Delete(di.MakeKey("test", "bar")), ShouldBeNil)

				// Get: "foo", "bar", "baz"
				get := []ds.PropertyMap{
					{"$kind": mkp("test"), "$id": mkp("foo")},
					{"$kind": mkp("test"), "$id": mkp("bar")},
					{"$kind": mkp("test"), "$id": mkp("baz")},
				}

				err := di.GetMulti(get)
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
					"$id":    mkpNI("foo"),
					"$kind":  mkpNI("FooType"),
					"Number": mkp(1337),
					"String": mkpNI("hello"),
					"Bytes":  mkp([]byte("world")),
					"Time":   mkp(testTime),
					"Float":  mkpNI(3.14),
					"Key":    mkp(di.MakeKey("Parent", "ParentID", "Child", 1337)),

					"ComplexSlice": mkp(1337, "string", []byte("bytes"), testTime, float32(3.14),
						float64(2.71), true, di.MakeKey("SomeKey", "SomeID")),
				}
				So(di.Put(put), ShouldBeNil)
				delete(put, "$key")

				get := ds.PropertyMap{
					"$id":   mkpNI("foo"),
					"$kind": mkpNI("FooType"),
				}
				So(di.Get(get), ShouldBeNil)
				So(get, ShouldResemble, put)
			})

			Convey(`With several entities installed`, func() {
				So(di.PutMulti([]ds.PropertyMap{
					{"$kind": mkp("Test"), "$id": mkp("foo"), "FooBar": mkp(true)},
					{"$kind": mkp("Test"), "$id": mkp("bar"), "FooBar": mkp(true)},
					{"$kind": mkp("Test"), "$id": mkp("baz")},
					{"$kind": mkp("Test"), "$id": mkp("qux")},
				}), ShouldBeNil)

				q := ds.NewQuery("Test")

				Convey(`Can query for entities with FooBar == true.`, func() {
					var results []ds.PropertyMap
					q = q.Eq("FooBar", true)
					So(di.GetAll(q, &results), ShouldBeNil)

					So(results, ShouldResemble, []ds.PropertyMap{
						{"$key": mkpNI(di.MakeKey("Test", "bar")), "FooBar": mkp(true)},
						{"$key": mkpNI(di.MakeKey("Test", "foo")), "FooBar": mkp(true)},
					})
				})

				Convey(`Can query for entities whose __key__ > "baz".`, func() {
					var results []ds.PropertyMap
					q = q.Gt("__key__", di.MakeKey("Test", "baz"))
					So(di.GetAll(q, &results), ShouldBeNil)

					So(results, ShouldResemble, []ds.PropertyMap{
						{"$key": mkpNI(di.MakeKey("Test", "foo")), "FooBar": mkp(true)},
						{"$key": mkpNI(di.MakeKey("Test", "qux"))},
					})
				})

				Convey(`Can transactionally get and put.`, func() {
					err := di.RunInTransaction(func(c context.Context) error {
						di := ds.Get(c)

						pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("qux")}
						if err := di.Get(pmap); err != nil {
							return err
						}

						pmap["ExtraField"] = mkp("Present!")
						return di.Put(pmap)
					}, nil)
					So(err, ShouldBeNil)

					pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("qux")}
					err = di.RunInTransaction(func(c context.Context) error {
						return ds.Get(c).Get(pmap)
					}, nil)
					So(err, ShouldBeNil)
					So(pmap, ShouldResemble, ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("qux"), "ExtraField": mkp("Present!")})
				})

				Convey(`Can fail in a transaction with no effect.`, func() {
					testError := errors.New("test error")

					err := di.RunInTransaction(func(c context.Context) error {
						di := ds.Get(c)

						pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("quux")}
						if err := di.Put(pmap); err != nil {
							return err
						}
						return testError
					}, nil)
					So(err, ShouldEqual, testError)

					pmap := ds.PropertyMap{"$kind": mkp("Test"), "$id": mkp("quux")}
					err = di.RunInTransaction(func(c context.Context) error {
						return ds.Get(c).Get(pmap)
					}, nil)
					So(err, ShouldEqual, ds.ErrNoSuchEntity)
				})
			})
		})
	})
}
