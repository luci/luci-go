// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bq

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	bqpb "go.chromium.org/luci/swarming/proto/bq"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAppendRows(t *testing.T) {
	t.Parallel()
	Convey("With mocks", t, func() {
		Convey("Test writting on equally sized protos", func() {
			tr := func(x int) *bqpb.TaskRequest {
				return &bqpb.TaskRequest{
					// simple hack to make sure string is same size (3 digits)
					Name: fmt.Sprintf("%d", x+100),
				}
			}
			generateTestRequests := func(n int) []proto.Message {
				requests := make([]proto.Message, n)
				for i := 0; i < n; i++ {
					requests[i] = tr(i)
				}
				return requests
			}
			generateEmpty := func(total, perRow int) [][]proto.Message {
				extra := 0
				if total%perRow > 0 {
					extra = 1
				}
				return make([][]proto.Message, total/perRow+extra)
			}
			generateExpectedData := func(total, perRow int) [][]proto.Message {
				data := generateEmpty(total, perRow)
				idx := 0
				for idx < total {
					row := make([]proto.Message, 0, perRow)
					for i := 0; i < perRow && idx < total; i, idx = i+1, idx+1 {
						row = append(row, tr(idx))
					}
					data[(idx-1)/perRow] = row
				}
				return data
			}
			generateMockSC := func(actual [][]proto.Message, w resultWaiter) *mockStreamClient {
				idx := 0
				return &mockStreamClient{
					appendRowsMock: func(ctx context.Context, rows [][]byte) (resultWaiter, error) {
						l := len(rows)
						protos := make([]proto.Message, l)
						for i, r := range rows {
							actual := &bqpb.TaskRequest{}
							So(proto.Unmarshal(r, actual), ShouldBeNil)
							protos[i] = actual
						}
						So(idx, ShouldBeLessThan, len(actual))
						actual[idx] = protos
						idx += 1
						return w, nil
					},
				}
			}

			noop := func(ctx context.Context) error {
				return nil
			}

			Convey("No calls for empty list", func() {
				ctx, _ := setup()
				const n = 0
				requests := generateTestRequests(n)
				const perRow = 100
				expected := generateExpectedData(n, perRow)
				actual := generateEmpty(n, perRow)
				mockSC := generateMockSC(actual, noop)
				g, ctx := errgroup.WithContext(ctx)
				exporter := bqWriter{
					maxSizePerWrite: 100,
					stream:          mockSC,
					results:         g,
				}
				So(exporter.writeProtos(ctx, requests), ShouldBeNil)
				So(mockSC.appendCalls, ShouldEqual, 0)
				So(actual, ShouldHaveLength, 0)
				So(actual, ShouldResembleProto, expected)
			})

			Convey("Works with prime groups", func() {
				ctx, _ := setup()
				const n = 100
				requests := generateTestRequests(n)
				const perRow = 23
				expected := generateExpectedData(n, perRow)
				actual := generateEmpty(n, perRow)
				mockSC := generateMockSC(actual, noop)
				fb, err := proto.Marshal(requests[0])
				So(err, ShouldBeNil)
				size := len(fb) * perRow
				g, ctx := errgroup.WithContext(ctx)
				exporter := bqWriter{
					maxSizePerWrite: size,
					stream:          mockSC,
					results:         g,
				}
				So(exporter.writeProtos(ctx, requests), ShouldBeNil)
				So(mockSC.appendCalls, ShouldEqual, 5)
				So(actual, ShouldResembleProto, expected)
			})

			Convey("Makes single call for less than maxSize", func() {
				ctx, _ := setup()
				const n = 100
				requests := generateTestRequests(n)
				const perRow = n
				expected := generateExpectedData(n, perRow)
				actual := generateEmpty(n, perRow)
				mockSC := generateMockSC(actual, noop)
				g, ctx := errgroup.WithContext(ctx)
				exporter := bqWriter{
					maxSizePerWrite: 10_000_000,
					stream:          mockSC,
					results:         g,
				}
				So(exporter.writeProtos(ctx, requests), ShouldBeNil)
				So(mockSC.appendCalls, ShouldEqual, 1)
				So(actual, ShouldResembleProto, expected)
			})

			Convey("Makes many calls for more than maxSize", func() {
				const n = 100
				requests := generateTestRequests(n)
				const perRow = 5
				expected := generateExpectedData(n, perRow)
				actual := generateEmpty(n, perRow)
				fb, err := proto.Marshal(requests[0])
				So(err, ShouldBeNil)
				size := len(fb) * perRow
				Convey("Happy trail finalizes all unfinished calls", func() {
					ctx, _ := setup()
					mockSC := generateMockSC(actual, noop)
					mockSC.finalizeMock = func(ctx context.Context) error {
						return nil
					}
					g, ctx := errgroup.WithContext(ctx)
					exporter := bqWriter{
						maxSizePerWrite: size,
						stream:          mockSC,
						results:         g,
					}
					So(exporter.writeProtos(ctx, requests), ShouldBeNil)
					So(mockSC.appendCalls, ShouldEqual, 20)
					So(actual, ShouldResembleProto, expected)
					So(exporter.finalize(ctx), ShouldBeNil)
					So(mockSC.finalizeCalls, ShouldEqual, 1)
				})
				Convey("If one resultWaiter fails, finalize will not be called", func() {
					ctx, _ := setup()
					var mut sync.Mutex
					idx := 0
					failOnce := func(ctx context.Context) error {
						mut.Lock()
						defer mut.Unlock()
						if idx == 0 {
							return errors.New("oops")
						}
						idx += 1
						return nil
					}
					mockSC := generateMockSC(actual, failOnce)
					mockSC.finalizeMock = func(ctx context.Context) error {
						return nil
					}
					g, ctx := errgroup.WithContext(ctx)
					exporter := bqWriter{
						maxSizePerWrite: size,
						stream:          mockSC,
						results:         g,
					}
					So(exporter.writeProtos(ctx, requests), ShouldBeNil)
					So(exporter.finalize(ctx), ShouldNotBeNil)
					So(mockSC.finalizeCalls, ShouldEqual, 0)
				})
			})
		})
	})
}
