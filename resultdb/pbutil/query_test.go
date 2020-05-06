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

package pbutil

import (
	"context"
	"testing"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/testing/mock"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQuery(t *testing.T) {
	t.Parallel()
	Convey(`TestQuery`, t, func() {
		ctx := context.Background()

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		client := pb.NewMockResultDBClient(ctl)

		fetch := func(req *pb.QueryTestResultsRequest) ([]*pb.TestResult, error) {
			itemC := make(chan proto.Message)
			errC := make(chan error)
			go func() {
				err := Query(ctx, itemC, client, req)
				close(itemC)
				errC <- err
			}()

			var results []*pb.TestResult
			for r := range itemC {
				results = append(results, r.(*pb.TestResult))
			}
			return results, <-errC
		}

		Convey("One page", func() {
			expected := []*pb.TestResult{{Name: "a"}, {Name: "b"}}
			client.EXPECT().
				QueryTestResults(gomock.Any(), mock.EqProto(&pb.QueryTestResultsRequest{})).
				Return(&pb.QueryTestResultsResponse{TestResults: expected}, nil)

			actual, err := fetch(&pb.QueryTestResultsRequest{})
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, expected)
		})

		Convey("Two pages", func() {
			firstPage := client.EXPECT().
				QueryTestResults(gomock.Any(), mock.EqProto(&pb.QueryTestResultsRequest{})).
				Return(&pb.QueryTestResultsResponse{
					TestResults:   []*pb.TestResult{{Name: "a"}, {Name: "b"}},
					NextPageToken: "token",
				}, nil)
			client.EXPECT().
				QueryTestResults(gomock.Any(), mock.EqProto(&pb.QueryTestResultsRequest{
					PageToken: "token",
				})).
				After(firstPage).
				Return(&pb.QueryTestResultsResponse{
					TestResults: []*pb.TestResult{{Name: "c"}},
				}, nil)

			actual, err := fetch(&pb.QueryTestResultsRequest{})
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, []*pb.TestResult{{Name: "a"}, {Name: "b"}, {Name: "c"}})
		})
	})
}
