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
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	grpc "google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	tspb "go.chromium.org/luci/tree_status/proto/v1"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/common"
	"go.chromium.org/luci/luci_notify/config"

	. "github.com/smartystreets/goconvey/convey"
)

// fakeTreeStatusClient simulates the behaviour of a real tree status instance,
// but locally, in-memory.
type fakeTreeStatusClient struct {
	statusForHosts map[string]treeStatus
	mu             sync.Mutex
}

func (ts *fakeTreeStatusClient) getStatus(c context.Context, treeName string) (*treeStatus, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	status, exists := ts.statusForHosts[treeName]
	if exists {
		return &status, nil
	}
	return nil, errors.New(fmt.Sprintf("No status for treeName %s", treeName))
}

func (ts *fakeTreeStatusClient) postStatus(c context.Context, message string, treeName string, status config.TreeCloserStatus, closingBuilderName string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	var messageStatus config.TreeCloserStatus
	if strings.Contains(message, "close") {
		messageStatus = config.Closed
	} else {
		messageStatus = config.Open
	}
	if messageStatus != status {
		return errors.Reason("message status does not match provided status").Err()
	}

	ts.statusForHosts[treeName] = treeStatus{
		username:           "buildbot@chromium.org",
		message:            message,
		status:             status,
		timestamp:          time.Now(),
		closingBuilderName: closingBuilderName,
	}
	return nil
}

func TestUpdateTrees(t *testing.T) {
	Convey("Test environment", t, func() {
		c := memory.Use(context.Background())
		c = common.SetAppIDForTest(c, "luci-notify-test")

		datastore.GetTestable(c).Consistent(true)
		c = memlogger.Use(c)
		log := logging.Get(c).(*memlogger.MemLogger)

		project1 := &config.Project{Name: "chromium", TreeClosingEnabled: true}
		project1Key := datastore.KeyForObj(c, project1)
		builder1 := &config.Builder{ProjectKey: project1Key, ID: "ci/builder1"}
		builder2 := &config.Builder{ProjectKey: project1Key, ID: "ci/builder2"}
		builder3 := &config.Builder{ProjectKey: project1Key, ID: "ci/builder3"}
		builder4 := &config.Builder{ProjectKey: project1Key, ID: "ci/builder4"}
		builder7 := &config.Builder{ProjectKey: project1Key, ID: "buildbucket/dart/luci.dart.ci/dart-sdk-win"}

		project2 := &config.Project{Name: "infra", TreeClosingEnabled: false}
		project2Key := datastore.KeyForObj(c, project2)
		builder5 := &config.Builder{ProjectKey: project2Key, ID: "ci/builder5"}
		builder6 := &config.Builder{ProjectKey: project2Key, ID: "ci/builder6"}

		So(datastore.Put(c, project1, builder1, builder2, builder3, builder4, project2, builder5, builder6), ShouldBeNil)

		earlierTime := time.Now().AddDate(-1, 0, 0).UTC()
		evenEarlierTime := time.Now().AddDate(-2, 0, 0).UTC()

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
					"chromium": {
						username:  botUsername,
						message:   statusMessage,
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

			status, err := ts.getStatus(c, "chromium")
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
					"chromium": {
						username:  "somedev@chromium.org",
						message:   "Closed because of reasons",
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

			status, err := ts.getStatus(c, "chromium")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Closed)
		})

		Convey("Opened manually, stays open with no new failures", func() {
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium": {
						username:  "somedev@chromium.org",
						message:   "Opened, because I feel like it",
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
				Timestamp:      evenEarlierTime,
			}), ShouldBeNil)
			defer cleanup()

			So(updateTrees(c, &ts), ShouldBeNil)

			status, err := ts.getStatus(c, "chromium")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Open)
			So(status.message, ShouldEqual, "Opened, because I feel like it")
		})

		Convey("Opened manually, closes on new failure", func() {
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium": {
						username:  "somedev@chromium.org",
						message:   "Opened, because I feel like it",
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

			status, err := ts.getStatus(c, "chromium")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Closed)
			So(status.closingBuilderName, ShouldEqual, "projects/chromium/buckets/ci/builders/builder1")
		})

		Convey("Opened manually, closes on new failure, with old builder ID", func() {
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium": {
						username:  "somedev@chromium.org",
						message:   "Opened, because I feel like it",
						status:    config.Open,
						timestamp: earlierTime,
					},
				},
			}

			So(datastore.Put(c, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder7),
				TreeStatusHost: "chromium-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Closed,
				Timestamp:      time.Now().UTC(),
			}), ShouldBeNil)
			defer cleanup()

			So(updateTrees(c, &ts), ShouldBeNil)

			status, err := ts.getStatus(c, "chromium")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Closed)
			So(status.closingBuilderName, ShouldEqual, "")
		})

		Convey("Multiple trees", func() {
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium": {
						username:  botUsername,
						message:   "Closed up",
						status:    config.Closed,
						timestamp: evenEarlierTime,
					},
					"v8": {
						username:  botUsername,
						message:   "Open for business",
						status:    config.Open,
						timestamp: evenEarlierTime,
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
				Timestamp:      earlierTime,
				Message:        "Correct message",
			}), ShouldBeNil)

			defer cleanup()

			So(updateTrees(c, &ts), ShouldBeNil)

			status, err := ts.getStatus(c, "chromium")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Open)
			So(status.message, ShouldStartWith, "Tree is open (Automatic: ")

			status, err = ts.getStatus(c, "v8")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Closed)
			So(status.message, ShouldEqual, "Tree is closed (Automatic: Correct message)")
			So(status.closingBuilderName, ShouldEqual, "projects/chromium/buckets/ci/builders/builder4")
		})

		Convey("Doesn't close when build is older than last status update", func() {
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium": {
						username:  "somedev@chromium.org",
						message:   "Opened, because I feel like it",
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
				Timestamp:      evenEarlierTime,
			}), ShouldBeNil)
			defer cleanup()

			So(updateTrees(c, &ts), ShouldBeNil)

			status, err := ts.getStatus(c, "chromium")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Open)
			So(status.message, ShouldEqual, "Opened, because I feel like it")
		})

		Convey("Doesn't open when build is older than last status update", func() {
			// This test replicates the likely state just after we've
			// automatically closed the tree: the tree is closed with
			// our username, and there is some failing TreeCloser older
			// than the status update.
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium": {
						username:  botUsername,
						message:   "Tree is closed (Automatic: some builder failed)",
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
				Timestamp:      evenEarlierTime,
			}, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder2),
				TreeStatusHost: "chromium-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Closed,
				Timestamp:      evenEarlierTime,
			}), ShouldBeNil)
			defer cleanup()

			So(updateTrees(c, &ts), ShouldBeNil)

			status, err := ts.getStatus(c, "chromium")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Closed)
			So(status.message, ShouldEqual, "Tree is closed (Automatic: some builder failed)")
		})

		Convey("Doesn't open when a builder is still failing", func() {
			// This test replicates the likely state after we've automatically
			// closed the tree, but some other builder has had a successful
			// build.
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium": {
						username:  botUsername,
						message:   "Tree is closed (Automatic: some builder failed)",
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
			}, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder2),
				TreeStatusHost: "chromium-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Closed,
				Timestamp:      evenEarlierTime,
			}), ShouldBeNil)
			defer cleanup()

			So(updateTrees(c, &ts), ShouldBeNil)

			status, err := ts.getStatus(c, "chromium")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Closed)
			So(status.message, ShouldEqual, "Tree is closed (Automatic: some builder failed)")
		})

		Convey("Multiple projects", func() {
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium": {
						username:  botUsername,
						message:   "Tree is closed (Automatic: some builder failed)",
						status:    config.Closed,
						timestamp: earlierTime,
					},
					"infra": {
						username:  botUsername,
						message:   "Tree is open (Automatic: Yes!)",
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
			}, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder5),
				TreeStatusHost: "infra-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Closed,
				Timestamp:      time.Now().UTC(),
				Message:        "Close it up!",
			}), ShouldBeNil)
			defer cleanup()

			So(updateTrees(c, &ts), ShouldBeNil)

			status, err := ts.getStatus(c, "chromium")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Open)
			So(status.message, ShouldStartWith, "Tree is open (Automatic: ")

			status, err = ts.getStatus(c, "infra")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Open)
			So(status.message, ShouldStartWith, "Tree is open (Automatic: ")

			hasExpectedLog := false
			for _, log := range log.Messages() {
				if log.Level == logging.Info {
					hasExpectedLog = true
					So(log.Msg, ShouldEqual, `Would update status for infra to "Tree is closed (Automatic: Close it up!)"`)
				}
			}

			So(hasExpectedLog, ShouldBeTrue)
		})

		Convey("Multiple projects, overlapping tree status hosts", func() {
			ts := fakeTreeStatusClient{
				statusForHosts: map[string]treeStatus{
					"chromium": {
						username:  botUsername,
						message:   "Tree is open (Flake)",
						status:    config.Open,
						timestamp: earlierTime,
					},
					"infra": {
						username:  botUsername,
						message:   "Tree is closed (Automatic: Some builder failed)",
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
			}, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder5),
				TreeStatusHost: "chromium-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Closed,
				Timestamp:      time.Now().UTC(),
			}, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder2),
				TreeStatusHost: "infra-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Open,
				Timestamp:      time.Now().UTC(),
			}, &config.TreeCloser{
				BuilderKey:     datastore.KeyForObj(c, builder6),
				TreeStatusHost: "infra-status.appspot.com",
				TreeCloser:     notifypb.TreeCloser{},
				Status:         config.Closed,
				Timestamp:      time.Now().UTC(),
			}), ShouldBeNil)
			defer cleanup()

			So(updateTrees(c, &ts), ShouldBeNil)

			status, err := ts.getStatus(c, "chromium")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Open)
			So(status.message, ShouldEqual, "Tree is open (Flake)")

			status, err = ts.getStatus(c, "infra")
			So(err, ShouldBeNil)
			So(status.status, ShouldEqual, config.Open)
			So(status.message, ShouldStartWith, "Tree is open (Automatic: ")
		})
	})
}

func TestHttpTreeStatusClient(t *testing.T) {
	Convey("Test environment for httpTreeStatusClient", t, func() {
		c := memory.Use(context.Background())
		c = common.SetAppIDForTest(c, "luci-notify-test")

		fakePrpcClient := &fakePRPCTreeStatusClient{}
		ts := httpTreeStatusClient{fakePrpcClient}

		Convey("getStatus, open tree", func() {
			status, err := ts.getStatus(c, "chromium-status.appspot.com")
			So(err, ShouldBeNil)

			expectedTime := time.Date(2020, time.March, 31, 5, 33, 52, 682351000, time.UTC)
			So(status, ShouldResemble, &treeStatus{
				username:  "someone@google.com",
				message:   "Tree is throttled (win rel 32 appears to be a goma flake. the other builds seem to be charging ahead OK. will fully open / fully close if win32 does/doesn't improve)",
				status:    config.Closed,
				timestamp: expectedTime,
			})
		})

		Convey("getStatus, closed tree", func() {
			status, err := ts.getStatus(c, "v8-status.appspot.com")
			So(err, ShouldBeNil)

			expectedTime := time.Date(2020, time.April, 2, 15, 21, 39, 981072000, time.UTC)
			So(status, ShouldResemble, &treeStatus{
				username:  "someone-else@google.com",
				message:   "open (flake?)",
				status:    config.Open,
				timestamp: expectedTime,
			})
		})

		Convey("postStatus", func() {
			err := ts.postStatus(c, "open for business", "dart", config.Open, "projects/chromium/buckets/bucket/builders/builder")
			So(err, ShouldBeNil)

			So(fakePrpcClient.latestStatus, ShouldNotBeNil)
			So(fakePrpcClient.latestStatus.Message, ShouldEqual, "open for business")
			So(fakePrpcClient.latestStatus.GeneralState, ShouldEqual, tspb.GeneralState_OPEN)
			So(fakePrpcClient.latestStatus.ClosingBuilderName, ShouldEqual, "projects/chromium/buckets/bucket/builders/builder")
		})
	})
}

type fakePRPCTreeStatusClient struct {
	latestStatus *tspb.Status
}

func (c *fakePRPCTreeStatusClient) ListStatus(ctx context.Context, in *tspb.ListStatusRequest, opts ...grpc.CallOption) (*tspb.ListStatusResponse, error) {
	return nil, errors.Reason("Not implemented").Err()
}

func (c *fakePRPCTreeStatusClient) GetStatus(ctx context.Context, in *tspb.GetStatusRequest, opts ...grpc.CallOption) (*tspb.Status, error) {
	if c.latestStatus != nil {
		return c.latestStatus, nil
	}
	if strings.HasPrefix(in.Name, "trees/v8-") {
		return &tspb.Status{
			Name:         in.Name,
			GeneralState: tspb.GeneralState_OPEN,
			Message:      "open (flake?)",
			CreateUser:   "someone-else@google.com",
			CreateTime:   timestamppb.New(time.Date(2020, time.April, 2, 15, 21, 39, 981072000, time.UTC)),
		}, nil
	}
	return &tspb.Status{
		Name:         "trees/chromium/status/fallback",
		GeneralState: tspb.GeneralState_CLOSED,
		Message:      "Tree is throttled (win rel 32 appears to be a goma flake. the other builds seem to be charging ahead OK. will fully open / fully close if win32 does/doesn't improve)",
		CreateUser:   "someone@google.com",
		CreateTime:   timestamppb.New(time.Date(2020, time.March, 31, 5, 33, 52, 682351000, time.UTC)),
	}, nil
}

func (c *fakePRPCTreeStatusClient) CreateStatus(ctx context.Context, in *tspb.CreateStatusRequest, opts ...grpc.CallOption) (*tspb.Status, error) {
	c.latestStatus = in.Status
	return in.Status, nil
}
