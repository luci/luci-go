// Copyright 2016 The LUCI Authors.
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

package main

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/grpc"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	dm "go.chromium.org/luci/dm/api/service/v1"
)

type event struct {
	name string
	msg  proto.Message
}

type recordingClient struct {
	deps   []*EditParams
	result *EditResult

	walkGraphRsp       *dm.GraphData
	ensureGraphDataRsp *dm.EnsureGraphDataRsp
}

func (r *recordingClient) EnsureGraphData(ctx context.Context, in *dm.EnsureGraphDataReq, opts ...grpc.CallOption) (*dm.EnsureGraphDataRsp, error) {
	r.deps = nil
	for _, q := range in.Quest {
		ep := &EditParams{}
		if err := json.Unmarshal([]byte(q.Parameters), ep); err != nil {
			panic(err)
		}
		r.deps = append(r.deps, ep)
	}

	rsp := r.ensureGraphDataRsp
	r.ensureGraphDataRsp = nil
	return rsp, nil
}

func (r *recordingClient) ActivateExecution(ctx context.Context, in *dm.ActivateExecutionReq, opts ...grpc.CallOption) (*empty.Empty, error) {
	panic("never happens")
}

func (r *recordingClient) FinishAttempt(ctx context.Context, in *dm.FinishAttemptReq, opts ...grpc.CallOption) (*empty.Empty, error) {
	r.result = &EditResult{}
	if err := json.Unmarshal([]byte(in.Data.Object), r.result); err != nil {
		panic(err)
	}
	return nil, nil
}

func (r *recordingClient) WalkGraph(ctx context.Context, in *dm.WalkGraphReq, opts ...grpc.CallOption) (*dm.GraphData, error) {
	rsp := r.walkGraphRsp
	r.walkGraphRsp = nil
	return rsp, nil
}

var _ dm.DepsClient = (*recordingClient)(nil)

func newEDRunner() (*editDistanceRun, *recordingClient) {
	recorder := &recordingClient{}
	return &editDistanceRun{cmdRun: cmdRun{
		Context: context.Background(),
		client:  recorder,
		questDesc: &dm.Quest_Desc{
			DistributorParameters: "{}",
		},
	}}, recorder
}

func TestEditDistance(t *testing.T) {
	Convey("edit distance", t, func() {
		edr, recorder := newEDRunner()

		Convey("base cases", func() {
			Convey("empty", func() {
				er, stop := edr.compute(&EditParams{})
				So(stop, ShouldBeFalse)
				So(er, ShouldResemble, EditResult{0, "", ""})
			})

			Convey("all sub", func() {
				er, stop := edr.compute(&EditParams{"hello", ""})
				So(stop, ShouldBeFalse)
				So(er, ShouldResemble, EditResult{5, "-----", ""})
			})

			Convey("all add", func() {
				er, stop := edr.compute(&EditParams{"", "hello"})
				So(stop, ShouldBeFalse)
				So(er, ShouldResemble, EditResult{5, "+++++", ""})
			})
		})

		Convey("single", func() {
			Convey("equal", func() {
				er, stop := edr.compute(&EditParams{"a", "a"})
				So(stop, ShouldBeFalse)
				So(er, ShouldResemble, EditResult{0, "=", ""})
			})

			Convey("different", func() {

				Convey("trigger", func() {
					recorder.ensureGraphDataRsp = &dm.EnsureGraphDataRsp{
						Accepted: true, ShouldHalt: true}
					_, stop := edr.compute(&EditParams{"a", "b"})
					So(stop, ShouldBeTrue)
					So(recorder.deps, ShouldResemble, []*EditParams{
						{"", ""},
						{"", "b"},
						{"a", ""},
					})
				})

				Convey("already done", func() {
					recorder.ensureGraphDataRsp = &dm.EnsureGraphDataRsp{
						Accepted: true,
						QuestIds: []*dm.Quest_ID{{Id: "1"}, {Id: "2"}, {Id: "3"}},
						Result: &dm.GraphData{
							Quests: map[string]*dm.Quest{
								"1": {Attempts: map[uint32]*dm.Attempt{
									1: dm.NewAttemptFinished(dm.NewJsonResult("{}")),
								}},
								"2": {Attempts: map[uint32]*dm.Attempt{
									1: dm.NewAttemptFinished(dm.NewJsonResult("{}")),
								}},
								"3": {Attempts: map[uint32]*dm.Attempt{
									1: dm.NewAttemptFinished(dm.NewJsonResult("{}")),
								}},
							},
						},
					}
					er, stop := edr.compute(&EditParams{"a", "b"})
					So(stop, ShouldBeFalse)
					So(er, ShouldResemble, EditResult{1, "~", ""})
				})
			})
		})
	})
}
