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
	"strconv"
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

	Convey("BuildOutputProperties", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("normal", func() {
			prop, err := structpb.NewStruct(map[string]interface{}{"key": "value"})
			So(err, ShouldBeNil)
			outProp := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				Proto: prop,
			}
			So(outProp.Put(ctx), ShouldBeNil)

			count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)

			outPropInDB := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
			}
			So(outPropInDB.Get(ctx), ShouldBeNil)
			So(outPropInDB.Proto, ShouldResembleProtoJSON, `{"key": "value"}`)
			So(outPropInDB.ChunkCount, ShouldEqual, 0)

			Convey("normal -> larger", func() {
				larger := proto.Clone(prop).(*structpb.Struct)
				larger.Fields["new_key"] = &structpb.Value{
					Kind: &structpb.Value_StringValue{
						StringValue: "new_value",
					},
				}

				outProp.Proto = larger
				So(outProp.Put(ctx), ShouldBeNil)
				So(outProp.ChunkCount, ShouldEqual, 0)

				outPropInDB := &BuildOutputProperties{
					Build: outProp.Build,
				}
				So(outPropInDB.Get(ctx), ShouldBeNil)
				So(outPropInDB.Proto, ShouldResembleProtoJSON, `{"key": "value", "new_key": "new_value"}`)
			})

			Convey("normal -> extreme large", func() {
				larger, err := structpb.NewStruct(map[string]interface{}{})
				So(err, ShouldBeNil)
				k := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_key"
				v := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_value"
				for i := 0; i < 10000; i++ {
					larger.Fields[k+strconv.Itoa(i)] = &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: v,
						},
					}
				}

				outProp.Proto = larger
				So(outProp.Put(ctx), ShouldBeNil)
				So(outProp.ChunkCount, ShouldEqual, 1)

				outPropInDB := &BuildOutputProperties{
					Build: outProp.Build,
				}
				So(outPropInDB.Get(ctx), ShouldBeNil)
				So(outPropInDB.Proto, ShouldResembleProto, larger)
			})
		})

		Convey("large", func() {
			largeProps, err := structpb.NewStruct(map[string]interface{}{})
			So(err, ShouldBeNil)
			k := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_key"
			v := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_value"
			for i := 0; i < 10000; i++ {
				largeProps.Fields[k+strconv.Itoa(i)] = &structpb.Value{
					Kind: &structpb.Value_StringValue{
						StringValue: v,
					},
				}
			}
			outProp := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				Proto: largeProps,
			}
			So(outProp.Put(ctx), ShouldBeNil)
			So(outProp.Proto, ShouldResembleProto, largeProps)
			So(outProp.ChunkCount, ShouldEqual, 1)

			count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)

			outPropInDB := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
			}
			So(datastore.Get(ctx, outPropInDB), ShouldBeNil)
			So(outPropInDB.ChunkCount, ShouldEqual, 1)

			So(outPropInDB.Get(ctx), ShouldBeNil)
			So(outPropInDB.Proto, ShouldResembleProto, largeProps)
			So(outPropInDB.ChunkCount, ShouldEqual, 0)

			Convey("large -> small", func() {
				prop, err := structpb.NewStruct(map[string]interface{}{"key": "value"})
				So(err, ShouldBeNil)
				// Proto got updated to a smaller one.
				outProp.Proto = prop

				So(outProp.Put(ctx), ShouldBeNil)
				So(outProp.ChunkCount, ShouldEqual, 0)

				outPropInDB := &BuildOutputProperties{
					Build: outProp.Build,
				}
				So(outPropInDB.Get(ctx), ShouldBeNil)
				So(outPropInDB.Proto, ShouldResembleProto, prop)
				So(outPropInDB.ChunkCount, ShouldEqual, 0)
			})

			Convey("large -> larger", func() {
				larger := proto.Clone(largeProps).(*structpb.Struct)
				curLen := len(larger.Fields)
				for i := 0; i < 10; i++ {
					larger.Fields[k+strconv.Itoa(curLen+i)] = &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: v,
						},
					}
				}
				// Proto got updated to an even larger one.
				outProp.Proto = larger

				So(outProp.Put(ctx), ShouldBeNil)
				So(outProp.ChunkCount, ShouldEqual, 1)
				So(outProp.Proto, ShouldResembleProto, larger)

				outPropInDB := &BuildOutputProperties{
					Build: outProp.Build,
				}
				So(outPropInDB.Get(ctx), ShouldBeNil)
				So(outPropInDB.Proto, ShouldResembleProto, larger)
				So(outPropInDB.ChunkCount, ShouldEqual, 0)
			})
		})

		Convey("too large (>1 chunks)", func() {
			originMaxPropertySize := maxPropertySize
			defer func() {
				maxPropertySize = originMaxPropertySize
			}()
			// to avoid take up too much memory in testing.
			maxPropertySize = 20

			largeProps, err := structpb.NewStruct(map[string]interface{}{})
			So(err, ShouldBeNil)
			k := "key"
			v := "value"

			Convey("3 chunks", func() {
				for i := 0; i < 3; i++ {
					largeProps.Fields[k+strconv.Itoa(i)] = &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: v,
						},
					}
				}
				outProp := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
					Proto: largeProps,
				}
				So(outProp.Put(ctx), ShouldBeNil)
				So(outProp.Proto, ShouldResembleProto, largeProps)
				So(outProp.ChunkCount, ShouldEqual, 3)

				count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 3)

				outPropInDB := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				}
				So(outPropInDB.Get(ctx), ShouldBeNil)
				So(outPropInDB.Proto, ShouldResembleProto, largeProps)
				So(outPropInDB.ChunkCount, ShouldEqual, 0)
			})

			Convey("4 chunks", func() {
				for i := 0; i < 10; i++ {
					largeProps.Fields[k+strconv.Itoa(i)] = &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: v,
						},
					}
				}
				outProp := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
					Proto: largeProps,
				}
				So(outProp.Put(ctx), ShouldBeNil)
				So(outProp.Proto, ShouldResembleProto, largeProps)
				So(outProp.ChunkCount, ShouldEqual, 4)

				count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 4)

				outPropInDB := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				}
				So(outPropInDB.Get(ctx), ShouldBeNil)
				So(outPropInDB.Proto, ShouldResembleProto, largeProps)
				So(outPropInDB.ChunkCount, ShouldEqual, 0)
			})

			Convey("8 chunks", func() {
				for i := 0; i < 30; i++ {
					largeProps.Fields[k+strconv.Itoa(i)] = &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: v,
						},
					}
				}
				outProp := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
					Proto: largeProps,
				}
				So(outProp.Put(ctx), ShouldBeNil)
				So(outProp.Proto, ShouldResembleProto, largeProps)
				So(outProp.ChunkCount, ShouldEqual, 8)

				count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 8)

				outPropInDB := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				}
				So(outPropInDB.Get(ctx), ShouldBeNil)
				So(outPropInDB.Proto, ShouldResembleProto, largeProps)
				So(outPropInDB.ChunkCount, ShouldEqual, 0)

				Convey("missing 2nd Chunk", func() {
					// Originally, it has 8 chunks. Now, intentionally delete the 2nd chunk
					chunk2 := &PropertyChunk{
						ID: 2,
						Parent: datastore.KeyForObj(ctx, &BuildOutputProperties{
							Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
						}),
					}
					So(datastore.Delete(ctx, chunk2), ShouldBeNil)

					count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
					So(err, ShouldBeNil)
					So(count, ShouldEqual, 7)

					outPropInDB := &BuildOutputProperties{
						Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
					}
					err = outPropInDB.Get(ctx)
					So(err, ShouldErrLike, "failed to decompress output properties bytes")
				})

				Convey("missing 7nd Chunk", func() {
					// Originally, it has 8 chunks. Now, intentionally delete the 7nd chunk
					chunk7 := &PropertyChunk{
						ID: 7,
						Parent: datastore.KeyForObj(ctx, &BuildOutputProperties{
							Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
						}),
					}
					So(datastore.Delete(ctx, chunk7), ShouldBeNil)

					count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
					So(err, ShouldBeNil)
					So(count, ShouldEqual, 7)

					outPropInDB := &BuildOutputProperties{
						Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
					}
					err = outPropInDB.Get(ctx)
					So(err, ShouldErrLike, "failed to fetch the rest chunks for BuildOutputProperties: datastore: no such entity")
				})
			})
		})

		Convey("BuildOutputProperties not exist", func() {
			outProp := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{
					ID: 999,
				}),
			}
			So(outProp.Get(ctx), ShouldEqual, datastore.ErrNoSuchEntity)
		})
	})
}
