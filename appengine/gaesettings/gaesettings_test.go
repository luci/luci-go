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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/filter/count"
	"go.chromium.org/luci/gae/filter/dscache"
	"go.chromium.org/luci/gae/filter/txnBuf"
	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWorks(t *testing.T) {
	Convey("Works", t, func() {
		ctx := memory.Use(context.Background())
		ctx = dscache.FilterRDS(ctx, nil)
		ctx, tc := testclock.UseTime(ctx, time.Unix(1444945245, 0))

		// Record access to memcache. There should be none.
		ctx, mcOps := count.FilterMC(ctx)

		s := Storage{}

		// Nothing's there yet.
		bundle, exp, err := s.FetchAllSettings(ctx)
		So(err, ShouldBeNil)
		So(exp, ShouldEqual, time.Second)
		So(len(bundle.Values), ShouldEqual, 0)

		conTime, err := s.GetConsistencyTime(ctx)
		So(conTime.IsZero(), ShouldBeTrue)
		So(err, ShouldBeNil)

		// Produce a bunch of versions.
		tc.Add(time.Minute)
		So(s.UpdateSetting(ctx, "key", json.RawMessage(`"val1"`)), ShouldBeNil)
		tc.Add(time.Minute)
		So(s.UpdateSetting(ctx, "key", json.RawMessage(`"val2"`)), ShouldBeNil)
		tc.Add(time.Minute)
		So(s.UpdateSetting(ctx, "key", json.RawMessage(`"val3"`)), ShouldBeNil)

		bundle, exp, err = s.FetchAllSettings(ctx)
		So(err, ShouldBeNil)
		So(exp, ShouldEqual, time.Second)
		So(*bundle.Values["key"], ShouldResemble, json.RawMessage(`"val3"`))

		conTime, err = s.GetConsistencyTime(ctx)
		So(conTime, ShouldResemble, clock.Now(ctx).UTC().Add(time.Second))
		So(err, ShouldBeNil)

		// Check all log entities is there.
		ds.GetTestable(ctx).CatchupIndexes()
		entities := []settingsEntity{}
		So(ds.GetAll(ctx, ds.NewQuery("gaesettings.SettingsLog"), &entities), ShouldBeNil)
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
			},
			"2": {
				ID:      "2",
				Version: 2,
				Value:   "{\n  \"key\": \"val2\"\n}",
			},
		})

		// Memcache must not be used even if dscache is installed in the context.
		So(mcOps.AddMulti.Total(), ShouldEqual, 0)
		So(mcOps.GetMulti.Total(), ShouldEqual, 0)
		So(mcOps.SetMulti.Total(), ShouldEqual, 0)
		So(mcOps.DeleteMulti.Total(), ShouldEqual, 0)
	})

	Convey("Handles namespace switch", t, func() {
		ctx := memory.Use(context.Background())
		ctx = dscache.FilterRDS(ctx, nil)

		namespaced := info.MustNamespace(ctx, "blah")

		s := Storage{}

		// Put something using default namespace.
		So(s.UpdateSetting(ctx, "key", json.RawMessage(`"val1"`)), ShouldBeNil)

		// Works when using default namespace.
		bundle, _, err := s.FetchAllSettings(ctx)
		So(err, ShouldBeNil)
		So(*bundle.Values["key"], ShouldResemble, json.RawMessage(`"val1"`))

		// Works when using non-default namespace too.
		bundle, _, err = s.FetchAllSettings(namespaced)
		So(err, ShouldBeNil)
		So(*bundle.Values["key"], ShouldResemble, json.RawMessage(`"val1"`))

		// Update using non-default namespace.
		So(s.UpdateSetting(namespaced, "key", json.RawMessage(`"val2"`)), ShouldBeNil)

		// Works when using default namespace.
		bundle, _, err = s.FetchAllSettings(ctx)
		So(err, ShouldBeNil)
		So(*bundle.Values["key"], ShouldResemble, json.RawMessage(`"val2"`))
	})

	Convey("Ignores transactions", t, func() {
		ctx := memory.Use(context.Background())
		s := Storage{}

		// Put something.
		So(s.UpdateSetting(ctx, "key", json.RawMessage(`"val1"`)), ShouldBeNil)

		// Works when fetching outside of a transaction.
		bundle, _, err := s.FetchAllSettings(ctx)
		So(err, ShouldBeNil)
		So(len(bundle.Values), ShouldEqual, 1)

		// Works when fetching from inside of a transaction.
		ds.RunInTransaction(ctx, func(ctx context.Context) error {
			bundle, _, err := s.FetchAllSettings(ctx)
			So(err, ShouldBeNil)
			So(len(bundle.Values), ShouldEqual, 1)
			return nil
		}, nil)
	})

	Convey("Ignores transactions and namespaces", t, func() {
		ctx := memory.Use(context.Background())
		s := Storage{}

		// Put something.
		So(s.UpdateSetting(ctx, "key", json.RawMessage(`"val1"`)), ShouldBeNil)

		// Works when fetching outside of a transaction.
		bundle, _, err := s.FetchAllSettings(ctx)
		So(err, ShouldBeNil)
		So(len(bundle.Values), ShouldEqual, 1)

		// Works when fetching from inside of a transaction.
		namespaced := info.MustNamespace(ctx, "blah")
		ds.RunInTransaction(namespaced, func(ctx context.Context) error {
			bundle, _, err := s.FetchAllSettings(ctx)
			So(err, ShouldBeNil)
			So(len(bundle.Values), ShouldEqual, 1)
			return nil
		}, nil)
	})

	Convey("Ignores transactions and txnBuf", t, func() {
		ctx := memory.Use(context.Background())
		s := Storage{}

		// Put something.
		So(s.UpdateSetting(ctx, "key", json.RawMessage(`"val1"`)), ShouldBeNil)

		// Works when fetching outside of a transaction.
		bundle, _, err := s.FetchAllSettings(ctx)
		So(err, ShouldBeNil)
		So(len(bundle.Values), ShouldEqual, 1)

		// Works when fetching from inside of a transaction.
		ds.RunInTransaction(txnBuf.FilterRDS(ctx), func(ctx context.Context) error {
			bundle, _, err := s.FetchAllSettings(ctx)
			So(err, ShouldBeNil)
			So(len(bundle.Values), ShouldEqual, 1)
			return nil
		}, nil)
	})

	Convey("Ignores transactions and namespaces and txnBuf", t, func() {
		ctx := memory.Use(context.Background())
		s := Storage{}

		// Put something.
		So(s.UpdateSetting(ctx, "key", json.RawMessage(`"val1"`)), ShouldBeNil)

		// Works when fetching outside of a transaction.
		bundle, _, err := s.FetchAllSettings(ctx)
		So(err, ShouldBeNil)
		So(len(bundle.Values), ShouldEqual, 1)

		// Works when fetching from inside of a transaction.
		namespaced := info.MustNamespace(ctx, "blah")
		ds.RunInTransaction(txnBuf.FilterRDS(namespaced), func(ctx context.Context) error {
			bundle, _, err := s.FetchAllSettings(ctx)
			So(err, ShouldBeNil)
			So(len(bundle.Values), ShouldEqual, 1)
			return nil
		}, nil)
	})
}
