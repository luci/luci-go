// Copyright 2020 The LUCI Authors.
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

package notify

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/logging/memlogger"
	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/config"

	. "github.com/smartystreets/goconvey/convey"
)

// fakeTreeStatusClient simulates the behaviour of a real tree status instance,
// but locally, in-memory.
type fakeTreeStatusClient struct {
	statusForHosts map[string]treeStatus
	nextKey        int
	mtx            sync.Mutex
}

func (ts *fakeTreeStatusClient) getStatus(c context.Context, host string) (*treeStatus, error) {
	ts.mtx.Lock()
	defer ts.mtx.Unlock()

	status, exists := ts.statusForHosts[host]
	if exists {
		return &status, nil
	}
	return nil, errors.New(fmt.Sprintf("No status for host %s", host))
}

func (ts *fakeTreeStatusClient) putStatus(c context.Context, host, message, prevKey string) error {
	ts.mtx.Lock()
	defer ts.mtx.Unlock()

	currStatus, exists := ts.statusForHosts[host]
	if exists && currStatus.key != prevKey {
		return errors.New(fmt.Sprintf(
			"prevKey %q passed to putStatus doesn't match previously stored key %q",
			prevKey, currStatus.key))
	}

	key := ts.nextKey
	ts.nextKey++

	var status config.TreeCloserStatus
	if strings.Contains(message, "close") {
		status = config.Closed
	} else {
		status = config.Open
	}

	ts.statusForHosts[host] = treeStatus{
		"buildbot@chromium.org", message, strconv.Itoa(key), status, time.Now(),
	}
	return nil
}

func TestUpdateTrees(t *testing.T) {
	Convey("Test environment", t, func() {
		c := gaetesting.TestingContextWithAppID("luci-notify-test")
		datastore.GetTestable(c).Consistent(true)
		c = memlogger.Use(c)

		project := &config.Project{Name: "chromium"}
		projectKey := datastore.KeyForObj(c, project)
		builder1 := &config.Builder{ProjectKey: projectKey, ID: "ci/builder1"}
		builder2 := &config.Builder{ProjectKey: projectKey, ID: "ci/builder2"}
		builder3 := &config.Builder{ProjectKey: projectKey, ID: "ci/builder3"}
		builder4 := &config.Builder{ProjectKey: projectKey, ID: "ci/builder4"}
		So(datastore.Put(c, project, builder1, builder2, builder3, builder4), ShouldBeNil)

		earlierTime := time.Now().AddDate(-1, 0, 0)

		cleanup := func() {
			var treeClosers []*config.TreeCloser
			So(datastore.GetAll(c, datastore.NewQuery("TreeClosers"), &treeClosers), ShouldBeNil)
			datastore.Delete(c, treeClosers)
		}

		// Helper function for basic tests. Sets an initial tree state, adds two tree closers
		// for the tree, and checks that updateTrees sets the tree to the correct state.
		testUpdateTrees := func(initialTreeStatus, builder1Status, builder2Status, expectedStatus config.TreeCloserStatus) {
			var statusMessage string
			if initialTreeStatus == config.Open {
				statusMessage = "Open for business"
			} else {
				statusMessage = "Closed up"
			}
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium-status.appspot.com": treeStatus{
						username:  botUsername,
						message:   statusMessage,
						key:       "key",
						status:    initialTreeStatus,
						timestamp: earlierTime,
					},
				},
			}

			So(datastore.Put(c, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder1),
				TreeStatusHost: "chromium-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         builder1Status,
				Timestamp:      time.Now().UTC(),
			}), ShouldBeNil)
			So(datastore.Put(c, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder2),
				TreeStatusHost: "chromium-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         builder2Status,
				Timestamp:      time.Now().UTC(),
			}), ShouldBeNil)
			defer cleanup()

			So(updateTrees(c, &ts), ShouldBeNil)

			status, err := ts.getStatus(c, "chromium-status.appspot.com")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, expectedStatus)
		}

		Convey("Open, both TCs failing, closes", func() {
			testUpdateTrees(config.Open, config.Closed, config.Closed, config.Closed)
		})

		Convey("Open, 1 failing & 1 passing TC, closes", func() {
			testUpdateTrees(config.Open, config.Closed, config.Open, config.Closed)
		})

		Convey("Open, both TCs passing, stays open", func() {
			testUpdateTrees(config.Open, config.Open, config.Open, config.Open)
		})

		Convey("Closed, both TCs failing, stays closed", func() {
			testUpdateTrees(config.Closed, config.Closed, config.Closed, config.Closed)
		})

		Convey("Closed, 1 failing & 1 passing TC, stays closed", func() {
			testUpdateTrees(config.Closed, config.Closed, config.Open, config.Closed)
		})

		Convey("Closed, both TCs, stays closed", func() {
			testUpdateTrees(config.Closed, config.Closed, config.Open, config.Closed)
		})

		Convey("Closed manually, doesn't re-open", func() {
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium-status.appspot.com": treeStatus{
						username:  "somedev@chromium.org",
						message:   "Closed because of reasons",
						key:       "key",
						status:    config.Closed,
						timestamp: earlierTime,
					},
				},
			}

			So(datastore.Put(c, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder1),
				TreeStatusHost: "chromium-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Open,
				Timestamp:      time.Now().UTC(),
			}), ShouldBeNil)
			defer cleanup()

			So(updateTrees(c, &ts), ShouldBeNil)

			status, err := ts.getStatus(c, "chromium-status.appspot.com")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Closed)
		})

		Convey("Opened manually, still closes", func() {
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium-status.appspot.com": treeStatus{
						username:  "somedev@chromium.org",
						message:   "Opened, because I feel like it",
						key:       "key",
						status:    config.Open,
						timestamp: earlierTime,
					},
				},
			}

			So(datastore.Put(c, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder1),
				TreeStatusHost: "chromium-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Closed,
				Timestamp:      time.Now().UTC(),
			}), ShouldBeNil)
			defer cleanup()

			So(updateTrees(c, &ts), ShouldBeNil)

			status, err := ts.getStatus(c, "chromium-status.appspot.com")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Closed)
		})

		Convey("Multiple trees", func() {
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium-status.appspot.com": treeStatus{
						username:  botUsername,
						message:   "Closed up",
						key:       "key",
						status:    config.Closed,
						timestamp: earlierTime,
					},
					"v8-status.appspot.com": treeStatus{
						username:  botUsername,
						message:   "Open for business",
						key:       "key",
						status:    config.Open,
						timestamp: earlierTime,
					},
				},
			}

			So(datastore.Put(c, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder1),
				TreeStatusHost: "chromium-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Open,
				Timestamp:      time.Now().UTC(),
			}), ShouldBeNil)
			So(datastore.Put(c, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder2),
				TreeStatusHost: "chromium-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Open,
				Timestamp:      time.Now().UTC(),
			}), ShouldBeNil)
			So(datastore.Put(c, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder3),
				TreeStatusHost: "chromium-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Open,
				Timestamp:      time.Now().UTC(),
			}), ShouldBeNil)

			So(datastore.Put(c, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder2),
				TreeStatusHost: "v8-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Open,
				Timestamp:      time.Now().UTC(),
			}), ShouldBeNil)
			So(datastore.Put(c, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder3),
				TreeStatusHost: "v8-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Open,
				Timestamp:      time.Now().UTC(),
			}), ShouldBeNil)
			So(datastore.Put(c, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder4),
				TreeStatusHost: "v8-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Closed,
				Timestamp:      time.Now().UTC(),
			}), ShouldBeNil)

			defer cleanup()

			So(updateTrees(c, &ts), ShouldBeNil)

			status, err := ts.getStatus(c, "chromium-status.appspot.com")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Open)

			status, err = ts.getStatus(c, "v8-status.appspot.com")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Closed)
		})
	})
}
