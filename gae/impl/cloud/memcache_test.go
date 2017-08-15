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
	"flag"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/gae/service/info"
	mc "go.chromium.org/gae/service/memcache"

	"github.com/bradfitz/gomemcache/memcache"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

var memcacheServer = flag.String("test.memcache-server", "",
	"[<addr>]:<port> of memcached service to test against. THIS WILL FLUSH THE CACHE.")

// TestMemcache tests the memcache implementation against a live "memcached"
// instance. The test assumes ownership of the instance, and will flush all of
// its keys in between test suites, so DO NOT connect this to a production
// memcached cluster!
//
// The memcache host is passed to this test suite via the "-test.memcache-host"
// flag. If the flag is not provided, this test suite will be skipped.
//
// Starting a local memcached server (on default port 11211) can be done with:
//	$ memcached -l localhost -vvv
//
// Testing against this service can be done using the flag:
// "-test.memcache-server localhost:11211
func TestMemcache(t *testing.T) {
	t.Parallel()

	// See if a memcache server is configured. If no server is configured, we will
	// skip this test suite.
	if *memcacheServer == "" {
		t.Logf("No memcache server detected (-test.memcache-server). Skipping test suite.")
		return
	}

	Convey(fmt.Sprintf(`A memcache instance bound to %q`, *memcacheServer), t, func() {
		client := memcache.New(*memcacheServer)
		if err := client.DeleteAll(); err != nil {
			t.Fatalf("failed to flush memcache before running test suite: %s", err)
		}

		cfg := Config{MC: client}
		c := cfg.Use(context.Background(), nil)

		get := func(c context.Context, keys ...string) []string {
			bmc := bindMemcacheClient(nil, info.GetNamespace(c))

			v := make([]string, len(keys))
			for i, k := range keys {
				itm, err := client.Get(bmc.makeKey(k))
				switch err {
				case nil:
					v[i] = string(itm.Value)

				case memcache.ErrCacheMiss:
					break

				default:
					t.Fatalf("item %q could not be loaded: %s", itm.Key, err)
				}
			}
			return v
		}

		Convey(`AddMulti`, func() {
			items := []mc.Item{
				mc.NewItem(c, "foo").SetValue([]byte("FOO")),
				mc.NewItem(c, "bar").SetValue([]byte("BAR")),
				mc.NewItem(c, "baz").SetValue([]byte("BAZ")),
			}
			So(mc.Add(c, items...), ShouldBeNil)
			So(get(c, "foo", "bar", "baz"), ShouldResemble, []string{"FOO", "BAR", "BAZ"})

			// Namespaced.
			oc := info.MustNamespace(c, "other")
			oitems := make([]mc.Item, len(items))
			for i, itm := range items {
				oitems[i] = mc.NewItem(oc, itm.Key())
				oitems[i].SetValue([]byte("OTHER_" + string(itm.Value())))
			}
			So(mc.Add(oc, oitems...), ShouldBeNil)
			So(get(oc, "foo", "bar", "baz"), ShouldResemble, []string{"OTHER_FOO", "OTHER_BAR", "OTHER_BAZ"})

			Convey(`Returns ErrNotStored if the item is already added.`, func() {
				So(mc.Add(c, items...), ShouldResemble, errors.MultiError{
					mc.ErrNotStored, mc.ErrNotStored, mc.ErrNotStored})
			})

			Convey(`GetMulti`, func() {
				getItems := []mc.Item{
					mc.NewItem(c, "foo"),
					mc.NewItem(c, "bar"),
					mc.NewItem(c, "baz"),
				}
				So(mc.Get(c, getItems...), ShouldBeNil)
				So(getItems[0].Value(), ShouldResemble, []byte("FOO"))
				So(getItems[1].Value(), ShouldResemble, []byte("BAR"))
				So(getItems[2].Value(), ShouldResemble, []byte("BAZ"))

				// Namespaced.
				So(mc.Get(oc, getItems...), ShouldBeNil)
				So(oitems[0].Value(), ShouldResemble, []byte("OTHER_FOO"))
				So(oitems[1].Value(), ShouldResemble, []byte("OTHER_BAR"))
				So(oitems[2].Value(), ShouldResemble, []byte("OTHER_BAZ"))
			})

			Convey(`SetMulti`, func() {
				items[2].SetValue([]byte("NEWBAZ"))
				items = append(items, mc.NewItem(c, "qux").SetValue([]byte("QUX")))

				oitems[2].SetValue([]byte("OTHER_NEWBAZ"))
				oitems = append(oitems, mc.NewItem(c, "qux").SetValue([]byte("OTHER_QUX")))

				So(mc.Set(c, items...), ShouldBeNil)
				So(mc.Set(oc, oitems...), ShouldBeNil)

				So(get(c, "foo", "bar", "baz", "qux"), ShouldResemble, []string{"FOO", "BAR", "NEWBAZ", "QUX"})
				So(get(oc, "foo", "bar", "baz", "qux"), ShouldResemble,
					[]string{"OTHER_FOO", "OTHER_BAR", "OTHER_NEWBAZ", "OTHER_QUX"})
			})

			Convey(`DeleteMulti`, func() {
				So(mc.Delete(c, "foo", "bar"), ShouldBeNil)
				So(get(c, "foo", "bar", "baz"), ShouldResemble, []string{"", "", "BAZ"})

				So(mc.Delete(oc, "foo", "bar"), ShouldBeNil)
				So(get(oc, "foo", "bar", "baz"), ShouldResemble, []string{"", "", "OTHER_BAZ"})
			})

			Convey(`CompareAndSwapMulti`, func() {
				// Get all of the values.
				So(mc.Get(c, items...), ShouldBeNil)

				// Change "foo"'s value.
				newFoo := mc.NewItem(c, "foo")
				newFoo.SetAll(items[0])
				newFoo.SetValue([]byte("NEWFOO"))
				So(mc.Set(c, newFoo), ShouldBeNil)

				Convey(`Works`, func() {
					// Do CAS on foo, bar. "foo" should fail because it's changed.
					items[0].SetValue([]byte("CASFOO"))
					items[1].SetValue([]byte("CASBAR"))
					So(mc.CompareAndSwap(c, items...), ShouldResemble, errors.MultiError{mc.ErrCASConflict, nil, nil})

					So(get(c, "foo", "bar", "baz"), ShouldResemble, []string{"NEWFOO", "CASBAR", "BAZ"})
				})

				Convey(`SetAll transfers CAS ID`, func() {
					// Do CAS on foo, bar. "foo" should fail because it's changed.
					foo := mc.NewItem(c, "foo")
					foo.SetAll(items[0])
					foo.SetValue([]byte("CASFOO"))

					bar := mc.NewItem(c, "bar")
					bar.SetAll(items[1])
					bar.SetValue([]byte("CASBAR"))

					So(mc.CompareAndSwap(c, foo, bar), ShouldResemble, errors.MultiError{mc.ErrCASConflict, nil})

					So(get(c, "foo", "bar"), ShouldResemble, []string{"NEWFOO", "CASBAR"})
				})
			})

			Convey(`Flush`, func() {
				So(mc.Flush(c), ShouldBeNil)
				So(get(c, "foo", "bar", "baz"), ShouldResemble, []string{"", "", ""})
			})
		})

		Convey(`Increment`, func() {

			Convey(`Missing`, func() {
				// IncrementExisting on non-existent item will return ErrCacheMiss.
				_, err := mc.IncrementExisting(c, "foo", 1)
				So(err, ShouldEqual, mc.ErrCacheMiss)

				// IncrementExisting with delta of 0 on non-existent item will return
				// ErrCacheMiss.
				_, err = mc.IncrementExisting(c, "foo", 1)
				So(err, ShouldEqual, mc.ErrCacheMiss)
			})

			Convey(`Initial value (positive)`, func() {
				// Can set the initial value.
				nv, err := mc.Increment(c, "foo", 1, 1336)
				So(err, ShouldBeNil)
				So(nv, ShouldEqual, 1337)
				So(get(c, "foo"), ShouldResemble, []string{"1337"})

				// Followup increment (initial value is ignored).
				nv, err = mc.Increment(c, "foo", 1, 20000)
				So(err, ShouldBeNil)
				So(nv, ShouldEqual, 1338)
			})

			Convey(`Initial value (positive, overflows)`, func() {
				// Can set the initial value.
				nv, err := mc.Increment(c, "foo", 10, math.MaxUint64)
				So(err, ShouldBeNil)
				So(nv, ShouldEqual, 9)
				So(get(c, "foo"), ShouldResemble, []string{"9"})
			})

			Convey(`Initial value (negative)`, func() {
				// Can set the initial value.
				nv, err := mc.Increment(c, "foo", -1, 1338)
				So(err, ShouldBeNil)
				So(nv, ShouldEqual, 1337)
				So(get(c, "foo"), ShouldResemble, []string{"1337"})
			})

			Convey(`Initial value (negative, underflows to zero)`, func() {
				// Can set the initial value.
				nv, err := mc.Increment(c, "foo", -1337, 2)
				So(err, ShouldBeNil)
				So(nv, ShouldEqual, 0)
				So(get(c, "foo"), ShouldResemble, []string{"0"})
			})

			Convey(`With large initial value (zero delta)`, func() {
				const iv = uint64(math.MaxUint64 - 1)

				// Can set the initial value.
				nv, err := mc.Increment(c, "foo", 0, iv)
				So(err, ShouldBeNil)
				So(nv, ShouldEqual, uint64(math.MaxUint64-1))

				Convey(`Can increment`, func() {
					nv, err := mc.IncrementExisting(c, "foo", 1)
					So(err, ShouldBeNil)
					So(nv, ShouldEqual, iv+1)
				})

				Convey(`Can increment (overflow, wraps)`, func() {
					nv, err := mc.IncrementExisting(c, "foo", 10)
					So(err, ShouldBeNil)
					So(nv, ShouldEqual, 8)
				})

				Convey(`Can decrement`, func() {
					nv, err := mc.IncrementExisting(c, "foo", -123)
					So(err, ShouldBeNil)
					So(nv, ShouldEqual, iv-123)
				})
			})

			Convey(`With small initial value (zero delta)`, func() {
				// Can set the initial value.
				nv, err := mc.Increment(c, "foo", 0, 1337)
				So(err, ShouldBeNil)
				So(nv, ShouldEqual, 1337)

				Convey(`Can decrement (underflow to zero)`, func() {
					nv, err := mc.IncrementExisting(c, "foo", -2000)
					So(err, ShouldBeNil)
					So(nv, ShouldEqual, 0)
				})
			})

			Convey(`Namespaced`, func() {
				oc := info.MustNamespace(c, "other")

				// Initial (non-namespaced).
				nv, err := mc.Increment(c, "foo", 1, 100)
				So(err, ShouldBeNil)
				So(nv, ShouldEqual, 101)

				// Initial (namespaced).
				_, err = mc.IncrementExisting(oc, "foo", -1)
				So(err, ShouldEqual, mc.ErrCacheMiss)

				nv, err = mc.Increment(oc, "foo", -1, 20000)
				So(err, ShouldBeNil)
				So(nv, ShouldEqual, 19999)

				// Increment (namespaced).
				nv, err = mc.IncrementExisting(oc, "foo", 1)
				So(err, ShouldBeNil)
				So(nv, ShouldEqual, 20000)

				// Increment (non-namespaced).
				nv, err = mc.IncrementExisting(c, "foo", 1)
				So(err, ShouldBeNil)
				So(nv, ShouldEqual, 102)
			})
		})

		Convey(`Stats returns ErrNoStats`, func() {
			_, err := mc.Stats(c)
			So(err, ShouldEqual, mc.ErrNoStats)
		})

		Convey(`A really long key gets hashed.`, func() {
			key := strings.Repeat("X", keyHashSizeThreshold+1)
			item := mc.NewItem(c, key)
			item.SetValue([]byte("ohaithere"))
			So(mc.Add(c, item), ShouldBeNil)

			// Retrieve the item. The memcache layer doesn't change the key, so we
			// will assert this by building the hash key ourselves and asserting that
			// it also retrieves the item.
			hashedKey := hashBytes([]byte(key))
			So(get(c, hashedKey), ShouldResemble, []string{"ohaithere"})
		})

		Convey(`Items will expire (sleeps 1 second).`, func() {
			item := mc.NewItem(c, "foo").SetValue([]byte("FOO")).SetExpiration(1 * time.Second)
			So(mc.Add(c, item), ShouldBeNil)

			// Immediate Get.
			So(mc.Get(c, item), ShouldBeNil)
			So(item.Value(), ShouldResemble, []byte("FOO"))

			// Expire.
			time.Sleep(1 * time.Second)
			So(mc.Get(c, item), ShouldEqual, mc.ErrCacheMiss)
		})
	})
}
