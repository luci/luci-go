// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package janitor

import (
	"errors"
	"testing"

	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

// testServicesClient implements logdog.ServicesClient sufficient for testing
// and instrumentation.
type testServicesClient struct {
	logdog.ServicesClient

	lsCallback func(*logdog.LoadStreamRequest) (*logdog.LoadStreamResponse, error)
	csCallback func(*logdog.CleanupStreamRequest) error
}

func (sc *testServicesClient) LoadStream(c context.Context, req *logdog.LoadStreamRequest, o ...grpc.CallOption) (
	*logdog.LoadStreamResponse, error) {
	if cb := sc.lsCallback; cb != nil {
		return cb(req)
	}
	return nil, errors.New("no callback implemented")
}

func (sc *testServicesClient) CleanupStream(c context.Context, req *logdog.CleanupStreamRequest, o ...grpc.CallOption) (
	*google.Empty, error) {
	if cb := sc.csCallback; cb != nil {
		if err := cb(req); err != nil {
			return nil, err
		}
	}
	return &google.Empty{}, nil
}

type testPurgeStorage struct {
	storage.Storage

	err    error
	purged types.StreamPath
}

func (s *testPurgeStorage) Purge(p types.StreamPath) error {
	s.purged = p
	return s.err
}

func TestHandlePurge(t *testing.T) {
	t.Parallel()

	Convey(`A testing setup`, t, func() {
		c, _ := testclock.UseTime(context.Background(), testclock.TestTimeUTC)

		st := testPurgeStorage{}

		// Set up our test Coordinator client stubs.
		stream := logdog.LoadStreamResponse{
			State: &logdog.LogStreamState{
				Path:          "testing/+/foo",
				ProtoVersion:  logpb.Version,
				TerminalIndex: -1,
				Archived:      false,
				Purged:        false,
			},
		}

		var cleanupRequest *logdog.CleanupStreamRequest
		sc := testServicesClient{
			lsCallback: func(req *logdog.LoadStreamRequest) (*logdog.LoadStreamResponse, error) {
				return &stream, nil
			},
			csCallback: func(req *logdog.CleanupStreamRequest) error {
				cleanupRequest = req
				return nil
			},
		}

		j := Janitor{
			Service: &sc,
			Storage: &st,
		}

		task := logdog.CleanupTask{
			Path: stream.State.Path,
		}

		Convey(`Will refuse to cleanup a non-archived stream.`, func() {
			So(j.Cleanup(c, &task), ShouldErrLike, "log stream is not archived")
		})

		Convey(`For an archived log stream`, func() {
			stream.State.Archived = true

			Convey(`Will successfully clean up the stream.`, func() {
				So(j.Cleanup(c, &task), ShouldBeNil)
				So(st.purged, ShouldEqual, "testing/+/foo")

				So(cleanupRequest, ShouldNotBeNil)
				So(cleanupRequest.Path, ShouldEqual, "testing/+/foo")
			})

			Convey(`Will return an error if the purge operation failed.`, func() {
				st.err = errors.New("test error")

				So(j.Cleanup(c, &task), ShouldErrLike, "test error")

				So(cleanupRequest, ShouldBeNil)
			})

			Convey(`Will return an error if the cleanup RPC failed.`, func() {
				sc.csCallback = func(*logdog.CleanupStreamRequest) error {
					return errors.New("test error")
				}

				So(j.Cleanup(c, &task), ShouldErrLike, "test error")
				So(st.purged, ShouldEqual, "testing/+/foo")
			})
		})
	})
}
