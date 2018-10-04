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

package gaesettings

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.chromium.org/gae/filter/count"
	"go.chromium.org/gae/filter/dscache"
	"go.chromium.org/gae/filter/txnBuf"
	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWorks(t *testing.T) {
	Convey("Works", t, func() {
		c := memory.Use(context.Background())
		c = dscache.AlwaysFilterRDS(c)
		c, tc := testclock.UseTime(c, time.Unix(1444945245, 0))

		// Record access to memcache. There should be none.
		c, mcOps := count.FilterMC(c)

		s := Storage{}

		// Nothing's there yet.
		bundle, exp, err := s.FetchAllSettings(c)
		So(err, ShouldBeNil)
		So(exp, ShouldEqual, time.Second)
		So(len(bundle.Values), ShouldEqual, 0)

		conTime, err := s.GetConsistencyTime(c)
		So(conTime.IsZero(), ShouldBeTrue)
		So(err, ShouldBeNil)

		// Produce a bunch of versions.
		tc.Add(time.Minute)
		So(s.UpdateSetting(c, "key", json.RawMessage(`"val1"`), "who1", "why1"), ShouldBeNil)
		tc.Add(time.Minute)
		So(s.UpdateSetting(c, "key", json.RawMessage(`"val2"`), "who2", "why2"), ShouldBeNil)
		tc.Add(time.Minute)
		So(s.UpdateSetting(c, "key", json.RawMessage(`"val3"`), "who3", "why3"), ShouldBeNil)

		bundle, exp, err = s.FetchAllSettings(c)
		So(err, ShouldBeNil)
		So(exp, ShouldEqual, time.Second)
		So(*bundle.Values["key"], ShouldResemble, json.RawMessage(`"val3"`))

		conTime, err = s.GetConsistencyTime(c)
		So(conTime, ShouldResemble, clock.Now(c).UTC().Add(time.Second))
		So(err, ShouldBeNil)

		// Check all log entities is there.
		ds.GetTestable(c).CatchupIndexes()
		entities := []settingsEntity{}
		So(ds.GetAll(c, ds.NewQuery("gaesettings.SettingsLog"), &entities), ShouldBeNil)
		So(len(entities), ShouldEqual, 2)
		asMap := map[string]settingsEntity{}
		for _, e := range entities {
			So(e.Kind, ShouldEqual, "gaesettings.SettingsLog")
			So(e.Parent.Kind(), ShouldEqual, "gaesettings.Settings")
			// Clear some fields to simplify assert below.
			e.Kind = ""
			e.Parent = nil
			e.When = time.Time{}
			asMap[e.ID] = e
		}
		So(asMap, ShouldResemble, map[string]settingsEntity{
			"1": {
				ID:      "1",
				Version: 1,
				Value:   "{\n  \"key\": \"val1\"\n}",
				Who:     "who1",
				Why:     "why1",
			},
			"2": {
				ID:      "2",
				Version: 2,
				Value:   "{\n  \"key\": \"val2\"\n}",
				Who:     "who2",
				Why:     "why2",
			},
		})

		// Memcache must not be used even if dscache is installed in the context.
		So(mcOps.AddMulti.Total(), ShouldEqual, 0)
		So(mcOps.GetMulti.Total(), ShouldEqual, 0)

		// TODO(iannucci): There's a bug in dscache that causes calls to memcache
		// sets and deletes even if dscache is disabled. This should be switched to
		// 0 when it is fixed (the test will break at that moment).
		So(mcOps.SetMulti.Total(), ShouldEqual, 3)
		So(mcOps.DeleteMulti.Total(), ShouldEqual, 3)
	})

	Convey("Handles namespace switch", t, func() {
		c := memory.Use(context.Background())
		c = dscache.AlwaysFilterRDS(c)

		namespaced := info.MustNamespace(c, "blah")

		s := Storage{}

		// Put something using default namespace.
		So(s.UpdateSetting(c, "key", json.RawMessage(`"val1"`), "who1", "why1"), ShouldBeNil)

		// Works when using default namespace.
		bundle, _, err := s.FetchAllSettings(c)
		So(err, ShouldBeNil)
		So(*bundle.Values["key"], ShouldResemble, json.RawMessage(`"val1"`))

		// Works when using non-default namespace too.
		bundle, _, err = s.FetchAllSettings(namespaced)
		So(err, ShouldBeNil)
		So(*bundle.Values["key"], ShouldResemble, json.RawMessage(`"val1"`))

		// Update using non-default namespace.
		So(s.UpdateSetting(namespaced, "key", json.RawMessage(`"val2"`), "who2", "why2"), ShouldBeNil)

		// Works when using default namespace.
		bundle, _, err = s.FetchAllSettings(c)
		So(err, ShouldBeNil)
		So(*bundle.Values["key"], ShouldResemble, json.RawMessage(`"val2"`))
	})

	Convey("Ignores transactions", t, func() {
		c := memory.Use(context.Background())
		s := Storage{}

		// Put something.
		So(s.UpdateSetting(c, "key", json.RawMessage(`"val1"`), "who1", "why1"), ShouldBeNil)

		// Works when fetching outside of a transaction.
		bundle, _, err := s.FetchAllSettings(c)
		So(err, ShouldBeNil)
		So(len(bundle.Values), ShouldEqual, 1)

		// Works when fetching from inside of a transaction.
		ds.RunInTransaction(c, func(c context.Context) error {
			bundle, _, err := s.FetchAllSettings(c)
			So(err, ShouldBeNil)
			So(len(bundle.Values), ShouldEqual, 1)
			return nil
		}, nil)
	})

	Convey("Ignores transactions and namespaces", t, func() {
		c := memory.Use(context.Background())
		s := Storage{}

		// Put something.
		So(s.UpdateSetting(c, "key", json.RawMessage(`"val1"`), "who1", "why1"), ShouldBeNil)

		// Works when fetching outside of a transaction.
		bundle, _, err := s.FetchAllSettings(c)
		So(err, ShouldBeNil)
		So(len(bundle.Values), ShouldEqual, 1)

		// Works when fetching from inside of a transaction.
		namespaced := info.MustNamespace(c, "blah")
		ds.RunInTransaction(namespaced, func(c context.Context) error {
			bundle, _, err := s.FetchAllSettings(c)
			So(err, ShouldBeNil)
			So(len(bundle.Values), ShouldEqual, 1)
			return nil
		}, nil)
	})

	Convey("Ignores transactions and txnBuf", t, func() {
		c := memory.Use(context.Background())
		s := Storage{}

		// Put something.
		So(s.UpdateSetting(c, "key", json.RawMessage(`"val1"`), "who1", "why1"), ShouldBeNil)

		// Works when fetching outside of a transaction.
		bundle, _, err := s.FetchAllSettings(c)
		So(err, ShouldBeNil)
		So(len(bundle.Values), ShouldEqual, 1)

		// Works when fetching from inside of a transaction.
		ds.RunInTransaction(txnBuf.FilterRDS(c), func(c context.Context) error {
			bundle, _, err := s.FetchAllSettings(c)
			So(err, ShouldBeNil)
			So(len(bundle.Values), ShouldEqual, 1)
			return nil
		}, nil)
	})

	Convey("Ignores transactions and namespaces and txnBuf", t, func() {
		c := memory.Use(context.Background())
		s := Storage{}

		// Put something.
		So(s.UpdateSetting(c, "key", json.RawMessage(`"val1"`), "who1", "why1"), ShouldBeNil)

		// Works when fetching outside of a transaction.
		bundle, _, err := s.FetchAllSettings(c)
		So(err, ShouldBeNil)
		So(len(bundle.Values), ShouldEqual, 1)

		// Works when fetching from inside of a transaction.
		namespaced := info.MustNamespace(c, "blah")
		ds.RunInTransaction(txnBuf.FilterRDS(namespaced), func(c context.Context) error {
			bundle, _, err := s.FetchAllSettings(c)
			So(err, ShouldBeNil)
			So(len(bundle.Values), ShouldEqual, 1)
			return nil
		}, nil)
	})
}
