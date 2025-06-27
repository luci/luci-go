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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestDetails(t *testing.T) {
	t.Parallel()

	ftt.Run("Details", t, func(t *ftt.Test) {
		t.Run("BuildSteps", func(t *ftt.Test) {
			ctx := memory.Use(context.Background())
			datastore.GetTestable(ctx).AutoIndex(true)
			datastore.GetTestable(ctx).Consistent(true)

			t.Run("CancelIncomplete", func(t *ftt.Test) {
				now := &timestamppb.Timestamp{
					Seconds: 123,
				}

				t.Run("error", func(t *ftt.Test) {
					s := &BuildSteps{
						IsZipped: true,
					}
					ch, err := s.CancelIncomplete(ctx, now)
					assert.Loosely(t, err, should.ErrLike("error creating reader"))
					assert.Loosely(t, ch, should.BeFalse)
					assert.Loosely(t, s, should.Resemble(&BuildSteps{
						IsZipped: true,
					}))
				})

				t.Run("not changed", func(t *ftt.Test) {
					t.Run("empty", func(t *ftt.Test) {
						b, err := proto.Marshal(&pb.Build{})
						assert.Loosely(t, err, should.BeNil)

						s := &BuildSteps{
							IsZipped: false,
							Bytes:    b,
						}
						ch, err := s.CancelIncomplete(ctx, now)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, ch, should.BeFalse)
						assert.Loosely(t, s, should.Resemble(&BuildSteps{
							IsZipped: false,
							Bytes:    b,
						}))
					})

					t.Run("completed", func(t *ftt.Test) {
						b, err := proto.Marshal(&pb.Build{
							Steps: []*pb.Step{
								{
									Status: pb.Status_SUCCESS,
								},
							},
						})
						assert.Loosely(t, err, should.BeNil)
						s := &BuildSteps{
							IsZipped: false,
							Bytes:    b,
						}
						ch, err := s.CancelIncomplete(ctx, now)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, ch, should.BeFalse)
						assert.Loosely(t, s, should.Resemble(&BuildSteps{
							IsZipped: false,
							Bytes:    b,
						}))
					})
				})

				t.Run("changed", func(t *ftt.Test) {
					b, err := proto.Marshal(&pb.Build{
						Steps: []*pb.Step{
							{
								Name: "step",
							},
						},
					})
					assert.Loosely(t, err, should.BeNil)
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
					assert.Loosely(t, err, should.BeNil)
					ch, err := s.CancelIncomplete(ctx, now)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, ch, should.BeTrue)
					assert.Loosely(t, s, should.Resemble(&BuildSteps{
						IsZipped: false,
						Bytes:    b,
					}))
				})
			})

			t.Run("FromProto", func(t *ftt.Test) {
				t.Run("not zipped", func(t *ftt.Test) {
					b, err := proto.Marshal(&pb.Build{
						Steps: []*pb.Step{
							{
								Name: "step",
							},
						},
					})
					assert.Loosely(t, err, should.BeNil)
					s := &BuildSteps{}
					assert.Loosely(t, s.FromProto([]*pb.Step{
						{
							Name: "step",
						},
					}), should.BeNil)
					assert.Loosely(t, s.Bytes, should.Resemble(b))
					assert.Loosely(t, s.IsZipped, should.BeFalse)
				})
			})

			t.Run("ToProto", func(t *ftt.Test) {
				t.Run("zipped", func(t *ftt.Test) {
					t.Run("error", func(t *ftt.Test) {
						s := &BuildSteps{
							IsZipped: true,
						}
						p, err := s.ToProto(ctx)
						assert.Loosely(t, err, should.ErrLike("error creating reader"))
						assert.Loosely(t, p, should.BeNil)
					})

					t.Run("ok", func(t *ftt.Test) {
						s := &BuildSteps{
							// { name: "step" }
							Bytes:    []byte{120, 156, 234, 98, 100, 227, 98, 41, 46, 73, 45, 0, 4, 0, 0, 255, 255, 9, 199, 2, 92},
							IsZipped: true,
						}
						p, err := s.ToProto(ctx)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, p, should.Resemble([]*pb.Step{
							{
								Name: "step",
							},
						}))
					})
				})

				t.Run("not zipped", func(t *ftt.Test) {
					b, err := proto.Marshal(&pb.Build{
						Steps: []*pb.Step{
							{
								Name: "step",
							},
						},
					})
					assert.Loosely(t, err, should.BeNil)
					s := &BuildSteps{
						IsZipped: false,
						Bytes:    b,
					}
					p, err := s.ToProto(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, p, should.Resemble([]*pb.Step{
						{
							Name: "step",
						},
					}))
				})
			})
		})

		t.Run("defaultStructValue", func(t *ftt.Test) {
			t.Run("nil struct", func(t *ftt.Test) {
				defaultStructValues(nil)
			})

			t.Run("empty struct", func(t *ftt.Test) {
				s := &structpb.Struct{}
				defaultStructValues(s)
				assert.Loosely(t, s, should.Resemble(&structpb.Struct{}))
			})

			t.Run("empty fields", func(t *ftt.Test) {
				s := &structpb.Struct{
					Fields: map[string]*structpb.Value{},
				}
				defaultStructValues(s)
				assert.Loosely(t, s, should.Resemble(&structpb.Struct{
					Fields: map[string]*structpb.Value{},
				}))
			})

			t.Run("nil value", func(t *ftt.Test) {
				s := &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": nil,
					},
				}
				defaultStructValues(s)
				assert.Loosely(t, s, should.Resemble(&structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": {
							Kind: &structpb.Value_NullValue{},
						},
					},
				}))
			})

			t.Run("empty value", func(t *ftt.Test) {
				s := &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": {},
					},
				}
				defaultStructValues(s)
				assert.Loosely(t, s, should.Resemble(&structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": {
							Kind: &structpb.Value_NullValue{},
						},
					},
				}))
			})

			t.Run("recursive", func(t *ftt.Test) {
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
				assert.Loosely(t, s, should.Resemble(&structpb.Struct{
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
				}))
			})
		})
	})

	ftt.Run("BuildOutputProperties", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		t.Run("normal", func(t *ftt.Test) {
			prop, err := structpb.NewStruct(map[string]any{"key": "value"})
			assert.Loosely(t, err, should.BeNil)
			outProp := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				Proto: prop,
			}
			assert.Loosely(t, outProp.Put(ctx), should.BeNil)

			count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, count, should.BeZero)

			outPropInDB := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
			}
			assert.Loosely(t, outPropInDB.Get(ctx), should.BeNil)
			assert.Loosely(t, outPropInDB.Proto, should.Resemble(mustStruct(map[string]any{
				"key": "value",
			})))
			assert.Loosely(t, outPropInDB.ChunkCount, should.BeZero)

			t.Run("normal -> larger", func(t *ftt.Test) {
				larger := proto.Clone(prop).(*structpb.Struct)
				larger.Fields["new_key"] = &structpb.Value{
					Kind: &structpb.Value_StringValue{
						StringValue: "new_value",
					},
				}

				outProp.Proto = larger
				assert.Loosely(t, outProp.Put(ctx), should.BeNil)
				assert.Loosely(t, outProp.ChunkCount, should.BeZero)

				outPropInDB := &BuildOutputProperties{
					Build: outProp.Build,
				}
				assert.Loosely(t, outPropInDB.Get(ctx), should.BeNil)
				assert.Loosely(t, outPropInDB.Proto, should.Resemble(mustStruct(map[string]any{
					"key":     "value",
					"new_key": "new_value",
				})))
			})

			t.Run("normal -> extreme large", func(t *ftt.Test) {
				larger, err := structpb.NewStruct(map[string]any{})
				assert.Loosely(t, err, should.BeNil)
				k := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_key"
				v := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_value"
				for i := range 10000 {
					larger.Fields[k+strconv.Itoa(i)] = &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: v,
						},
					}
				}

				outProp.Proto = larger
				assert.Loosely(t, outProp.Put(ctx), should.BeNil)
				assert.Loosely(t, outProp.ChunkCount, should.Equal(1))

				outPropInDB := &BuildOutputProperties{
					Build: outProp.Build,
				}
				assert.Loosely(t, outPropInDB.Get(ctx), should.BeNil)
				assert.Loosely(t, outPropInDB.Proto, should.Resemble(larger))
			})
		})

		t.Run("large", func(t *ftt.Test) {
			largeProps, err := structpb.NewStruct(map[string]any{})
			assert.Loosely(t, err, should.BeNil)
			k := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_key"
			v := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_value"
			for i := range 10000 {
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
			assert.Loosely(t, outProp.Put(ctx), should.BeNil)
			assert.Loosely(t, outProp.Proto, should.Resemble(largeProps))
			assert.Loosely(t, outProp.ChunkCount, should.Equal(1))

			count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, count, should.Equal(1))

			outPropInDB := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
			}
			assert.Loosely(t, datastore.Get(ctx, outPropInDB), should.BeNil)
			assert.Loosely(t, outPropInDB.ChunkCount, should.Equal(1))

			assert.Loosely(t, outPropInDB.Get(ctx), should.BeNil)
			assert.Loosely(t, outPropInDB.Proto, should.Resemble(largeProps))
			assert.Loosely(t, outPropInDB.ChunkCount, should.BeZero)

			t.Run("large -> small", func(t *ftt.Test) {
				prop, err := structpb.NewStruct(map[string]any{"key": "value"})
				assert.Loosely(t, err, should.BeNil)
				// Proto got updated to a smaller one.
				outProp.Proto = prop

				assert.Loosely(t, outProp.Put(ctx), should.BeNil)
				assert.Loosely(t, outProp.ChunkCount, should.BeZero)

				outPropInDB := &BuildOutputProperties{
					Build: outProp.Build,
				}
				assert.Loosely(t, outPropInDB.Get(ctx), should.BeNil)
				assert.Loosely(t, outPropInDB.Proto, should.Resemble(prop))
				assert.Loosely(t, outPropInDB.ChunkCount, should.BeZero)
			})

			t.Run("large -> larger", func(t *ftt.Test) {
				larger := proto.Clone(largeProps).(*structpb.Struct)
				curLen := len(larger.Fields)
				for i := range 10 {
					larger.Fields[k+strconv.Itoa(curLen+i)] = &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: v,
						},
					}
				}
				// Proto got updated to an even larger one.
				outProp.Proto = larger

				assert.Loosely(t, outProp.Put(ctx), should.BeNil)
				assert.Loosely(t, outProp.ChunkCount, should.Equal(1))
				assert.Loosely(t, outProp.Proto, should.Resemble(larger))

				outPropInDB := &BuildOutputProperties{
					Build: outProp.Build,
				}
				assert.Loosely(t, outPropInDB.Get(ctx), should.BeNil)
				assert.Loosely(t, outPropInDB.Proto, should.Resemble(larger))
				assert.Loosely(t, outPropInDB.ChunkCount, should.BeZero)
			})
		})

		t.Run("too large (>1 chunks)", func(t *ftt.Test) {
			originMaxPropertySize := maxPropertySize
			defer func() {
				maxPropertySize = originMaxPropertySize
			}()
			// to avoid take up too much memory in testing.
			maxPropertySize = 100

			largeProps, err := structpb.NewStruct(map[string]any{})
			assert.Loosely(t, err, should.BeNil)
			k := "largeeeeeee_key"
			v := "largeeeeeee_value"

			t.Run(">1 and <4 chunks", func(t *ftt.Test) {
				for i := range 60 {
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
				assert.Loosely(t, outProp.Put(ctx), should.BeNil)
				assert.Loosely(t, outProp.Proto, should.Resemble(largeProps))
				assert.Loosely(t, outProp.ChunkCount, should.BeBetween(1, 4))

				count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.BeBetween(1, 4))

				outPropInDB := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				}
				assert.Loosely(t, outPropInDB.Get(ctx), should.BeNil)
				assert.Loosely(t, outPropInDB.Proto, should.Resemble(largeProps))
				assert.Loosely(t, outPropInDB.ChunkCount, should.BeZero)
			})

			t.Run("~4 chunks", func(t *ftt.Test) {
				for i := range 120 {
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
				assert.Loosely(t, outProp.Put(ctx), should.BeNil)
				assert.Loosely(t, outProp.Proto, should.Resemble(largeProps))
				assert.Loosely(t, outProp.ChunkCount, should.Equal(4))

				count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(4))

				outPropInDB := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				}
				assert.Loosely(t, outPropInDB.Get(ctx), should.BeNil)
				assert.Loosely(t, outPropInDB.Proto, should.Resemble(largeProps))
				assert.Loosely(t, outPropInDB.ChunkCount, should.BeZero)
			})

			t.Run("> 4 chunks", func(t *ftt.Test) {
				for i := range 500 {
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
				assert.Loosely(t, outProp.Put(ctx), should.BeNil)
				assert.Loosely(t, outProp.Proto, should.Resemble(largeProps))
				assert.Loosely(t, outProp.ChunkCount, should.BeGreaterThan(4))

				count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.BeGreaterThan(4))

				outPropInDB := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
				}
				assert.Loosely(t, outPropInDB.Get(ctx), should.BeNil)
				assert.Loosely(t, outPropInDB.Proto, should.Resemble(largeProps))
				assert.Loosely(t, outPropInDB.ChunkCount, should.BeZero)

				t.Run("missing 2nd Chunk", func(t *ftt.Test) {
					// Originally, it has >4 chunks. Now, intentionally delete the 2nd chunk
					chunk2 := &PropertyChunk{
						ID:    2,
						Bytes: []byte("I am not valid compressed bytes."),
						Parent: datastore.KeyForObj(ctx, &BuildOutputProperties{
							Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
						}),
					}
					assert.Loosely(t, datastore.Put(ctx, chunk2), should.BeNil)

					outPropInDB := &BuildOutputProperties{
						Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
					}
					err = outPropInDB.Get(ctx)
					assert.Loosely(t, err, should.ErrLike("failed to decompress output properties bytes"))
				})

				t.Run("missing 5nd Chunk", func(t *ftt.Test) {
					// Originally, it has >4 chunks. Now, intentionally delete the 5nd chunk
					chunk5 := &PropertyChunk{
						ID: 5,
						Parent: datastore.KeyForObj(ctx, &BuildOutputProperties{
							Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
						}),
					}
					assert.Loosely(t, datastore.Delete(ctx, chunk5), should.BeNil)

					outPropInDB := &BuildOutputProperties{
						Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
					}
					err = outPropInDB.Get(ctx)
					assert.Loosely(t, err, should.ErrLike("failed to fetch the rest chunks for BuildOutputProperties: datastore: no such entity"))
				})
			})
		})

		t.Run("BuildOutputProperties not exist", func(t *ftt.Test) {
			outProp := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{
					ID: 999,
				}),
			}
			assert.Loosely(t, outProp.Get(ctx), should.Equal(datastore.ErrNoSuchEntity))
		})

		t.Run("GetMultiOutputProperties", func(t *ftt.Test) {
			largeProps, err := structpb.NewStruct(map[string]any{})
			assert.Loosely(t, err, should.BeNil)
			k := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_key"
			v := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_value"
			for i := range 10000 {
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
			assert.Loosely(t, outProp1.Put(ctx), should.BeNil)
			assert.Loosely(t, outProp2.Put(ctx), should.BeNil)

			outPropInDB1 := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 123}),
			}
			outPropInDB2 := &BuildOutputProperties{
				Build: datastore.KeyForObj(ctx, &Build{ID: 456}),
			}
			assert.Loosely(t, GetMultiOutputProperties(ctx, outPropInDB1, outPropInDB2), should.BeNil)
			assert.Loosely(t, outPropInDB1.Proto, should.Resemble(largeProps))
			assert.Loosely(t, outPropInDB2.Proto, should.Resemble(largeProps))

			t.Run("one empty, one found", func(t *ftt.Test) {
				outPropInDB1 := &BuildOutputProperties{}
				outPropInDB2 := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 456}),
				}
				assert.Loosely(t, GetMultiOutputProperties(ctx, outPropInDB1, outPropInDB2), should.BeNil)
				assert.Loosely(t, outPropInDB1.Proto, should.BeNil)
				assert.Loosely(t, outPropInDB2.Proto, should.Resemble(largeProps))
			})

			t.Run("one not found, one found", func(t *ftt.Test) {
				outPropInDB1 := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 999}),
				}
				outPropInDB2 := &BuildOutputProperties{
					Build: datastore.KeyForObj(ctx, &Build{ID: 456}),
				}
				err := GetMultiOutputProperties(ctx, outPropInDB1, outPropInDB2)
				assert.Loosely(t, err, should.NotBeNil)
				me, _ := err.(errors.MultiError)
				assert.Loosely(t, me[0], should.ErrLike(datastore.ErrNoSuchEntity))
				assert.Loosely(t, outPropInDB1.Proto, should.BeNil)
				assert.Loosely(t, outPropInDB2.Proto, should.Resemble(largeProps))
			})
		})
	})
}
