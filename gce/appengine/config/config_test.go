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

package config

import (
	"context"
	"testing"

	gae "go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/memory"

	gce "go.chromium.org/luci/gce/api/config/v1"
	rpc "go.chromium.org/luci/gce/appengine/rpc/memory"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDeref(t *testing.T) {
	t.Parallel()

	Convey("deref", t, func() {
		Convey("empty", func() {
			c := withInterface(gae.Use(context.Background()), memory.New(nil))
			cfg := &Config{}
			So(deref(c, cfg), ShouldBeNil)
			So(cfg.VMs.GetVms(), ShouldHaveLength, 0)
		})

		Convey("missing", func() {
			c := withInterface(gae.Use(context.Background()), memory.New(nil))
			cfg := &Config{
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Attributes: &gce.VM{
								Metadata: []*gce.Metadata{
									{
										Metadata: &gce.Metadata_FromFile{
											FromFile: "key:file",
										},
									},
								},
							},
						},
					},
				},
			}
			So(deref(c, cfg), ShouldErrLike, "failed to fetch")
		})

		Convey("revision", func() {
			c := withInterface(gae.UseWithAppID(context.Background(), "gce"), memory.New(map[config.Set]memory.Files{
				"services/gce": map[string]string{
					"file": "val",
				},
			}))
			cfg := &Config{
				revision: "revision",
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Attributes: &gce.VM{
								Metadata: []*gce.Metadata{
									{
										Metadata: &gce.Metadata_FromFile{
											FromFile: "key:file",
										},
									},
								},
							},
						},
					},
				},
			}
			So(deref(c, cfg), ShouldErrLike, "config revision mismatch")
			So(cfg.VMs.Vms[0].Attributes.Metadata, ShouldContain, &gce.Metadata{
				Metadata: &gce.Metadata_FromFile{
					FromFile: "key:file",
				},
			})
		})

		Convey("dereferences", func() {
			c := withInterface(gae.UseWithAppID(context.Background(), "gce"), memory.New(map[config.Set]memory.Files{
				"services/gce": map[string]string{
					"metadata/file": "val2",
				},
			}))
			cfg := &Config{
				revision: "12f8812df1e3182615a8f105db567f8d792a1440",
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Attributes: &gce.VM{
								Metadata: []*gce.Metadata{
									{
										Metadata: &gce.Metadata_FromText{
											FromText: "key:val1",
										},
									},
									{
										Metadata: &gce.Metadata_FromFile{
											FromFile: "key:metadata/file",
										},
									},
								},
							},
						},
					},
				},
			}
			So(deref(c, cfg), ShouldBeNil)
			So(cfg.VMs.Vms[0].Attributes.Metadata, ShouldContain, &gce.Metadata{
				Metadata: &gce.Metadata_FromText{
					FromText: "key:val1",
				},
			})
			So(cfg.VMs.Vms[0].Attributes.Metadata, ShouldContain, &gce.Metadata{
				Metadata: &gce.Metadata_FromText{
					FromText: "key:val2",
				},
			})
		})
	})
}

func TestFetch(t *testing.T) {
	t.Parallel()

	Convey("fetch", t, func() {
		Convey("invalid", func() {
			Convey("kinds", func() {
				c := withInterface(gae.UseWithAppID(context.Background(), "gce"), memory.New(map[config.Set]memory.Files{
					"services/gce": map[string]string{
						kindsFile: "invalid",
						vmsFile:   "",
					},
				}))
				_, err := fetch(c)
				So(err, ShouldErrLike, "failed to load")
			})

			Convey("vms", func() {
				c := withInterface(gae.UseWithAppID(context.Background(), "gce"), memory.New(map[config.Set]memory.Files{
					"services/gce": map[string]string{
						kindsFile: "",
						vmsFile:   "invalid",
					},
				}))
				_, err := fetch(c)
				So(err, ShouldErrLike, "failed to load")
			})
		})

		Convey("empty", func() {
			Convey("missing", func() {
				c := withInterface(gae.Use(context.Background()), memory.New(nil))
				cfg, err := fetch(c)
				So(err, ShouldBeNil)
				So(cfg.Kinds, ShouldResemble, &gce.Kinds{})
				So(cfg.VMs, ShouldResemble, &gce.Configs{})
			})

			Convey("implicit", func() {
				c := withInterface(gae.UseWithAppID(context.Background(), "example.com:gce"), memory.New(map[config.Set]memory.Files{
					"services/gce": {},
				}))
				cfgs, err := fetch(c)
				So(err, ShouldBeNil)
				So(cfgs.Kinds, ShouldResemble, &gce.Kinds{})
				So(cfgs.VMs, ShouldResemble, &gce.Configs{})
			})

			Convey("explicit", func() {
				c := withInterface(gae.UseWithAppID(context.Background(), "example.com:gce"), memory.New(map[config.Set]memory.Files{
					"services/gce": {
						kindsFile: "",
						vmsFile:   "",
					},
				}))
				cfgs, err := fetch(c)
				So(err, ShouldBeNil)
				So(cfgs.Kinds, ShouldResemble, &gce.Kinds{})
				So(cfgs.VMs, ShouldResemble, &gce.Configs{})
			})
		})
	})
}

func TestMerge(t *testing.T) {
	t.Parallel()

	Convey("merge", t, func() {
		c := context.Background()

		Convey("empty", func() {
			cfg := &Config{
				Kinds: &gce.Kinds{
					Kind: []*gce.Kind{
						{
							Name: "kind",
							Attributes: &gce.VM{
								Project: "project",
								Zone:    "zone",
							},
						},
					},
				},
			}
			So(merge(c, cfg), ShouldBeNil)
			So(cfg.VMs.GetVms(), ShouldHaveLength, 0)
		})

		Convey("unknown kind", func() {
			cfg := &Config{
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Kind: "kind",
						},
					},
				},
			}
			So(merge(c, cfg), ShouldErrLike, "unknown kind")
		})

		Convey("merged", func() {
			cfg := &Config{
				Kinds: &gce.Kinds{
					Kind: []*gce.Kind{
						{
							Name: "kind",
							Attributes: &gce.VM{
								Disk: []*gce.Disk{
									{
										Image: "image 1",
									},
								},
								Metadata: []*gce.Metadata{
									{
										Metadata: &gce.Metadata_FromText{
											FromText: "metadata 1",
										},
									},
								},
								NetworkInterface: []*gce.NetworkInterface{
									{
										Network: "network 1",
									},
								},
								Project: "project 1",
								Tag: []string{
									"tag 1",
								},
								Zone: "zone",
							},
						},
					},
				},
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Attributes: &gce.VM{
								MachineType: "type",
							},
						},
						{
							Kind: "kind",
							Attributes: &gce.VM{
								Disk: []*gce.Disk{
									{
										Image: "image 2",
									},
								},
								Metadata: []*gce.Metadata{
									{
										Metadata: &gce.Metadata_FromFile{
											FromFile: "metadata 2",
										},
									},
								},
								NetworkInterface: []*gce.NetworkInterface{
									{
										Network: "network 2",
									},
								},
								Project: "project 2",
								Tag: []string{
									"tag 2",
								},
							},
						},
					},
				},
			}
			So(merge(c, cfg), ShouldBeNil)
			So(cfg.VMs.GetVms(), ShouldResemble, []*gce.Config{
				{
					Attributes: &gce.VM{
						MachineType: "type",
					},
				},
				{
					Kind: "kind",
					Attributes: &gce.VM{
						Disk: []*gce.Disk{
							{
								Image: "image 2",
							},
						},
						Metadata: []*gce.Metadata{
							{
								Metadata: &gce.Metadata_FromFile{
									FromFile: "metadata 2",
								},
							},
						},
						NetworkInterface: []*gce.NetworkInterface{
							{
								Network: "network 2",
							},
						},
						Project: "project 2",
						Tag: []string{
							"tag 2",
						},
						Zone: "zone",
					},
				},
			})
		})
	})
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	Convey("normalize", t, func() {
		c := context.Background()

		Convey("empty", func() {
			cfg := &Config{}
			So(normalize(c, cfg), ShouldBeNil)
			So(cfg.VMs.GetVms(), ShouldHaveLength, 0)
		})

		Convey("amount", func() {
			cfg := &Config{
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Amount: &gce.Amount{
								Change: []*gce.Schedule{
									{
										Length: &gce.TimePeriod{
											Time: &gce.TimePeriod_Duration{
												Duration: "1h",
											},
										},
									},
								},
							},
						},
					},
				},
			}
			So(normalize(c, cfg), ShouldBeNil)
			So(cfg.VMs.GetVms()[0].Amount.Change[0].Length.GetSeconds(), ShouldEqual, 3600)
		})

		Convey("lifetime", func() {
			cfg := &Config{
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Lifetime: &gce.TimePeriod{
								Time: &gce.TimePeriod_Duration{
									Duration: "1h",
								},
							},
						},
					},
				},
			}
			So(normalize(c, cfg), ShouldBeNil)
			So(cfg.VMs.GetVms()[0].Lifetime.GetSeconds(), ShouldEqual, 3600)
		})

		Convey("timeout", func() {
			cfg := &Config{
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Timeout: &gce.TimePeriod{
								Time: &gce.TimePeriod_Duration{
									Duration: "1h",
								},
							},
						},
					},
				},
			}
			So(normalize(c, cfg), ShouldBeNil)
			So(cfg.VMs.GetVms()[0].Timeout.GetSeconds(), ShouldEqual, 3600)
		})
	})
}

func TestSync(t *testing.T) {
	t.Parallel()

	Convey("sync", t, func() {
		srv := &rpc.Config{}
		c := withServer(context.Background(), srv)

		Convey("none", func() {
			cfg := &Config{}
			So(sync(c, cfg), ShouldBeNil)
			rsp, err := srv.List(c, &gce.ListRequest{})
			So(err, ShouldBeNil)
			So(rsp.Configs, ShouldBeEmpty)
		})

		Convey("creates", func() {
			cfg := &Config{
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Prefix: "prefix",
						},
					},
				},
			}
			So(sync(c, cfg), ShouldBeNil)
			rsp, err := srv.List(c, &gce.ListRequest{})
			So(err, ShouldBeNil)
			So(rsp.Configs, ShouldHaveLength, 1)
			So(rsp.Configs[0].Prefix, ShouldEqual, "prefix")
		})

		Convey("updates", func() {
			srv.Ensure(c, &gce.EnsureRequest{
				Id: "prefix",
				Config: &gce.Config{
					Amount: &gce.Amount{
						Default: 1,
					},
					Prefix: "prefix",
				},
			})
			cfg := &Config{
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{
							Amount: &gce.Amount{
								Default: 2,
							},
							Prefix: "prefix",
						},
					},
				},
			}
			So(sync(c, cfg), ShouldBeNil)
			rsp, err := srv.List(c, &gce.ListRequest{})
			So(err, ShouldBeNil)
			So(rsp.Configs, ShouldHaveLength, 1)
			So(rsp.Configs, ShouldContain, &gce.Config{
				Amount: &gce.Amount{
					Default: 2,
				},
				Prefix: "prefix",
			})
		})

		Convey("deletes", func() {
			srv.Ensure(c, &gce.EnsureRequest{
				Id: "prefix1",
				Config: &gce.Config{
					Prefix: "prefix1",
				},
			})
			cfg := &Config{}
			So(sync(c, cfg), ShouldBeNil)
			rsp, err := srv.List(c, &gce.ListRequest{})
			So(err, ShouldBeNil)
			So(rsp.Configs, ShouldBeEmpty)
		})
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()

	Convey("validate", t, func() {
		c := context.Background()

		Convey("invalid", func() {
			Convey("kinds", func() {
				cfg := &Config{
					Kinds: &gce.Kinds{
						Kind: []*gce.Kind{
							{},
						},
					},
				}
				err := validate(c, cfg)
				So(err, ShouldErrLike, "is required")
			})

			Convey("configs", func() {
				cfg := &Config{
					VMs: &gce.Configs{
						Vms: []*gce.Config{
							{},
						},
					},
				}
				err := validate(c, cfg)
				So(err, ShouldErrLike, "is required")
			})
		})

		Convey("valid", func() {
			Convey("empty", func() {
				cfg := &Config{}
				err := validate(c, cfg)
				So(err, ShouldBeNil)
			})
		})
	})
}
