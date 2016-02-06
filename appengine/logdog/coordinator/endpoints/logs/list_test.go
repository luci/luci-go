// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"fmt"
	"testing"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/appengine/logdog/coordinator/hierarchy"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func listPaths(l *logs.ListResponse) []string {
	s := make([]string, len(l.Components))
	for i, c := range l.Components {
		s[i] = c.Path
		if c.Stream {
			s[i] += "$"
		}
	}
	return s
}

func TestList(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration, a List request`, t, func() {
		c, _ := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)
		c, _ = featureBreaker.FilterRDS(c, nil)

		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		c = ct.UseConfig(c, &svcconfig.Coordinator{
			AdminAuthGroup: "test-administrators",
		})

		s := Server{}
		req := logs.ListRequest{}

		// Install a set of stock log streams to query against.
		streams := map[string]*coordinator.LogStream{}
		descs := map[string]*logpb.LogStreamDescriptor{}
		for i, v := range []types.StreamPath{
			"testing/+/foo",
			"testing/+/foo/bar",
			"other/+/foo/bar",
			"other/+/baz",
			"purged/+/foo",
		} {
			prefix, name := v.Split()
			desc := ct.TestLogStreamDescriptor(c, string(name))
			desc.Prefix = string(prefix)

			ls, err := ct.TestLogStream(c, desc)
			if err != nil {
				panic(fmt.Errorf("failed to generate log stream %d: %v", i, err))
			}

			if err := hierarchy.Put(ds.Get(c), ls); err != nil {
				panic(fmt.Errorf("failed to put log stream %d: %v", i, err))
			}

			if prefix.Segments()[0] == "purged" {
				if err := hierarchy.Purge(ds.Get(c), ls.Path(), true); err != nil {
					panic(fmt.Errorf("failed to purge log stream %d: %v", i, err))
				}
			}

			descs[string(v)] = desc
			streams[string(v)] = ls
		}
		ds.Get(c).Testable().CatchupIndexes()

		Convey(`A default list request will return top-level entries.`, func() {
			l, err := s.List(c, &req)
			So(err, ShouldBeRPCOK)
			So(listPaths(l), ShouldResembleV, []string{"other", "purged", "testing"})
		})

		Convey(`A recursive list will return all elements iteratively.`, func() {
			req.Recursive = true
			req.MaxResults = 4

			for _, round := range [][]string{
				{"other", "other/+", "other/+/baz$", "other/+/foo"},
				{"other/+/foo/bar$", "purged", "purged/+", "testing"},
				{"testing/+", "testing/+/foo$", "testing/+/foo", "testing/+/foo/bar$"},
				{},
			} {
				l, err := s.List(c, &req)
				So(err, ShouldBeRPCOK)
				So(listPaths(l), ShouldResembleV, round)

				if len(round) < int(req.MaxResults) {
					So(l.Next, ShouldEqual, "")
					break
				}

				So(l.Next, ShouldNotEqual, "")
				req.Next = l.Next
			}
		})

		Convey(`A list including purged will fail for an unanthenticated user.`, func() {
			req.IncludePurged = true

			_, err := s.List(c, &req)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as an administrator`, func() {
			fs.IdentityGroups = []string{"test-administrators"}

			Convey(`A list including purged will include purged elements.`, func() {
				req.StreamOnly = true
				req.Recursive = true
				req.IncludePurged = true

				l, err := s.List(c, &req)
				So(err, ShouldBeRPCOK)
				So(listPaths(l), ShouldResembleV, []string{
					"other/+/baz$",
					"other/+/foo/bar$",
					"purged/+/foo$",
					"testing/+/foo$",
					"testing/+/foo/bar$",
				})
			})
		})
	})
}
