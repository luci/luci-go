// Copyright 2018 The LUCI Authors.
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

package rpc

import (
	"context"
	"testing"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/appengine/model"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Config", t, func() {
		srv := &Config{}
		c := memory.Use(context.Background())
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)

		Convey("Delete", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					cfg, err := srv.Delete(c, nil)
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})

				Convey("empty", func() {
					cfg, err := srv.Delete(c, &config.DeleteRequest{})
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})
			})

			Convey("valid", func() {
				So(datastore.Put(c, &model.Config{
					ID: "id",
				}), ShouldBeNil)
				cfg, err := srv.Delete(c, &config.DeleteRequest{
					Id: "id",
				})
				So(err, ShouldBeNil)
				So(cfg, ShouldResemble, &emptypb.Empty{})
				err = datastore.Get(c, &model.Config{
					ID: "id",
				})
				So(err, ShouldEqual, datastore.ErrNoSuchEntity)
			})
		})

		Convey("Ensure", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					cfg, err := srv.Ensure(c, nil)
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})

				Convey("empty", func() {
					cfg, err := srv.Ensure(c, &config.EnsureRequest{})
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})

				Convey("ID", func() {
					cfg, err := srv.Ensure(c, &config.EnsureRequest{
						Config: &config.Config{},
					})
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})
			})

			Convey("valid", func() {
				Convey("new", func() {
					cfg, err := srv.Ensure(c, &config.EnsureRequest{
						Id: "id",
						Config: &config.Config{
							Attributes: &config.VM{
								Disk: []*config.Disk{
									{},
								},
								MachineType: "type",
								NetworkInterface: []*config.NetworkInterface{
									{},
								},
								Project: "project",
								Zone:    "zone",
							},
							Lifetime: &config.TimePeriod{
								Time: &config.TimePeriod_Seconds{
									Seconds: 3600,
								},
							},
							Prefix: "prefix",
						},
					})
					So(err, ShouldBeNil)
					So(cfg, ShouldResembleProto, &config.Config{
						Attributes: &config.VM{
							Disk: []*config.Disk{
								{},
							},
							MachineType: "type",
							Project:     "project",
							NetworkInterface: []*config.NetworkInterface{
								{},
							},
							Zone: "zone",
						},
						Lifetime: &config.TimePeriod{
							Time: &config.TimePeriod_Seconds{
								Seconds: 3600,
							},
						},
						Prefix: "prefix",
					})
				})

				Convey("update doesn't erase currentAmount", func() {
					So(datastore.Put(c, &model.Config{
						ID: "id",
						Config: &config.Config{
							Amount: &config.Amount{
								Max: 20,
								Min: 10,
							},
							CurrentAmount: 15,
							Owner: []string{
								"owners",
							},
							Prefix: "prefix",
						},
					}), ShouldBeNil)

					cfg, err := srv.Ensure(c, &config.EnsureRequest{
						Id: "id",
						Config: &config.Config{
							Amount: &config.Amount{
								Max: 100,
								Min: 50,
							},
							Attributes: &config.VM{
								Disk: []*config.Disk{
									{},
								},
								MachineType: "type",
								NetworkInterface: []*config.NetworkInterface{
									{},
								},
								Project: "project",
								Zone:    "zone",
							},
							Lifetime: &config.TimePeriod{
								Time: &config.TimePeriod_Seconds{
									Seconds: 3600,
								},
							},
							Prefix: "prefix",
						},
					})
					So(err, ShouldBeNil)
					So(cfg, ShouldResembleProto, &config.Config{
						Amount: &config.Amount{
							Max: 100,
							Min: 50,
						},
						CurrentAmount: 15, // Same as before.
						Attributes: &config.VM{
							Disk: []*config.Disk{
								{},
							},
							MachineType: "type",
							Project:     "project",
							NetworkInterface: []*config.NetworkInterface{
								{},
							},
							Zone: "zone",
						},
						Lifetime: &config.TimePeriod{
							Time: &config.TimePeriod_Seconds{
								Seconds: 3600,
							},
						},
						Prefix: "prefix",
					})
				})

				Convey("update doesn't erase DUTs", func() {
					So(datastore.Put(c, &model.Config{
						ID: "id",
						Config: &config.Config{
							Duts: map[string]*emptypb.Empty{
								"dut-1": {},
								"dut-2": {},
							},
							Owner: []string{
								"owners",
							},
							Prefix: "prefix",
						},
					}), ShouldBeNil)

					cfg, err := srv.Ensure(c, &config.EnsureRequest{
						Id: "id",
						Config: &config.Config{
							Amount: &config.Amount{
								Max: 100,
								Min: 50,
							},
							Attributes: &config.VM{
								Disk: []*config.Disk{
									{},
								},
								MachineType: "type",
								NetworkInterface: []*config.NetworkInterface{
									{},
								},
								Project: "project",
								Zone:    "zone",
							},
							Lifetime: &config.TimePeriod{
								Time: &config.TimePeriod_Seconds{
									Seconds: 3600,
								},
							},
							Prefix: "prefix",
						},
					})
					So(err, ShouldBeNil)
					So(cfg, ShouldResembleProto, &config.Config{
						Amount: &config.Amount{
							Max: 100,
							Min: 50,
						},
						CurrentAmount: 0,
						Attributes: &config.VM{
							Disk: []*config.Disk{
								{},
							},
							MachineType: "type",
							Project:     "project",
							NetworkInterface: []*config.NetworkInterface{
								{},
							},
							Zone: "zone",
						},
						Lifetime: &config.TimePeriod{
							Time: &config.TimePeriod_Seconds{
								Seconds: 3600,
							},
						},
						Prefix: "prefix",
						// Same as before.
						Duts: map[string]*emptypb.Empty{
							"dut-1": {},
							"dut-2": {},
						},
					})
				})
			})
		})

		Convey("Get", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					cfg, err := srv.Get(c, nil)
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})

				Convey("empty", func() {
					cfg, err := srv.Get(c, &config.GetRequest{})
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
				})

				Convey("unauthorized owners", func() {
					c = auth.WithState(c, &authtest.FakeState{
						IdentityGroups: []string{"owners1"},
					})
					So(datastore.Put(c, &model.Config{
						ID: "id",
						Config: &config.Config{
							Prefix: "prefix",
							Owner: []string{
								"owners2",
							},
						},
					}), ShouldBeNil)
					cfg, err := srv.Get(c, &config.GetRequest{
						Id: "id",
					})
					So(err, ShouldErrLike, "no config found with ID \"id\" or unauthorized user")
					So(cfg, ShouldBeNil)
				})
			})

			Convey("valid", func() {
				Convey("not found", func() {
					cfg, err := srv.Get(c, &config.GetRequest{
						Id: "id",
					})
					So(err, ShouldErrLike, "no config found with ID \"id\" or unauthorized user")
					So(cfg, ShouldBeNil)
				})

				Convey("found", func() {
					c = auth.WithState(c, &authtest.FakeState{
						IdentityGroups: []string{"owners"},
					})
					So(datastore.Put(c, &model.Config{
						ID: "id",
						Config: &config.Config{
							Prefix: "prefix",
							Owner: []string{
								"owners",
							},
						},
					}), ShouldBeNil)
					cfg, err := srv.Get(c, &config.GetRequest{
						Id: "id",
					})
					So(err, ShouldBeNil)
					So(cfg, ShouldResembleProto, &config.Config{
						Prefix: "prefix",
						Owner: []string{
							"owners",
						},
					})
				})
			})
		})

		Convey("List", func() {
			Convey("invalid", func() {
				Convey("page token", func() {
					req := &config.ListRequest{
						PageToken: "token",
					}
					_, err := srv.List(c, req)
					So(err, ShouldErrLike, "invalid page token")
				})
			})

			Convey("valid", func() {
				Convey("nil", func() {
					Convey("none", func() {
						rsp, err := srv.List(c, nil)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldBeEmpty)
					})

					Convey("one", func() {
						So(datastore.Put(c, &model.Config{
							ID: "id",
						}), ShouldBeNil)
						rsp, err := srv.List(c, nil)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 1)
					})
				})

				Convey("empty", func() {
					Convey("none", func() {
						req := &config.ListRequest{}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldBeEmpty)
					})

					Convey("one", func() {
						So(datastore.Put(c, &model.Config{
							ID: "id",
						}), ShouldBeNil)
						req := &config.ListRequest{}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 1)
					})
				})

				Convey("pages", func() {
					So(datastore.Put(c, &model.Config{ID: "id1"}), ShouldBeNil)
					So(datastore.Put(c, &model.Config{ID: "id2"}), ShouldBeNil)
					So(datastore.Put(c, &model.Config{ID: "id3"}), ShouldBeNil)

					Convey("default", func() {
						req := &config.ListRequest{}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldNotBeEmpty)
					})

					Convey("one", func() {
						req := &config.ListRequest{
							PageSize: 1,
						}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 1)
						So(rsp.NextPageToken, ShouldNotBeEmpty)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 1)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 1)
						So(rsp.NextPageToken, ShouldBeEmpty)
					})

					Convey("two", func() {
						req := &config.ListRequest{
							PageSize: 2,
						}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 2)
						So(rsp.NextPageToken, ShouldNotBeEmpty)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 1)
						So(rsp.NextPageToken, ShouldBeEmpty)
					})

					Convey("many", func() {
						req := &config.ListRequest{
							PageSize: 200,
						}
						rsp, err := srv.List(c, req)
						So(err, ShouldBeNil)
						So(rsp.Configs, ShouldHaveLength, 3)
						So(rsp.NextPageToken, ShouldBeEmpty)
					})
				})
			})
		})

		Convey("Update", func() {
			c = auth.WithState(c, &authtest.FakeState{
				IdentityGroups: []string{"owners"},
			})
			So(datastore.Put(c, &model.Config{
				ID: "id",
				Config: &config.Config{
					Amount: &config.Amount{
						Max: 3,
						Min: 1,
					},
					CurrentAmount: 1,
					Owner: []string{
						"owners",
					},
					Prefix: "prefix",
					Duts:   map[string]*emptypb.Empty{"dut1": {}},
				},
			}), ShouldBeNil)

			Convey("invalid", func() {
				Convey("nil", func() {
					cfg, err := srv.Update(c, nil)
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
					mdl := &model.Config{
						ID: "id",
					}
					err = datastore.Get(c, mdl)
					So(err, ShouldBeNil)
					So(mdl.Config.CurrentAmount, ShouldEqual, 1)
				})

				Convey("empty", func() {
					cfg, err := srv.Update(c, &config.UpdateRequest{})
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
					mdl := &model.Config{
						ID: "id",
					}
					err = datastore.Get(c, mdl)
					So(err, ShouldBeNil)
					So(mdl.Config.CurrentAmount, ShouldEqual, 1)
				})

				Convey("id", func() {
					cfg, err := srv.Update(c, &config.UpdateRequest{
						UpdateMask: &field_mask.FieldMask{
							Paths: []string{
								"config.current_amount",
							},
						},
					})
					So(err, ShouldErrLike, "ID is required")
					So(cfg, ShouldBeNil)
					mdl := &model.Config{
						ID: "id",
					}
					err = datastore.Get(c, mdl)
					So(err, ShouldBeNil)
					So(mdl.Config.CurrentAmount, ShouldEqual, 1)
				})

				Convey("update mask", func() {
					Convey("missing", func() {
						cfg, err := srv.Update(c, &config.UpdateRequest{
							Id: "id",
						})
						So(err, ShouldErrLike, "update mask is required")
						So(cfg, ShouldBeNil)
						mdl := &model.Config{
							ID: "id",
						}
						err = datastore.Get(c, mdl)
						So(err, ShouldBeNil)
						So(mdl.Config.CurrentAmount, ShouldEqual, 1)
					})

					Convey("empty", func() {
						cfg, err := srv.Update(c, &config.UpdateRequest{
							Id: "id",
							UpdateMask: &field_mask.FieldMask{
								Paths: []string{},
							},
						})
						So(err, ShouldErrLike, "update mask is required")
						So(cfg, ShouldBeNil)
						mdl := &model.Config{
							ID: "id",
						}
						err = datastore.Get(c, mdl)
						So(err, ShouldBeNil)
						So(mdl.Config.CurrentAmount, ShouldEqual, 1)
					})

					Convey("invalid mask", func() {
						cfg, err := srv.Update(c, &config.UpdateRequest{
							Id: "id",
							UpdateMask: &field_mask.FieldMask{
								Paths: []string{
									"config.amount.default",
								},
							},
						})
						So(err, ShouldErrLike, "invalid or immutable")
						So(cfg, ShouldBeNil)
						mdl := &model.Config{
							ID: "id",
						}
						err = datastore.Get(c, mdl)
						So(err, ShouldBeNil)
						So(mdl.Config.CurrentAmount, ShouldEqual, 1)
					})

					Convey("immutable field", func() {
						cfg, err := srv.Update(c, &config.UpdateRequest{
							Id: "id",
							UpdateMask: &field_mask.FieldMask{
								Paths: []string{
									"config.prefix",
								},
							},
						})
						So(err, ShouldErrLike, "invalid or immutable")
						So(cfg, ShouldBeNil)
						mdl := &model.Config{
							ID: "id",
						}
						err = datastore.Get(c, mdl)
						So(err, ShouldBeNil)
						So(mdl.Config.CurrentAmount, ShouldEqual, 1)
					})
				})
			})

			Convey("valid", func() {
				Convey("unauthorized", func() {
					c = auth.WithState(c, &authtest.FakeState{})
					cfg, err := srv.Update(c, &config.UpdateRequest{
						Id: "id",
						Config: &config.Config{
							CurrentAmount: 2,
						},
						UpdateMask: &field_mask.FieldMask{
							Paths: []string{
								"config.current_amount",
							},
						},
					})
					So(err, ShouldErrLike, "unauthorized user")
					So(cfg, ShouldBeNil)
					mdl := &model.Config{
						ID: "id",
					}
					err = datastore.Get(c, mdl)
					So(err, ShouldBeNil)
					So(mdl.Config.CurrentAmount, ShouldEqual, 1)
				})

				Convey("authorized", func() {
					Convey("CurrentAmount", func() {
						Convey("min", func() {
							cfg, err := srv.Update(c, &config.UpdateRequest{
								Id: "id",
								UpdateMask: &field_mask.FieldMask{
									Paths: []string{
										"config.current_amount",
									},
								},
							})
							So(err, ShouldBeNil)
							So(cfg.CurrentAmount, ShouldEqual, 1)
							mdl := &model.Config{
								ID: "id",
							}
							err = datastore.Get(c, mdl)
							So(err, ShouldBeNil)
							So(mdl.Config.CurrentAmount, ShouldEqual, 1)
						})

						Convey("max", func() {
							cfg, err := srv.Update(c, &config.UpdateRequest{
								Id: "id",
								Config: &config.Config{
									CurrentAmount: 4,
								},
								UpdateMask: &field_mask.FieldMask{
									Paths: []string{
										"config.current_amount",
									},
								},
							})
							So(err, ShouldBeNil)
							So(cfg.CurrentAmount, ShouldEqual, 3)
							mdl := &model.Config{
								ID: "id",
							}
							err = datastore.Get(c, mdl)
							So(err, ShouldBeNil)
							So(mdl.Config.CurrentAmount, ShouldEqual, 3)
						})

						Convey("updates", func() {
							cfg, err := srv.Update(c, &config.UpdateRequest{
								Id: "id",
								Config: &config.Config{
									CurrentAmount: 2,
								},
								UpdateMask: &field_mask.FieldMask{
									Paths: []string{
										"config.current_amount",
									},
								},
							})
							So(err, ShouldBeNil)
							So(cfg.CurrentAmount, ShouldEqual, 2)
							mdl := &model.Config{
								ID: "id",
							}
							err = datastore.Get(c, mdl)
							So(err, ShouldBeNil)
							So(mdl.Config.CurrentAmount, ShouldEqual, 2)
						})
					})
					Convey("duts", func() {
						Convey("updates", func() {
							cfg, err := srv.Update(c, &config.UpdateRequest{
								Id: "id",
								Config: &config.Config{
									Duts: map[string]*emptypb.Empty{
										"hello": {},
										"world": {},
									},
								},
								UpdateMask: &field_mask.FieldMask{
									Paths: []string{
										"config.duts",
									},
								},
							})
							So(err, ShouldBeNil)
							So(cfg.CurrentAmount, ShouldEqual, 2)
							So(cfg.Duts, ShouldResembleProto, map[string]*emptypb.Empty{
								"hello": {},
								"world": {},
							})
							mdl := &model.Config{
								ID: "id",
							}
							err = datastore.Get(c, mdl)
							So(err, ShouldBeNil)
							So(mdl.Config.CurrentAmount, ShouldEqual, 2)
							So(mdl.Config.Duts, ShouldResembleProto, map[string]*emptypb.Empty{
								"hello": {},
								"world": {},
							})
						})
					})
				})
			})
		})
	})
	Convey("equalDuts", t, func() {
		Convey("fail", func() {
			Convey("different length", func() {
				s1 := map[string]*emptypb.Empty{
					"dut1": {},
				}
				s2 := map[string]*emptypb.Empty{
					"dut1": {},
					"dut2": {},
				}
				isEqual := dutsEqual(s1, s2)
				So(isEqual, ShouldBeFalse)
			})
			Convey("different keys", func() {
				s1 := map[string]*emptypb.Empty{
					"dut1": {},
				}
				s2 := map[string]*emptypb.Empty{
					"dut2": {},
				}
				isEqual := dutsEqual(s1, s2)
				So(isEqual, ShouldBeFalse)
			})
		})
		Convey("pass", func() {
			Convey("same keys", func() {
				s1 := map[string]*emptypb.Empty{
					"dut1": {},
					"dut2": {},
				}
				s2 := map[string]*emptypb.Empty{
					"dut1": {},
					"dut2": {},
				}
				isEqual := dutsEqual(s1, s2)
				So(isEqual, ShouldBeTrue)
			})
			Convey("empty", func() {
				s1 := map[string]*emptypb.Empty{}
				s2 := map[string]*emptypb.Empty{}
				isEqual := dutsEqual(s1, s2)
				So(isEqual, ShouldBeTrue)
			})
		})
	})
}
