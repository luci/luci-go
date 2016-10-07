// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock/testclock"
	milo "github.com/luci/luci-go/milo/api/proto"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestGRPC(t *testing.T) {
	c := memory.Use(context.Background())
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)

	Convey(`A test environment`, t, func() {
		// Add in a public master to satisfy acl.
		name := "testmaster"
		bname := "testbuilder"
		me := &buildbotMasterEntry{Name: name, Internal: false}
		ds.Put(c, me)
		ds.GetTestable(c).Consistent(true)
		ds.GetTestable(c).AutoIndex(true)

		Convey(`Get finished builds`, func() {
			// Add in some builds.
			for i := 0; i < 5; i++ {
				ds.Put(c, &buildbotBuild{
					Master:      name,
					Buildername: bname,
					Number:      i,
					Finished:    true,
				})
			}
			ds.Put(c, &buildbotBuild{
				Master:      name,
				Buildername: bname,
				Number:      6,
				Finished:    false,
			})
			ds.GetTestable(c).CatchupIndexes()

			svc := Service{}
			r := &milo.BuildbotBuildsRequest{
				Master:  name,
				Builder: bname,
			}
			result, err := svc.GetBuildbotBuildsJSON(c, r)
			So(err, ShouldBeNil)
			So(len(result.Builds), ShouldEqual, 5)

			Convey(`Also get incomplete builds`, func() {
				r := &milo.BuildbotBuildsRequest{
					Master:         name,
					Builder:        bname,
					IncludeCurrent: true,
				}
				result, err := svc.GetBuildbotBuildsJSON(c, r)
				So(err, ShouldBeNil)
				So(len(result.Builds), ShouldEqual, 6)
			})
		})
	})
}
