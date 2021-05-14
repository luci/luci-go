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

package model

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDetails(t *testing.T) {
	t.Parallel()

	Convey("Details", t, func() {
		Convey("BuildSteps", func() {
			ctx := memory.Use(context.Background())
			datastore.GetTestable(ctx).AutoIndex(true)
			datastore.GetTestable(ctx).Consistent(true)

			Convey("CancelIncomplete", func() {
				now := &timestamppb.Timestamp{
					Seconds: 123,
				}

				Convey("error", func() {
					s := &BuildSteps{
						IsZipped: true,
					}
					ch, err := s.CancelIncomplete(ctx, now)
					So(err, ShouldErrLike, "error creating reader")
					So(ch, ShouldBeFalse)
					So(s, ShouldResemble, &BuildSteps{
						IsZipped: true,
					})
				})

				Convey("not changed", func() {
					Convey("empty", func() {
						b, err := proto.Marshal(&pb.Build{})
						So(err, ShouldBeNil)

						s := &BuildSteps{
							IsZipped: false,
							Bytes:    b,
						}
						ch, err := s.CancelIncomplete(ctx, now)
						So(err, ShouldBeNil)
						So(ch, ShouldBeFalse)
						So(s, ShouldResemble, &BuildSteps{
							IsZipped: false,
							Bytes:    b,
						})
					})

					Convey("completed", func() {
						b, err := proto.Marshal(&pb.Build{
							Steps: []*pb.Step{
								{
									Status: pb.Status_SUCCESS,
								},
							},
						})
						So(err, ShouldBeNil)
						s := &BuildSteps{
							IsZipped: false,
							Bytes:    b,
						}
						ch, err := s.CancelIncomplete(ctx, now)
						So(err, ShouldBeNil)
						So(ch, ShouldBeFalse)
						So(s, ShouldResemble, &BuildSteps{
							IsZipped: false,
							Bytes:    b,
						})
					})
				})

				Convey("changed", func() {
					b, err := proto.Marshal(&pb.Build{
						Steps: []*pb.Step{
							{
								Name: "step",
							},
						},
					})
					So(err, ShouldBeNil)
					s := &BuildSteps{
						IsZipped: false,
						Bytes:    b,
					}
					b, err = proto.Marshal(&pb.Build{
						Steps: []*pb.Step{
							{
								EndTime: now,
								Name:    "step",
								Status:  pb.Status_CANCELED,
							},
						},
					})
					So(err, ShouldBeNil)
					ch, err := s.CancelIncomplete(ctx, now)
					So(err, ShouldBeNil)
					So(ch, ShouldBeTrue)
					So(s, ShouldResemble, &BuildSteps{
						IsZipped: false,
						Bytes:    b,
					})
				})
			})

			Convey("FromProto", func() {
				Convey("not zipped", func() {
					b, err := proto.Marshal(&pb.Build{
						Steps: []*pb.Step{
							{
								Name: "step",
							},
						},
					})
					So(err, ShouldBeNil)
					s := &BuildSteps{}
					So(s.FromProto([]*pb.Step{
						{
							Name: "step",
						},
					}), ShouldBeNil)
					So(s.Bytes, ShouldResemble, b)
					So(s.IsZipped, ShouldBeFalse)
				})
			})

			Convey("ToProto", func() {
				Convey("zipped", func() {
					Convey("error", func() {
						s := &BuildSteps{
							IsZipped: true,
						}
						p, err := s.ToProto(ctx)
						So(err, ShouldErrLike, "error creating reader")
						So(p, ShouldBeNil)
					})

					Convey("ok", func() {
						s := &BuildSteps{
							// { name: "step" }
							Bytes:    []byte{120, 156, 234, 98, 100, 227, 98, 41, 46, 73, 45, 0, 4, 0, 0, 255, 255, 9, 199, 2, 92},
							IsZipped: true,
						}
						p, err := s.ToProto(ctx)
						So(err, ShouldBeNil)
						So(p, ShouldResembleProto, []*pb.Step{
							{
								Name: "step",
							},
						})
					})
				})

				Convey("not zipped", func() {
					b, err := proto.Marshal(&pb.Build{
						Steps: []*pb.Step{
							{
								Name: "step",
							},
						},
					})
					So(err, ShouldBeNil)
					s := &BuildSteps{
						IsZipped: false,
						Bytes:    b,
					}
					p, err := s.ToProto(ctx)
					So(err, ShouldBeNil)
					So(p, ShouldResembleProto, []*pb.Step{
						{
							Name: "step",
						},
					})
				})
			})
		})

		Convey("defaultStructValue", func() {
			Convey("nil struct", func() {
				defaultStructValues(nil)
			})

			Convey("empty struct", func() {
				s := &structpb.Struct{}
				defaultStructValues(s)
				So(s, ShouldResembleProto, &structpb.Struct{})
			})

			Convey("empty fields", func() {
				s := &structpb.Struct{
					Fields: map[string]*structpb.Value{},
				}
				defaultStructValues(s)
				So(s, ShouldResembleProto, &structpb.Struct{
					Fields: map[string]*structpb.Value{},
				})
			})

			Convey("nil value", func() {
				s := &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": nil,
					},
				}
				defaultStructValues(s)
				So(s, ShouldResembleProto, &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": {
							Kind: &structpb.Value_NullValue{},
						},
					},
				})
			})

			Convey("empty value", func() {
				s := &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": {},
					},
				}
				defaultStructValues(s)
				So(s, ShouldResembleProto, &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": {
							Kind: &structpb.Value_NullValue{},
						},
					},
				})
			})

			Convey("recursive", func() {
				s := &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": {
							Kind: &structpb.Value_StructValue{
								StructValue: &structpb.Struct{
									Fields: map[string]*structpb.Value{
										"key": {},
									},
								},
							},
						},
					},
				}
				defaultStructValues(s)
				So(s, ShouldResembleProto, &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": {
							Kind: &structpb.Value_StructValue{
								StructValue: &structpb.Struct{
									Fields: map[string]*structpb.Value{
										"key": {
											Kind: &structpb.Value_NullValue{},
										},
									},
								},
							},
						},
					},
				})
			})
		})
	})
}
