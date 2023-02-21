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

package gerrit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSimpleDeduper(t *testing.T) {
	t.Parallel()

	Convey("listChangesDeduper works", t, func() {
		epoch := time.Date(2020, time.February, 3, 10, 30, 0, 0, time.UTC)
		ci := func(i int64, t time.Time) *gerritpb.ChangeInfo {
			return &gerritpb.ChangeInfo{
				Number:  i,
				Updated: timestamppb.New(t),
			}
		}
		l := listChangesDeduper{}

		a := []*gerritpb.ChangeInfo{
			ci(2, epoch.Add(time.Minute)),
			ci(1, epoch.Add(time.Second)),
		}
		So(l.appendSorted(nil, a), ShouldResembleProto, a)
		So(l.mergeSorted(a, a), ShouldResembleProto, a)

		b := []*gerritpb.ChangeInfo{
			ci(3, epoch.Add(time.Minute+time.Second)),
			ci(1, epoch.Add(time.Minute)),
			ci(4, epoch.Add(time.Second)),
			ci(5, epoch.Add(time.Second)),
		}
		c := l.mergeSorted(a, b)
		So(c, ShouldResembleProto, []*gerritpb.ChangeInfo{
			ci(3, epoch.Add(time.Minute+time.Second)),
			ci(1, epoch.Add(time.Minute)),
			ci(2, epoch.Add(time.Minute)),
			ci(4, epoch.Add(time.Second)),
			ci(5, epoch.Add(time.Second)),
		})

		So(l.appendSorted(c, []*gerritpb.ChangeInfo{
			ci(4, epoch.Add(time.Second)),
			ci(5, epoch.Add(time.Second)),
			ci(6, epoch.Add(time.Second)),
			ci(7, epoch.Add(time.Millisecond)),
		}), ShouldResembleProto, []*gerritpb.ChangeInfo{
			ci(3, epoch.Add(time.Minute+time.Second)),
			ci(1, epoch.Add(time.Minute)),
			ci(2, epoch.Add(time.Minute)),
			ci(4, epoch.Add(time.Second)),
			ci(5, epoch.Add(time.Second)),
			ci(6, epoch.Add(time.Second)),
			ci(7, epoch.Add(time.Millisecond)),
		})
	})
}

func TestPagingListChanges(t *testing.T) {
	t.Parallel()

	Convey("PagingListChanges works", t, func() {
		ctx := context.Background()
		if testing.Verbose() {
			ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
		}
		now := time.Date(2020, time.February, 3, 10, 30, 0, 0, time.UTC)
		makeCI := func(i int64, age time.Duration) *gerritpb.ChangeInfo {
			return &gerritpb.ChangeInfo{
				Number:  i,
				Updated: timestamppb.New(now.Add(-age)),
			}
		}
		const more = true
		const noMore = false
		makeResp := func(moreChanges bool, cs ...*gerritpb.ChangeInfo) *gerritpb.ListChangesResponse {
			return &gerritpb.ListChangesResponse{MoreChanges: moreChanges, Changes: cs}
		}

		fake := fakeListChanges{}
		pager := listChangesPager{
			client: &fake,
			req: &gerritpb.ListChangesRequest{
				Query: "status:new",
			},
			opts: PagingListChangesOptions{
				Limit: 100, // required
			},
		}

		Convey("Happy 1 RPC path", func() {
			pager.opts.Limit = 1
			pager.opts.PageSize = 100
			fake.reset(makeResp(noMore, makeCI(1, time.Second), makeCI(2, 2*time.Second), makeCI(3, 3*time.Second)))
			resp, err := pager.pagingListChanges(ctx)
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, makeResp(more, makeCI(1, time.Second)))
			So(fake.calls, ShouldResembleProto, []*gerritpb.ListChangesRequest{
				{Query: "status:new", Limit: 100},
			})
		})

		Convey("Propagates request and grpc opts", func() {
			pager.opts.Limit = 2
			pager.req.Options = []gerritpb.QueryOption{gerritpb.QueryOption_LABELS}
			pager.grpcOpts = []grpc.CallOption{grpc.MaxCallSendMsgSize(1)}

			fake.expectGrpcOpts = []grpc.CallOption{grpc.MaxCallSendMsgSize(1)}
			fake.reset(makeResp(noMore, makeCI(1, time.Second)))

			resp, err := pager.pagingListChanges(ctx)
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, makeResp(noMore, makeCI(1, time.Second)))
			So(fake.calls, ShouldResembleProto, []*gerritpb.ListChangesRequest{
				{
					Query:   "status:new",
					Options: []gerritpb.QueryOption{gerritpb.QueryOption_LABELS},
					Limit:   100,
					Offset:  0,
				},
			})
		})

		Convey("Empty response is trusted", func() {
			fake.reset(makeResp(noMore))
			resp, err := pager.pagingListChanges(ctx)
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, makeResp(noMore))
		})

		Convey("Doesn't trust MoreChanges=false", func() {
			pager.opts.Limit = 3
			pager.opts.PageSize = 2
			pager.opts.MoreChangesTrustFactor = 0.9
			fake.reset(
				// false should not be trusted here...
				makeResp(noMore, makeCI(1, 1*time.Second), makeCI(2, 2*time.Second), makeCI(3, 3*time.Second)),
				// ... so even older ones are checked, and now it's trusted.
				makeResp(noMore, makeCI(3, 3*time.Second)),
				// finally, check for newer changes, but there is still just #1.
				makeResp(noMore, makeCI(1, 1*time.Second)),
			)
			resp, err := pager.pagingListChanges(ctx)
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, makeResp(noMore,
				makeCI(1, 1*time.Second),
				makeCI(2, 2*time.Second),
				makeCI(3, 3*time.Second),
			))
			// Early fail if initial setup changes, which would also break fake.calls
			// assertion below, which should speed up the debugging time.
			So(FormatTime(now), ShouldEqual, `"2020-02-03 10:30:00.000000000"`)
			So(fake.calls, ShouldResembleProto, []*gerritpb.ListChangesRequest{
				{
					Query: "status:new",
					Limit: 2,
				},
				{
					Query: `status:new before:"2020-02-03 10:29:57.000000000"`, // before:#3.Updated
					Limit: 2,
				},
				{
					Query: `status:new after:"2020-02-03 10:29:59.000000000"`, // after:#1.Updated.
					Limit: 2,
				},
			})
		})

		Convey("Avoid misses due to racy updates", func() {
			pager.opts.Limit = 4
			pager.opts.PageSize = 3
			fake.reset(
				makeResp(more, makeCI(1, 1*time.Second), makeCI(2, 2*time.Second), makeCI(3, 3*time.Second)),
				// Simulate #4 getting updated concurrently, so it's missing from olders
				// changes:
				makeResp(noMore, makeCI(3, 3*time.Second), makeCI(5, 5*time.Second)),
				makeResp(noMore, makeCI(4, 4*time.Millisecond), makeCI(1, 1*time.Second)),
			)
			resp, err := pager.pagingListChanges(ctx)
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, makeResp(more,
				makeCI(4, 4*time.Millisecond),
				makeCI(1, 1*time.Second),
				makeCI(2, 2*time.Second),
				makeCI(3, 3*time.Second),
			))
			So(FormatTime(now), ShouldEqual, `"2020-02-03 10:30:00.000000000"`)
			So(fake.calls, ShouldResembleProto, []*gerritpb.ListChangesRequest{
				{
					Query: "status:new",
					Limit: 3,
				},
				{
					Query: `status:new before:"2020-02-03 10:29:57.000000000"`, // before:#3.Updated
					Limit: 3,
				},
				{
					Query: `status:new after:"2020-02-03 10:29:59.000000000"`, // after:#1.Updated.
					Limit: 3,
				},
			})
		})

		Convey("Return partial result on errors", func() {
			pager.opts.Limit = 4
			pager.opts.PageSize = 3
			fake.reset(
				makeResp(more, makeCI(1, 1*time.Second), makeCI(2, 2*time.Second), makeCI(3, 3*time.Second)),
				status.Errorf(codes.Internal, "boooo"),
			)
			resp, err := pager.pagingListChanges(ctx)
			So(err, ShouldErrLike, "boooo")
			So(resp, ShouldResembleProto, makeResp(more,
				makeCI(1, 1*time.Second),
				makeCI(2, 2*time.Second),
				makeCI(3, 3*time.Second),
			))
		})

		Convey("Bail if ordered by updated DESC assumption doesn't hold", func() {
			pager.opts.Limit = 4
			pager.opts.PageSize = 3
			fake.reset(makeResp(more, makeCI(1, 10*time.Second), makeCI(2, 1*time.Second)))
			_, err := pager.pagingListChanges(ctx)
			So(err, ShouldErrLike, "ListChangesResponse.Changes not ordered by updated timestamp")
		})

		Convey("Bail if too many changes have the same updated timestamp", func() {
			pager.opts.Limit = 4
			pager.opts.PageSize = 2
			fake.reset(
				makeResp(more, makeCI(1, 1*time.Second), makeCI(2, 9*time.Second)),
				makeResp(more, makeCI(2, 9*time.Second), makeCI(3, 9*time.Second)),
				// Strictly speaking, the exact same RPC can be avoided, but such a situation
				// is rare, so let's not complicate code needlessly.
				makeResp(more, makeCI(2, 9*time.Second), makeCI(3, 9*time.Second)),
			)
			resp, err := pager.pagingListChanges(ctx)
			So(err, ShouldErrLike, `PagingListChanges stuck on query:"status:new before:\"2020-02-03 10:29:51.000000000\""`)
			So(resp, ShouldResembleProto, makeResp(more,
				makeCI(1, 1*time.Second),
				makeCI(2, 9*time.Second),
				makeCI(3, 9*time.Second),
			))
			So(fake.calls, ShouldHaveLength, 3)
		})

		Convey("Bail if too many changes are concurrently updated", func() {
			pager.opts.Limit = 4
			pager.opts.PageSize = 3
			fake.reset(
				makeResp(more, makeCI(1, 1*time.Second), makeCI(2, 2*time.Second), makeCI(3, 3*time.Second)),
				makeResp(more, makeCI(3, 3*time.Second), makeCI(4, 4*time.Second), makeCI(5, 5*time.Second)),
				// Simulate 6 and 7 being updated after first call.
				makeResp(more, makeCI(6, 6*time.Millisecond), makeCI(7, 7*time.Millisecond)),
			)
			_, err := pager.pagingListChanges(ctx)
			So(err, ShouldErrLike, `PagingListChanges can't keep up with the rate of updates`)
			So(err, ShouldErrLike, `Try increasing PagingListChangesOptions.PageSize`)
			So(fake.calls, ShouldHaveLength, 3)
		})
	})
}

type fakeListChanges struct {
	results        []any
	calls          []*gerritpb.ListChangesRequest
	expectGrpcOpts []grpc.CallOption
}

func (f *fakeListChanges) reset(results ...any) {
	f.calls = nil
	f.results = results
}

func (f *fakeListChanges) ListChanges(ctx context.Context, in *gerritpb.ListChangesRequest, opts ...grpc.CallOption) (*gerritpb.ListChangesResponse, error) {
	logging.Debugf(ctx, "faking ListChanges(%s)", in)
	So(opts, ShouldResemble, f.expectGrpcOpts)
	f.calls = append(f.calls, in)
	if len(f.results) == 0 {
		// Unexpected call. List all of them.
		So(f.calls, ShouldBeNil)
		panic("unreachable")
	}
	r := f.results[0]
	f.results = f.results[1:]
	if v, ok := r.(*gerritpb.ListChangesResponse); ok {
		return v, nil
	}
	if v, ok := r.(error); ok {
		return nil, v
	}
	panic(fmt.Errorf("unrecognized result: %v", r))
}
