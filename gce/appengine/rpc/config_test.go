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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Config", t, func(t *ftt.Test) {
		srv := &Config{}
		c := memory.Use(context.Background())
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)

		t.Run("Delete", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					cfg, err := srv.Delete(c, nil)
					assert.Loosely(t, err, should.ErrLike("ID is required"))
					assert.Loosely(t, cfg, should.BeNil)
				})

				t.Run("empty", func(t *ftt.Test) {
					cfg, err := srv.Delete(c, &config.DeleteRequest{})
					assert.Loosely(t, err, should.ErrLike("ID is required"))
					assert.Loosely(t, cfg, should.BeNil)
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(c, &model.Config{
					ID: "id",
				}), should.BeNil)
				cfg, err := srv.Delete(c, &config.DeleteRequest{
					Id: "id",
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg, should.Resemble(&emptypb.Empty{}))
				err = datastore.Get(c, &model.Config{
					ID: "id",
				})
				assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))
			})
		})

		t.Run("Ensure", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					cfg, err := srv.Ensure(c, nil)
					assert.Loosely(t, err, should.ErrLike("ID is required"))
					assert.Loosely(t, cfg, should.BeNil)
				})

				t.Run("empty", func(t *ftt.Test) {
					cfg, err := srv.Ensure(c, &config.EnsureRequest{})
					assert.Loosely(t, err, should.ErrLike("ID is required"))
					assert.Loosely(t, cfg, should.BeNil)
				})

				t.Run("ID", func(t *ftt.Test) {
					cfg, err := srv.Ensure(c, &config.EnsureRequest{
						Config: &config.Config{},
					})
					assert.Loosely(t, err, should.ErrLike("ID is required"))
					assert.Loosely(t, cfg, should.BeNil)
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				t.Run("new", func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, cfg, should.Resemble(&config.Config{
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
					}))
				})

				t.Run("update doesn't erase currentAmount", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(c, &model.Config{
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
					}), should.BeNil)

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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, cfg, should.Resemble(&config.Config{
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
					}))
				})

				t.Run("update doesn't erase DUTs", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(c, &model.Config{
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
					}), should.BeNil)

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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, cfg, should.Resemble(&config.Config{
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
					}))
				})
			})
		})

		t.Run("Get", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					cfg, err := srv.Get(c, nil)
					assert.Loosely(t, err, should.ErrLike("ID is required"))
					assert.Loosely(t, cfg, should.BeNil)
				})

				t.Run("empty", func(t *ftt.Test) {
					cfg, err := srv.Get(c, &config.GetRequest{})
					assert.Loosely(t, err, should.ErrLike("ID is required"))
					assert.Loosely(t, cfg, should.BeNil)
				})

				t.Run("unauthorized owners", func(t *ftt.Test) {
					c = auth.WithState(c, &authtest.FakeState{
						IdentityGroups: []string{"owners1"},
					})
					assert.Loosely(t, datastore.Put(c, &model.Config{
						ID: "id",
						Config: &config.Config{
							Prefix: "prefix",
							Owner: []string{
								"owners2",
							},
						},
					}), should.BeNil)
					cfg, err := srv.Get(c, &config.GetRequest{
						Id: "id",
					})
					assert.Loosely(t, err, should.ErrLike("no config found with ID \"id\" or unauthorized user"))
					assert.Loosely(t, cfg, should.BeNil)
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				t.Run("not found", func(t *ftt.Test) {
					cfg, err := srv.Get(c, &config.GetRequest{
						Id: "id",
					})
					assert.Loosely(t, err, should.ErrLike("no config found with ID \"id\" or unauthorized user"))
					assert.Loosely(t, cfg, should.BeNil)
				})

				t.Run("found", func(t *ftt.Test) {
					c = auth.WithState(c, &authtest.FakeState{
						IdentityGroups: []string{"owners"},
					})
					assert.Loosely(t, datastore.Put(c, &model.Config{
						ID: "id",
						Config: &config.Config{
							Prefix: "prefix",
							Owner: []string{
								"owners",
							},
						},
					}), should.BeNil)
					cfg, err := srv.Get(c, &config.GetRequest{
						Id: "id",
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, cfg, should.Resemble(&config.Config{
						Prefix: "prefix",
						Owner: []string{
							"owners",
						},
					}))
				})
			})
		})

		t.Run("List", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("page token", func(t *ftt.Test) {
					req := &config.ListRequest{
						PageToken: "token",
					}
					_, err := srv.List(c, req)
					assert.Loosely(t, err, should.ErrLike("invalid page token"))
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					t.Run("none", func(t *ftt.Test) {
						rsp, err := srv.List(c, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Configs, should.BeEmpty)
					})

					t.Run("one", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(c, &model.Config{
							ID: "id",
						}), should.BeNil)
						rsp, err := srv.List(c, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Configs, should.HaveLength(1))
					})
				})

				t.Run("empty", func(t *ftt.Test) {
					t.Run("none", func(t *ftt.Test) {
						req := &config.ListRequest{}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Configs, should.BeEmpty)
					})

					t.Run("one", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(c, &model.Config{
							ID: "id",
						}), should.BeNil)
						req := &config.ListRequest{}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Configs, should.HaveLength(1))
					})
				})

				t.Run("pages", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(c, &model.Config{ID: "id1"}), should.BeNil)
					assert.Loosely(t, datastore.Put(c, &model.Config{ID: "id2"}), should.BeNil)
					assert.Loosely(t, datastore.Put(c, &model.Config{ID: "id3"}), should.BeNil)

					t.Run("default", func(t *ftt.Test) {
						req := &config.ListRequest{}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Configs, should.NotBeEmpty)
					})

					t.Run("one", func(t *ftt.Test) {
						req := &config.ListRequest{
							PageSize: 1,
						}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Configs, should.HaveLength(1))
						assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Configs, should.HaveLength(1))

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Configs, should.HaveLength(1))
						assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					})

					t.Run("two", func(t *ftt.Test) {
						req := &config.ListRequest{
							PageSize: 2,
						}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Configs, should.HaveLength(2))
						assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Configs, should.HaveLength(1))
						assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					})

					t.Run("many", func(t *ftt.Test) {
						req := &config.ListRequest{
							PageSize: 200,
						}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Configs, should.HaveLength(3))
						assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					})
				})
			})
		})

		t.Run("Update", func(t *ftt.Test) {
			c = auth.WithState(c, &authtest.FakeState{
				IdentityGroups: []string{"owners"},
			})
			assert.Loosely(t, datastore.Put(c, &model.Config{
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
			}), should.BeNil)

			t.Run("invalid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					cfg, err := srv.Update(c, nil)
					assert.Loosely(t, err, should.ErrLike("ID is required"))
					assert.Loosely(t, cfg, should.BeNil)
					mdl := &model.Config{
						ID: "id",
					}
					err = datastore.Get(c, mdl)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, mdl.Config.CurrentAmount, should.Equal(1))
				})

				t.Run("empty", func(t *ftt.Test) {
					cfg, err := srv.Update(c, &config.UpdateRequest{})
					assert.Loosely(t, err, should.ErrLike("ID is required"))
					assert.Loosely(t, cfg, should.BeNil)
					mdl := &model.Config{
						ID: "id",
					}
					err = datastore.Get(c, mdl)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, mdl.Config.CurrentAmount, should.Equal(1))
				})

				t.Run("id", func(t *ftt.Test) {
					cfg, err := srv.Update(c, &config.UpdateRequest{
						UpdateMask: &field_mask.FieldMask{
							Paths: []string{
								"config.current_amount",
							},
						},
					})
					assert.Loosely(t, err, should.ErrLike("ID is required"))
					assert.Loosely(t, cfg, should.BeNil)
					mdl := &model.Config{
						ID: "id",
					}
					err = datastore.Get(c, mdl)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, mdl.Config.CurrentAmount, should.Equal(1))
				})

				t.Run("update mask", func(t *ftt.Test) {
					t.Run("missing", func(t *ftt.Test) {
						cfg, err := srv.Update(c, &config.UpdateRequest{
							Id: "id",
						})
						assert.Loosely(t, err, should.ErrLike("update mask is required"))
						assert.Loosely(t, cfg, should.BeNil)
						mdl := &model.Config{
							ID: "id",
						}
						err = datastore.Get(c, mdl)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, mdl.Config.CurrentAmount, should.Equal(1))
					})

					t.Run("empty", func(t *ftt.Test) {
						cfg, err := srv.Update(c, &config.UpdateRequest{
							Id: "id",
							UpdateMask: &field_mask.FieldMask{
								Paths: []string{},
							},
						})
						assert.Loosely(t, err, should.ErrLike("update mask is required"))
						assert.Loosely(t, cfg, should.BeNil)
						mdl := &model.Config{
							ID: "id",
						}
						err = datastore.Get(c, mdl)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, mdl.Config.CurrentAmount, should.Equal(1))
					})

					t.Run("invalid mask", func(t *ftt.Test) {
						cfg, err := srv.Update(c, &config.UpdateRequest{
							Id: "id",
							UpdateMask: &field_mask.FieldMask{
								Paths: []string{
									"config.amount.default",
								},
							},
						})
						assert.Loosely(t, err, should.ErrLike("invalid or immutable"))
						assert.Loosely(t, cfg, should.BeNil)
						mdl := &model.Config{
							ID: "id",
						}
						err = datastore.Get(c, mdl)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, mdl.Config.CurrentAmount, should.Equal(1))
					})

					t.Run("immutable field", func(t *ftt.Test) {
						cfg, err := srv.Update(c, &config.UpdateRequest{
							Id: "id",
							UpdateMask: &field_mask.FieldMask{
								Paths: []string{
									"config.prefix",
								},
							},
						})
						assert.Loosely(t, err, should.ErrLike("invalid or immutable"))
						assert.Loosely(t, cfg, should.BeNil)
						mdl := &model.Config{
							ID: "id",
						}
						err = datastore.Get(c, mdl)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, mdl.Config.CurrentAmount, should.Equal(1))
					})
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				t.Run("unauthorized", func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.ErrLike("unauthorized user"))
					assert.Loosely(t, cfg, should.BeNil)
					mdl := &model.Config{
						ID: "id",
					}
					err = datastore.Get(c, mdl)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, mdl.Config.CurrentAmount, should.Equal(1))
				})

				t.Run("authorized", func(t *ftt.Test) {
					t.Run("CurrentAmount", func(t *ftt.Test) {
						t.Run("min", func(t *ftt.Test) {
							cfg, err := srv.Update(c, &config.UpdateRequest{
								Id: "id",
								UpdateMask: &field_mask.FieldMask{
									Paths: []string{
										"config.current_amount",
									},
								},
							})
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, cfg.CurrentAmount, should.Equal(1))
							mdl := &model.Config{
								ID: "id",
							}
							err = datastore.Get(c, mdl)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, mdl.Config.CurrentAmount, should.Equal(1))
						})

						t.Run("max", func(t *ftt.Test) {
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
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, cfg.CurrentAmount, should.Equal(3))
							mdl := &model.Config{
								ID: "id",
							}
							err = datastore.Get(c, mdl)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, mdl.Config.CurrentAmount, should.Equal(3))
						})

						t.Run("updates", func(t *ftt.Test) {
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
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, cfg.CurrentAmount, should.Equal(2))
							mdl := &model.Config{
								ID: "id",
							}
							err = datastore.Get(c, mdl)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, mdl.Config.CurrentAmount, should.Equal(2))
						})
					})
					t.Run("duts", func(t *ftt.Test) {
						t.Run("updates", func(t *ftt.Test) {
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
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, cfg.CurrentAmount, should.Equal(2))
							assert.Loosely(t, cfg.Duts, should.Resemble(map[string]*emptypb.Empty{
								"hello": {},
								"world": {},
							}))
							mdl := &model.Config{
								ID: "id",
							}
							err = datastore.Get(c, mdl)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, mdl.Config.CurrentAmount, should.Equal(2))
							assert.Loosely(t, mdl.Config.Duts, should.Resemble(map[string]*emptypb.Empty{
								"hello": {},
								"world": {},
							}))
						})
					})
				})
			})
		})
	})
	ftt.Run("equalDuts", t, func(t *ftt.Test) {
		t.Run("fail", func(t *ftt.Test) {
			t.Run("different length", func(t *ftt.Test) {
				s1 := map[string]*emptypb.Empty{
					"dut1": {},
				}
				s2 := map[string]*emptypb.Empty{
					"dut1": {},
					"dut2": {},
				}
				isEqual := dutsEqual(s1, s2)
				assert.Loosely(t, isEqual, should.BeFalse)
			})
			t.Run("different keys", func(t *ftt.Test) {
				s1 := map[string]*emptypb.Empty{
					"dut1": {},
				}
				s2 := map[string]*emptypb.Empty{
					"dut2": {},
				}
				isEqual := dutsEqual(s1, s2)
				assert.Loosely(t, isEqual, should.BeFalse)
			})
		})
		t.Run("pass", func(t *ftt.Test) {
			t.Run("same keys", func(t *ftt.Test) {
				s1 := map[string]*emptypb.Empty{
					"dut1": {},
					"dut2": {},
				}
				s2 := map[string]*emptypb.Empty{
					"dut1": {},
					"dut2": {},
				}
				isEqual := dutsEqual(s1, s2)
				assert.Loosely(t, isEqual, should.BeTrue)
			})
			t.Run("empty", func(t *ftt.Test) {
				s1 := map[string]*emptypb.Empty{}
				s2 := map[string]*emptypb.Empty{}
				isEqual := dutsEqual(s1, s2)
				assert.Loosely(t, isEqual, should.BeTrue)
			})
		})
	})
}
