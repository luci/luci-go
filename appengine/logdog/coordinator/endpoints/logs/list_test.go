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
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func listPaths(l *logdog.ListResponse) []string {
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

		svcStub := ct.Services{}
		svcStub.InitConfig()
		svcStub.ServiceConfig.Coordinator.AdminAuthGroup = "test-administrators"
		c = coordinator.WithServices(c, &svcStub)

		svc := New()

		// di is a datastore bound to the test project namespace.
		const project = "test-project"
		if err := coordinator.WithProjectNamespace(&c, config.ProjectName(project)); err != nil {
			panic(err)
		}
		di := ds.Get(c)

		req := logdog.ListRequest{
			Project: project,
		}

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

			ls := ct.TestLogStream(c, desc)
			if err := hierarchy.Put(di, ls.Path()); err != nil {
				panic(fmt.Errorf("failed to put log stream %d: %v", i, err))
			}

			if prefix.Segments()[0] == "purged" {
				if err := hierarchy.MarkPurged(di, ls.Path(), true); err != nil {
					panic(fmt.Errorf("failed to purge log stream %d: %v", i, err))
				}
			}

			descs[string(v)] = desc
			streams[string(v)] = ls
		}
		di.Testable().CatchupIndexes()

		Convey(`A default list request will return top-level entries.`, func() {
			l, err := svc.List(c, &req)
			So(err, ShouldBeRPCOK)
			So(listPaths(l), ShouldResemble, []string{"other", "purged", "testing"})
		})

		Convey(`If the project does not exist, will return nothing.`, func() {
			req.Project = "does-not-exist"

			l, err := svc.List(c, &req)
			So(err, ShouldBeRPCOK)
			So(listPaths(l), ShouldResemble, []string{})
		})

		Convey(`Will skip elements if requested.`, func() {
			req.Offset = int32(2)
			l, err := svc.List(c, &req)
			So(err, ShouldBeRPCOK)
			So(listPaths(l), ShouldResemble, []string{"testing"})
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
				l, err := svc.List(c, &req)
				So(err, ShouldBeRPCOK)
				So(listPaths(l), ShouldResemble, round)

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

			_, err := svc.List(c, &req)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as an administrator`, func() {
			fs.IdentityGroups = []string{"test-administrators"}

			Convey(`A list including purged will include purged elements.`, func() {
				req.StreamOnly = true
				req.Recursive = true
				req.IncludePurged = true

				l, err := svc.List(c, &req)
				So(err, ShouldBeRPCOK)
				So(listPaths(l), ShouldResemble, []string{
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
