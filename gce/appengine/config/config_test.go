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
	"go.chromium.org/luci/gce/api/projects/v1"
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
					},
				}))
				_, err := fetch(c)
				So(err, ShouldErrLike, "failed to load")
			})

			Convey("projects", func() {
				c := withInterface(gae.UseWithAppID(context.Background(), "gce"), memory.New(map[config.Set]memory.Files{
					"services/gce": map[string]string{
						projectsFile: "invalid",
					},
				}))
				_, err := fetch(c)
				So(err, ShouldErrLike, "failed to load")
			})

			Convey("vms", func() {
				c := withInterface(gae.UseWithAppID(context.Background(), "gce"), memory.New(map[config.Set]memory.Files{
					"services/gce": map[string]string{
						vmsFile: "invalid",
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
				So(cfg.Projects, ShouldResemble, &projects.Configs{})
				So(cfg.VMs, ShouldResemble, &gce.Configs{})
			})

			Convey("implicit", func() {
				c := withInterface(gae.UseWithAppID(context.Background(), "example.com:gce"), memory.New(map[config.Set]memory.Files{
					"services/gce": {},
				}))
				cfg, err := fetch(c)
				So(err, ShouldBeNil)
				So(cfg.Kinds, ShouldResemble, &gce.Kinds{})
				So(cfg.Projects, ShouldResemble, &projects.Configs{})
				So(cfg.VMs, ShouldResemble, &gce.Configs{})
			})

			Convey("explicit", func() {
				c := withInterface(gae.UseWithAppID(context.Background(), "example.com:gce"), memory.New(map[config.Set]memory.Files{
					"services/gce": {
						kindsFile:    "",
						projectsFile: "",
						vmsFile:      "",
					},
				}))
				cfg, err := fetch(c)
				So(err, ShouldBeNil)
				So(cfg.Kinds, ShouldResemble, &gce.Kinds{})
				So(cfg.Projects, ShouldResemble, &projects.Configs{})
				So(cfg.VMs, ShouldResemble, &gce.Configs{})
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

		Convey("revision", func() {
			cfg := &Config{
				revision: "revision",
				Projects: &projects.Configs{
					Project: []*projects.Config{
						{},
					},
				},
				VMs: &gce.Configs{
					Vms: []*gce.Config{
						{},
					},
				},
			}
			So(normalize(c, cfg), ShouldBeNil)
			So(cfg.Projects.GetProject()[0].Revision, ShouldEqual, "revision")
			So(cfg.VMs.GetVms()[0].Revision, ShouldEqual, "revision")
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

func TestSyncPrjs(t *testing.T) {
	t.Parallel()

	Convey("syncPrjs", t, func() {
		srv := &rpc.Projects{}
		c := withProjServer(context.Background(), srv)

		Convey("nil", func() {
			prjs := []*projects.Config{}
			So(syncPrjs(c, prjs), ShouldBeNil)
			rsp, err := srv.List(c, &projects.ListRequest{})
			So(err, ShouldBeNil)
			So(rsp.Projects, ShouldBeEmpty)
		})

		Convey("creates", func() {
			prjs := []*projects.Config{
				{
					Project: "project",
				},
			}
			So(syncPrjs(c, prjs), ShouldBeNil)
			rsp, err := srv.List(c, &projects.ListRequest{})
			So(err, ShouldBeNil)
			So(rsp.Projects, ShouldHaveLength, 1)
			So(rsp.Projects[0].Project, ShouldEqual, "project")
		})

		Convey("updates", func() {
			srv.Ensure(c, &projects.EnsureRequest{
				Id: "project",
				Project: &projects.Config{
					Project: "project",
					Region: []string{
						"region1",
					},
					Revision: "revision-1",
				},
			})
			prjs := []*projects.Config{
				{
					Project: "project",
					Region: []string{
						"region2",
						"region3",
					},
					Revision: "revision-2",
				},
			}
			So(syncPrjs(c, prjs), ShouldBeNil)
			rsp, err := srv.List(c, &projects.ListRequest{})
			So(err, ShouldBeNil)
			So(rsp.Projects, ShouldHaveLength, 1)
			So(rsp.Projects, ShouldContain, &projects.Config{
				Project: "project",
				Region: []string{
					"region2",
					"region3",
				},
				Revision: "revision-2",
			})
		})

		Convey("deletes", func() {
			srv.Ensure(c, &projects.EnsureRequest{
				Id: "project",
				Project: &projects.Config{
					Project: "project",
				},
			})
			So(syncPrjs(c, nil), ShouldBeNil)
			rsp, err := srv.List(c, &projects.ListRequest{})
			So(err, ShouldBeNil)
			So(rsp.Projects, ShouldBeEmpty)
		})
	})
}

func TestSyncVMs(t *testing.T) {
	t.Parallel()

	Convey("syncVMs", t, func() {
		srv := &rpc.Config{}
		c := withVMsServer(context.Background(), srv)

		Convey("nil", func() {
			vms := []*gce.Config{}
			So(syncVMs(c, vms), ShouldBeNil)
			rsp, err := srv.List(c, &gce.ListRequest{})
			So(err, ShouldBeNil)
			So(rsp.Configs, ShouldBeEmpty)
		})

		Convey("creates", func() {
			vms := []*gce.Config{
				{
					Prefix: "prefix",
				},
			}
			So(syncVMs(c, vms), ShouldBeNil)
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
					Prefix:   "prefix",
					Revision: "revision-1",
				},
			})
			vms := []*gce.Config{
				{
					Amount: &gce.Amount{
						Default: 2,
					},
					Prefix:   "prefix",
					Revision: "revision-2",
				},
			}
			So(syncVMs(c, vms), ShouldBeNil)
			rsp, err := srv.List(c, &gce.ListRequest{})
			So(err, ShouldBeNil)
			So(rsp.Configs, ShouldHaveLength, 1)
			So(rsp.Configs, ShouldContain, &gce.Config{
				Amount: &gce.Amount{
					Default: 2,
				},
				Prefix:   "prefix",
				Revision: "revision-2",
			})
		})

		Convey("deletes", func() {
			srv.Ensure(c, &gce.EnsureRequest{
				Id: "prefix",
				Config: &gce.Config{
					Prefix: "prefix",
				},
			})
			So(syncVMs(c, nil), ShouldBeNil)
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
				So(validate(c, cfg), ShouldErrLike, "is required")
			})

			Convey("projects", func() {
				cfg := &Config{
					Projects: &projects.Configs{
						Project: []*projects.Config{
							{},
						},
					},
				}
				So(validate(c, cfg), ShouldErrLike, "is required")
			})

			Convey("vms", func() {
				cfg := &Config{
					VMs: &gce.Configs{
						Vms: []*gce.Config{
							{},
						},
					},
				}
				So(validate(c, cfg), ShouldErrLike, "is required")
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
