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
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/count"
	"go.chromium.org/luci/gae/filter/dscache"
	"go.chromium.org/luci/gae/filter/txnBuf"
	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
)

func TestMain(m *testing.M) {
	registry.RegisterCmpOption(cmp.AllowUnexported(settingsEntity{}))
	os.Exit(m.Run())
}

func TestWorks(t *testing.T) {
	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = dscache.FilterRDS(ctx, nil)
		ctx, tc := testclock.UseTime(ctx, time.Unix(1444945245, 0))

		// Record access to memcache. There should be none.
		ctx, mcOps := count.FilterMC(ctx)

		s := Storage{}

		// Nothing's there yet.
		bundle, exp, err := s.FetchAllSettings(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, exp, should.Equal(time.Second))
		assert.Loosely(t, len(bundle.Values), should.BeZero)

		conTime, err := s.GetConsistencyTime(ctx)
		assert.Loosely(t, conTime.IsZero(), should.BeTrue)
		assert.Loosely(t, err, should.BeNil)

		// Produce a bunch of versions.
		tc.Add(time.Minute)
		assert.Loosely(t, s.UpdateSetting(ctx, "key", json.RawMessage(`"val1"`)), should.BeNil)
		tc.Add(time.Minute)
		assert.Loosely(t, s.UpdateSetting(ctx, "key", json.RawMessage(`"val2"`)), should.BeNil)
		tc.Add(time.Minute)
		assert.Loosely(t, s.UpdateSetting(ctx, "key", json.RawMessage(`"val3"`)), should.BeNil)

		bundle, exp, err = s.FetchAllSettings(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, exp, should.Equal(time.Second))
		assert.Loosely(t, *bundle.Values["key"], should.Resemble(json.RawMessage(`"val3"`)))

		conTime, err = s.GetConsistencyTime(ctx)
		assert.Loosely(t, conTime, should.Resemble(clock.Now(ctx).UTC().Add(time.Second)))
		assert.Loosely(t, err, should.BeNil)

		// Check all log entities is there.
		ds.GetTestable(ctx).CatchupIndexes()
		entities := []settingsEntity{}
		assert.Loosely(t, ds.GetAll(ctx, ds.NewQuery("gaesettings.SettingsLog"), &entities), should.BeNil)
		assert.Loosely(t, len(entities), should.Resemble(2))
		asMap := map[string]settingsEntity{}
		for _, e := range entities {
			assert.Loosely(t, e.Kind, should.Resemble("gaesettings.SettingsLog"))
			assert.Loosely(t, e.Parent.Kind(), should.Resemble("gaesettings.Settings"))
			// Clear some fields to simplify assert below.
			e.Kind = ""
			e.Parent = nil
			e.When = time.Time{}
			asMap[e.ID] = e
		}
		assert.Loosely(t, asMap, should.Resemble(map[string]settingsEntity{
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
		}))
		assert.

			// Memcache must not be used even if dscache is installed in the context.
			Loosely(t, mcOps.AddMulti.Total(), should.BeZero)
		assert.Loosely(t, mcOps.GetMulti.Total(), should.BeZero)
		assert.Loosely(t, mcOps.SetMulti.Total(), should.BeZero)
		assert.Loosely(t, mcOps.DeleteMulti.Total(), should.BeZero)
	})

	ftt.Run("Handles namespace switch", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = dscache.FilterRDS(ctx, nil)

		namespaced := info.MustNamespace(ctx, "blah")

		s := Storage{}
		assert.

			// Put something using default namespace.
			Loosely(t, s.UpdateSetting(ctx, "key", json.RawMessage(`"val1"`)), should.BeNil)

		// Works when using default namespace.
		bundle, _, err := s.FetchAllSettings(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, *bundle.Values["key"], should.Resemble(json.RawMessage(`"val1"`)))

		// Works when using non-default namespace too.
		bundle, _, err = s.FetchAllSettings(namespaced)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, *bundle.Values["key"], should.Resemble(json.RawMessage(`"val1"`)))
		assert.

			// Update using non-default namespace.
			Loosely(t, s.UpdateSetting(namespaced, "key", json.RawMessage(`"val2"`)), should.BeNil)

		// Works when using default namespace.
		bundle, _, err = s.FetchAllSettings(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, *bundle.Values["key"], should.Resemble(json.RawMessage(`"val2"`)))
	})

	ftt.Run("Ignores transactions", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		s := Storage{}
		assert.

			// Put something.
			Loosely(t, s.UpdateSetting(ctx, "key", json.RawMessage(`"val1"`)), should.BeNil)

		// Works when fetching outside of a transaction.
		bundle, _, err := s.FetchAllSettings(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(bundle.Values), should.Resemble(1))

		// Works when fetching from inside of a transaction.
		ds.RunInTransaction(ctx, func(ctx context.Context) error {
			bundle, _, err := s.FetchAllSettings(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(bundle.Values), should.Resemble(1))
			return nil
		}, nil)
	})

	ftt.Run("Ignores transactions and namespaces", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		s := Storage{}
		assert.

			// Put something.
			Loosely(t, s.UpdateSetting(ctx, "key", json.RawMessage(`"val1"`)), should.BeNil)

		// Works when fetching outside of a transaction.
		bundle, _, err := s.FetchAllSettings(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(bundle.Values), should.Resemble(1))

		// Works when fetching from inside of a transaction.
		namespaced := info.MustNamespace(ctx, "blah")
		ds.RunInTransaction(namespaced, func(ctx context.Context) error {
			bundle, _, err := s.FetchAllSettings(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(bundle.Values), should.Resemble(1))
			return nil
		}, nil)
	})

	ftt.Run("Ignores transactions and txnBuf", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		s := Storage{}
		assert.

			// Put something.
			Loosely(t, s.UpdateSetting(ctx, "key", json.RawMessage(`"val1"`)), should.BeNil)

		// Works when fetching outside of a transaction.
		bundle, _, err := s.FetchAllSettings(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(bundle.Values), should.Resemble(1))

		// Works when fetching from inside of a transaction.
		ds.RunInTransaction(txnBuf.FilterRDS(ctx), func(ctx context.Context) error {
			bundle, _, err := s.FetchAllSettings(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(bundle.Values), should.Resemble(1))
			return nil
		}, nil)
	})

	ftt.Run("Ignores transactions and namespaces and txnBuf", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		s := Storage{}
		assert.

			// Put something.
			Loosely(t, s.UpdateSetting(ctx, "key", json.RawMessage(`"val1"`)), should.BeNil)

		// Works when fetching outside of a transaction.
		bundle, _, err := s.FetchAllSettings(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(bundle.Values), should.Resemble(1))

		// Works when fetching from inside of a transaction.
		namespaced := info.MustNamespace(ctx, "blah")
		ds.RunInTransaction(txnBuf.FilterRDS(namespaced), func(ctx context.Context) error {
			bundle, _, err := s.FetchAllSettings(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(bundle.Values), should.Resemble(1))
			return nil
		}, nil)
	})
}
