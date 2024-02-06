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

	"go.chromium.org/luci/common/errors"
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
			prop, err := structpb.NewStruct(map[string]any{"key": "value"})
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
			So(outPropInDB.Proto, ShouldResembleProto, mustStruct(map[string]any{
				"key": "value",
			}))
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
				So(outPropInDB.Proto, ShouldResembleProto, mustStruct(map[string]any{
					"key":     "value",
					"new_key": "new_value",
				}))
			})

			Convey("normal -> extreme large", func() {
				larger, err := structpb.NewStruct(map[string]any{})
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
				So(outProp.ChunkCount, ShouldAlmostEqual, 1, 1)

				outPropInDB := &BuildOutputProperties{
					Build: outProp.Build,
				}
				So(outPropInDB.Get(ctx), ShouldBeNil)
				So(outPropInDB.Proto, ShouldResembleProto, larger)
			})
		})

		Convey("large", func() {
			largeProps, err := structpb.NewStruct(map[string]any{})
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
			So(outProp.ChunkCount, ShouldAlmostEqual, 1, 1)

			count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
			So(err, ShouldBeNil)
			So(count, ShouldAlmostEqual, 1, 1)

			outPropInDB := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
			}
			So(datastore.Get(ctx, outPropInDB), ShouldBeNil)
			So(outPropInDB.ChunkCount, ShouldAlmostEqual, 1, 1)

			So(outPropInDB.Get(ctx), ShouldBeNil)
			So(outPropInDB.Proto, ShouldResembleProto, largeProps)
			So(outPropInDB.ChunkCount, ShouldEqual, 0)

			Convey("large -> small", func() {
				prop, err := structpb.NewStruct(map[string]any{"key": "value"})
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
				So(outProp.ChunkCount, ShouldAlmostEqual, 1, 1)
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
			maxPropertySize = 100

			largeProps, err := structpb.NewStruct(map[string]any{})
			So(err, ShouldBeNil)
			k := "largeeeeeee_key"
			v := "largeeeeeee_value"

			Convey(">1 and <4 chunks", func() {
				for i := 0; i < 60; i++ {
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
				So(outProp.ChunkCount, ShouldBeBetween, 1, 4)

				count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
				So(err, ShouldBeNil)
				So(count, ShouldBeBetween, 1, 4)

				outPropInDB := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				}
				So(outPropInDB.Get(ctx), ShouldBeNil)
				So(outPropInDB.Proto, ShouldResembleProto, largeProps)
				So(outPropInDB.ChunkCount, ShouldEqual, 0)
			})

			Convey("~4 chunks", func() {
				for i := 0; i < 120; i++ {
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
				So(outProp.ChunkCount, ShouldAlmostEqual, 4, 2)

				count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
				So(err, ShouldBeNil)
				So(count, ShouldAlmostEqual, 4, 2)

				outPropInDB := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				}
				So(outPropInDB.Get(ctx), ShouldBeNil)
				So(outPropInDB.Proto, ShouldResembleProto, largeProps)
				So(outPropInDB.ChunkCount, ShouldEqual, 0)
			})

			Convey("> 4 chunks", func() {
				for i := 0; i < 500; i++ {
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
				So(outProp.ChunkCount, ShouldBeGreaterThan, 4)

				count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
				So(err, ShouldBeNil)
				So(count, ShouldBeGreaterThan, 4)

				outPropInDB := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				}
				So(outPropInDB.Get(ctx), ShouldBeNil)
				So(outPropInDB.Proto, ShouldResembleProto, largeProps)
				So(outPropInDB.ChunkCount, ShouldEqual, 0)

				Convey("missing 2nd Chunk", func() {
					// Originally, it has >4 chunks. Now, intentionally delete the 2nd chunk
					chunk2 := &PropertyChunk{
						ID:    2,
						Bytes: []byte("I am not valid compressed bytes."),
						Parent: datastore.KeyForObj(ctx, &BuildOutputProperties{
							Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
						}),
					}
					So(datastore.Put(ctx, chunk2), ShouldBeNil)

					outPropInDB := &BuildOutputProperties{
						Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
					}
					err = outPropInDB.Get(ctx)
					So(err, ShouldErrLike, "failed to decompress output properties bytes")
				})

				Convey("missing 5nd Chunk", func() {
					// Originally, it has >4 chunks. Now, intentionally delete the 5nd chunk
					chunk5 := &PropertyChunk{
						ID: 5,
						Parent: datastore.KeyForObj(ctx, &BuildOutputProperties{
							Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
						}),
					}
					So(datastore.Delete(ctx, chunk5), ShouldBeNil)

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

		Convey("GetMultiOutputProperties", func() {
			largeProps, err := structpb.NewStruct(map[string]any{})
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
			outProp1 := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				Proto: largeProps,
			}
			outProp2 := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 456}),
				Proto: largeProps,
			}
			So(outProp1.Put(ctx), ShouldBeNil)
			So(outProp2.Put(ctx), ShouldBeNil)

			outPropInDB1 := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
			}
			outPropInDB2 := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 456}),
			}
			So(GetMultiOutputProperties(ctx, outPropInDB1, outPropInDB2), ShouldBeNil)
			So(outPropInDB1.Proto, ShouldResembleProto, largeProps)
			So(outPropInDB2.Proto, ShouldResembleProto, largeProps)

			Convey("one empty, one found", func() {
				outPropInDB1 := &BuildOutputProperties{}
				outPropInDB2 := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 456}),
				}
				So(GetMultiOutputProperties(ctx, outPropInDB1, outPropInDB2), ShouldBeNil)
				So(outPropInDB1.Proto, ShouldBeNil)
				So(outPropInDB2.Proto, ShouldResembleProto, largeProps)
			})

			Convey("one not found, one found", func() {
				outPropInDB1 := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 999}),
				}
				outPropInDB2 := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 456}),
				}
				err := GetMultiOutputProperties(ctx, outPropInDB1, outPropInDB2)
				So(err, ShouldNotBeNil)
				me, _ := err.(errors.MultiError)
				So(me[0], ShouldErrLike, datastore.ErrNoSuchEntity)
				So(outPropInDB1.Proto, ShouldBeNil)
				So(outPropInDB2.Proto, ShouldResembleProto, largeProps)
			})
		})
	})
}
