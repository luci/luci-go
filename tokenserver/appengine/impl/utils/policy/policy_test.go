// Copyright 2017 The LUCI Authors.
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

package policy

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/filter/count"
)

// queryableForm implements Queryable.
type queryableForm struct {
	rev    string
	bundle ConfigBundle
}

func (q *queryableForm) ConfigRevision() string {
	return q.rev
}

func TestImportConfigs(t *testing.T) {
	t.Parallel()

	ftt.Run("Happy path", t, func(t *ftt.Test) {
		base := gaetesting.TestingContext()
		c := prepareServiceConfig(base, map[string]string{
			"config_1.cfg": "seconds: 12345",
			"config_2.cfg": "seconds: 67890",
		})

		fetchCalls := 0
		prepareCalls := 0

		p := Policy{
			Name: "testing",
			Fetch: func(c context.Context, f ConfigFetcher) (ConfigBundle, error) {
				fetchCalls++
				var ts1, ts2 timestamppb.Timestamp
				assert.Loosely(t, f.FetchTextProto(c, "config_1.cfg", &ts1), should.BeNil)
				assert.Loosely(t, f.FetchTextProto(c, "config_2.cfg", &ts2), should.BeNil)
				return ConfigBundle{"config_1.cfg": &ts1, "config_2.cfg": &ts2}, nil
			},
			Prepare: func(c context.Context, cfg ConfigBundle, revision string) (Queryable, error) {
				prepareCalls++
				return &queryableForm{revision, cfg}, nil
			},
		}

		// Fetched for the first time.
		rev, err := p.ImportConfigs(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, rev, should.Equal("dc17888617ce2d87e0e33c1ad12034d51127fda3"))
		assert.Loosely(t, fetchCalls, should.Equal(1))
		assert.Loosely(t, prepareCalls, should.Equal(1))

		// No change in configs -> early exit.
		rev, err = p.ImportConfigs(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, rev, should.Equal("dc17888617ce2d87e0e33c1ad12034d51127fda3"))
		assert.Loosely(t, fetchCalls, should.Equal(2))
		assert.Loosely(t, prepareCalls, should.Equal(1))

		// New configs appear.
		c = prepareServiceConfig(base, map[string]string{
			"config_1.cfg": "seconds: 11111",
			"config_2.cfg": "seconds: 22222",
		})
		rev, err = p.ImportConfigs(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, rev, should.Equal("6bc6fb514320d908db4955ff8769f59e2cd0ae09"))
		assert.Loosely(t, fetchCalls, should.Equal(3))
		assert.Loosely(t, prepareCalls, should.Equal(2))
	})

	ftt.Run("Validation errors", t, func(t *ftt.Test) {
		base := gaetesting.TestingContext()
		c := prepareServiceConfig(base, map[string]string{
			"config.cfg": "seconds: 12345",
		})

		p := Policy{
			Name: "testing",
			Fetch: func(c context.Context, f ConfigFetcher) (ConfigBundle, error) {
				var ts timestamppb.Timestamp
				assert.Loosely(t, f.FetchTextProto(c, "config.cfg", &ts), should.BeNil)
				return ConfigBundle{"config.cfg": &ts}, nil
			},
			Validate: func(v *validation.Context, cfg ConfigBundle) {
				assert.Loosely(t, cfg["config.cfg"], should.NotBeNil)
				v.SetFile("config.cfg")
				v.Errorf("validation error")
			},
			Prepare: func(c context.Context, cfg ConfigBundle, revision string) (Queryable, error) {
				panic("must not be called")
			},
		}

		_, err := p.ImportConfigs(c)
		assert.Loosely(t, err, should.ErrLike(`in "config.cfg": validation error`))
	})
}

func TestQueryable(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		base, tc := testclock.UseTime(gaetesting.TestingContext(), testclock.TestTimeUTC)
		base = mathrand.Set(base, rand.New(rand.NewSource(2)))
		base, counter := count.FilterRDS(base)

		p := Policy{
			Name: "testing",
			Fetch: func(c context.Context, f ConfigFetcher) (ConfigBundle, error) {
				var ts timestamppb.Timestamp
				assert.Loosely(t, f.FetchTextProto(c, "config.cfg", &ts), should.BeNil)
				return ConfigBundle{"config.cfg": &ts}, nil
			},
			Prepare: func(c context.Context, cfg ConfigBundle, revision string) (Queryable, error) {
				return &queryableForm{revision, cfg}, nil
			},
		}

		// No imported configs yet.
		q, err := p.Queryable(base)
		assert.Loosely(t, err, should.Equal(ErrNoPolicy))
		assert.Loosely(t, q, should.BeNil)

		c1 := prepareServiceConfig(base, map[string]string{
			"config.cfg": "seconds: 12345",
		})
		rev1, err := p.ImportConfigs(c1)
		assert.Loosely(t, err, should.BeNil)

		q, err = p.Queryable(base)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, q.ConfigRevision(), should.Equal(rev1))
		qf := q.(*queryableForm)
		assert.Loosely(t, qf.bundle["config.cfg"], should.Match(&timestamppb.Timestamp{Seconds: 12345}))

		assert.Loosely(t, counter.GetMulti.Total(), should.Equal(4))

		// 3 min later using exact same config, no datastore calls.
		tc.Add(3 * time.Minute)
		q, err = p.Queryable(base)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, q.(*queryableForm), should.Equal(qf))
		assert.Loosely(t, counter.GetMulti.Total(), should.Equal(4))

		// 2 min after that, the datastore is rechecked, since the local cache has
		// expired. No new configs there, exact same 'q' is returned.
		tc.Add(2 * time.Minute)
		q, err = p.Queryable(base)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, q.(*queryableForm), should.Equal(qf))
		assert.Loosely(t, counter.GetMulti.Total(), should.Equal(5)) // datastore call!

		// Config has been updated.
		c2 := prepareServiceConfig(base, map[string]string{
			"config.cfg": "seconds: 6789",
		})
		rev2, err := p.ImportConfigs(c2)
		assert.Loosely(t, err, should.BeNil)

		// Currently cached config is still fresh though, so it is still used.
		q, err = p.Queryable(base)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, q.ConfigRevision(), should.Equal(rev1))

		// 5 minutes later the cached copy expires and new one is fetched from
		// the datastore.
		tc.Add(5 * time.Minute)
		q, err = p.Queryable(base)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, q.ConfigRevision(), should.Equal(rev2))
		qf = q.(*queryableForm)
		assert.Loosely(t, qf.bundle["config.cfg"], should.Match(&timestamppb.Timestamp{Seconds: 6789}))
	})
}
