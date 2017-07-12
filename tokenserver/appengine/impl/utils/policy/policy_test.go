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
	"math/rand"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/luci/gae/filter/count"
	"github.com/luci/luci-go/appengine/gaetesting"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/config/validation"
	"github.com/luci/luci-go/common/data/rand/mathrand"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
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

	Convey("Happy path", t, func() {
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
				var ts1, ts2 timestamp.Timestamp
				So(f.FetchTextProto(c, "config_1.cfg", &ts1), ShouldBeNil)
				So(f.FetchTextProto(c, "config_2.cfg", &ts2), ShouldBeNil)
				return ConfigBundle{"config_1.cfg": &ts1, "config_2.cfg": &ts2}, nil
			},
			Prepare: func(cfg ConfigBundle, revision string) (Queryable, error) {
				prepareCalls++
				return &queryableForm{revision, cfg}, nil
			},
		}

		// Fetched for the first time.
		rev, err := p.ImportConfigs(c)
		So(err, ShouldBeNil)
		So(rev, ShouldEqual, "5ff9f21804de0eb09acc939efe2da14dfccc47ef")
		So(fetchCalls, ShouldEqual, 1)
		So(prepareCalls, ShouldEqual, 1)

		// No change in configs -> early exit.
		rev, err = p.ImportConfigs(c)
		So(err, ShouldBeNil)
		So(rev, ShouldEqual, "5ff9f21804de0eb09acc939efe2da14dfccc47ef")
		So(fetchCalls, ShouldEqual, 2)
		So(prepareCalls, ShouldEqual, 1)

		// New configs appear.
		c = prepareServiceConfig(base, map[string]string{
			"config_1.cfg": "seconds: 11111",
			"config_2.cfg": "seconds: 22222",
		})
		rev, err = p.ImportConfigs(c)
		So(err, ShouldBeNil)
		So(rev, ShouldEqual, "c012dcea7abf16792b1c728775ea1955ac8f3a20")
		So(fetchCalls, ShouldEqual, 3)
		So(prepareCalls, ShouldEqual, 2)
	})

	Convey("Validation errors", t, func() {
		base := gaetesting.TestingContext()
		c := prepareServiceConfig(base, map[string]string{
			"config.cfg": "seconds: 12345",
		})

		p := Policy{
			Name: "testing",
			Fetch: func(c context.Context, f ConfigFetcher) (ConfigBundle, error) {
				var ts timestamp.Timestamp
				So(f.FetchTextProto(c, "config.cfg", &ts), ShouldBeNil)
				return ConfigBundle{"config.cfg": &ts}, nil
			},
			Validate: func(cfg ConfigBundle, v *validation.Context) {
				So(cfg["config.cfg"], ShouldNotBeNil)
				v.SetFile("config.cfg")
				v.Error("validation error")
			},
			Prepare: func(cfg ConfigBundle, revision string) (Queryable, error) {
				panic("must not be called")
			},
		}

		_, err := p.ImportConfigs(c)
		So(err, ShouldErrLike, `in "config.cfg": validation error`)
	})
}

func TestQueryable(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		base, tc := testclock.UseTime(gaetesting.TestingContext(), testclock.TestTimeUTC)
		base = mathrand.Set(base, rand.New(rand.NewSource(2)))
		base, counter := count.FilterRDS(base)

		p := Policy{
			Name: "testing",
			Fetch: func(c context.Context, f ConfigFetcher) (ConfigBundle, error) {
				var ts timestamp.Timestamp
				So(f.FetchTextProto(c, "config.cfg", &ts), ShouldBeNil)
				return ConfigBundle{"config.cfg": &ts}, nil
			},
			Prepare: func(cfg ConfigBundle, revision string) (Queryable, error) {
				return &queryableForm{revision, cfg}, nil
			},
		}

		// No imported configs yet.
		q, err := p.Queryable(base)
		So(err, ShouldEqual, ErrNoPolicy)
		So(q, ShouldEqual, nil)

		c1 := prepareServiceConfig(base, map[string]string{
			"config.cfg": "seconds: 12345",
		})
		rev1, err := p.ImportConfigs(c1)
		So(err, ShouldBeNil)

		q, err = p.Queryable(base)
		So(err, ShouldBeNil)
		So(q.ConfigRevision(), ShouldEqual, rev1)
		qf := q.(*queryableForm)
		So(qf.bundle["config.cfg"], ShouldResemble, &timestamp.Timestamp{Seconds: 12345})

		So(counter.GetMulti.Total(), ShouldEqual, 4)

		// 3 min later using exact same config, no datastore calls.
		tc.Add(3 * time.Minute)
		q, err = p.Queryable(base)
		So(err, ShouldBeNil)
		So(q.(*queryableForm), ShouldEqual, qf)
		So(counter.GetMulti.Total(), ShouldEqual, 4)

		// 2 min after that, the datastore is rechecked, since the local cache has
		// expired. No new configs there, exact same 'q' is returned.
		tc.Add(2 * time.Minute)
		q, err = p.Queryable(base)
		So(err, ShouldBeNil)
		So(q.(*queryableForm), ShouldEqual, qf)
		So(counter.GetMulti.Total(), ShouldEqual, 5) // datastore call!

		// Config has been updated.
		c2 := prepareServiceConfig(base, map[string]string{
			"config.cfg": "seconds: 6789",
		})
		rev2, err := p.ImportConfigs(c2)
		So(err, ShouldBeNil)

		// Currently cached config is still fresh though, so it is still used.
		q, err = p.Queryable(base)
		So(err, ShouldBeNil)
		So(q.ConfigRevision(), ShouldEqual, rev1)

		// 5 minutes later the cached copy expires and new one is fetched from
		// the datastore.
		tc.Add(5 * time.Minute)
		q, err = p.Queryable(base)
		So(err, ShouldBeNil)
		So(q.ConfigRevision(), ShouldEqual, rev2)
		qf = q.(*queryableForm)
		So(qf.bundle["config.cfg"], ShouldResemble, &timestamp.Timestamp{Seconds: 6789})
	})
}
