// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/gaetesting"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type testPurgeStorage struct {
	storage.Storage

	err    error
	purged types.StreamPath
	closed bool
}

func (s *testPurgeStorage) Purge(p types.StreamPath) error {
	s.purged = p
	return s.err
}

func (s *testPurgeStorage) Close() {
	s.closed = true
}

func TestHandleStorageCleanup(t *testing.T) {
	t.Parallel()

	Convey(`A testing setup`, t, func() {
		c := gaetesting.TestingContext()
		c, tc := testclock.UseTime(c, testclock.TestTimeUTC)

		st := testPurgeStorage{}
		b := Backend{
			s: coordinator.Service{
				StorageFunc: func(context.Context) (storage.Storage, error) {
					return &st, nil
				},
			},
		}

		tb := testBase{Context: c}
		r := httprouter.New()
		b.InstallHandlers(r, tb.base)

		s := httptest.NewServer(r)
		defer s.Close()

		ls := ct.TestLogStream(c, ct.TestLogStreamDescriptor(c, "foo"))
		ls.State = coordinator.LSArchived
		So(ls.Put(ds.Get(c)), ShouldBeNil)
		tc.Add(time.Second)

		Convey(`Will succeed, deleting the task, if the stream path is missing.`, func() {
			resp, err := http.PostForm(fmt.Sprintf("%s/archive/cleanup", s.URL), nil)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
		})

		Convey(`Will succeed, deleting the task, if invalid stream path.`, func() {
			resp, err := http.PostForm(fmt.Sprintf("%s/archive/cleanup", s.URL), mkValues(map[string]string{
				"path": "!!!INVALID PATH!!!",
			}))
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
		})

		Convey(`Will error if a storage instance could not be obtained.`, func() {
			b.s.StorageFunc = func(context.Context) (storage.Storage, error) {
				return nil, errors.New("test error")
			}

			resp, err := http.PostForm(fmt.Sprintf("%s/archive/cleanup", s.URL), mkValues(map[string]string{
				"path": "testing/+/foo",
			}))
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)

			So(ds.Get(c).Get(ls), ShouldBeNil)
			So(ls.State, ShouldEqual, coordinator.LSArchived)
		})

		Convey(`Will successfully clean up the stream.`, func() {
			resp, err := http.PostForm(fmt.Sprintf("%s/archive/cleanup", s.URL), mkValues(map[string]string{
				"path": "testing/+/foo",
			}))
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
			So(st.purged, ShouldEqual, "testing/+/foo")
			So(st.closed, ShouldBeTrue)

			So(ds.Get(c).Get(ls), ShouldBeNil)
			So(ls.State, ShouldEqual, coordinator.LSDone)
			So(ls.Updated, ShouldResemble, ds.RoundTime(clock.Now(c).UTC()))
		})

		Convey(`Will return an error if the purge operaetion failed.`, func() {
			st.err = errors.New("test error")

			resp, err := http.PostForm(fmt.Sprintf("%s/archive/cleanup", s.URL), mkValues(map[string]string{
				"path": "testing/+/foo",
			}))
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)
			So(st.closed, ShouldBeTrue)

			So(ds.Get(c).Get(ls), ShouldBeNil)
			So(ls.State, ShouldEqual, coordinator.LSArchived)
		})
	})
}
