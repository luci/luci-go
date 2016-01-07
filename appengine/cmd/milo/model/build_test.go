// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

var _ = fmt.Print

func TestBuildRoot(t *testing.T) {
	t.Parallel()

	Convey("Build", t, func() {
		c := memory.Use(context.Background())
		ds := datastore.Get(c)
		buildRootIdx := datastore.IndexDefinition{
			Kind:     "Build",
			Ancestor: true,
			SortBy: []datastore.IndexColumn{
				{
					Property:   "revisions.generation",
					Descending: true,
				},
			},
		}
		ds.Testable().AddIndexes(&buildRootIdx)

		b := GetBuildRoot(c, "master.chromium.linux", "linux_android_rel_ng")

		// Subtract 7 seconds because testclock has nanoseconds, and we ignore
		// nano-seconds in the datastore (only store milliseconds).
		t := TimeID{
			Time: testclock.TestTimeUTC.Add(-time.Nanosecond * 7),
		}

		build := &Build{
			ExecutionTime: t,
			BuildRoot:     b.Key,
			Revisions: []RevisionInfo{
				{Generation: 1},
			},
		}

		build2 := &Build{
			ExecutionTime: TimeID{t.Add(time.Second)},
			BuildRoot:     b.Key,
			Revisions: []RevisionInfo{
				{Generation: 2},
			},
		}

		So(ds.Put(build), ShouldBeNil)
		So(ds.Put(build2), ShouldBeNil)
		ds.Testable().CatchupIndexes()

		Convey("BuildRoot", func() {
			Convey("GetBuilds", func() {
				q := b.GetBuilds()

				Convey("returns a valid query", func() {
					builds := make([]*Build, 0, 2)
					So(ds.GetAll(q, &builds), ShouldBeNil)
				})
			})
		})

		Convey("GetBuilds", func() {
			b2 := GetBuildRoot(c, "master.foo", "bar_baz_bam")

			otherBuild := &Build{
				ExecutionTime: t,
				BuildRoot:     b2.Key,
				Revisions: []RevisionInfo{
					{Generation: 3},
				},
			}
			So(ds.Put(otherBuild), ShouldBeNil)

			sources := []BuildRoot{b, b2}
			Convey("less than total", func() {
				rslt, err := GetBuilds(c, sources, 1)
				So(err, ShouldBeNil)

				So(rslt, ShouldResembleV, [][]*Build{
					{build2},
					{otherBuild},
				})
			})

			Convey("total", func() {
				rslt, err := GetBuilds(c, sources, 2)
				So(err, ShouldBeNil)

				So(rslt, ShouldResembleV, [][]*Build{
					{build2, build},
					{otherBuild},
				})
			})
		})
	})
}
