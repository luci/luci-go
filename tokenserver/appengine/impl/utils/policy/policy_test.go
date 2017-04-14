// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package policy

import (
	"testing"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/luci/luci-go/appengine/gaetesting"
	"github.com/luci/luci-go/common/config/validation"

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
		// TODO
	})
}
