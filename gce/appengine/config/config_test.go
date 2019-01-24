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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDeref(t *testing.T) {
	t.Parallel()

	Convey("deref", t, func() {
		Convey("empty", func() {
			c := withInterface(gae.Use(context.Background()), memory.New(nil))
			cfgs := &gce.Configs{}
			So(deref(c, cfgs), ShouldBeNil)
		})

		Convey("missing", func() {
			c := withInterface(gae.Use(context.Background()), memory.New(nil))
			cfgs := &gce.Configs{
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
			}
			So(deref(c, cfgs), ShouldErrLike, "failed to fetch")
		})

		Convey("dereferences", func() {
			c := withInterface(gae.UseWithAppID(context.Background(), "gce"), memory.New(map[config.Set]memory.Files{
				"services/gce": map[string]string{
					"metadata/file": "val2",
				},
			}))
			cfgs := &gce.Configs{
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
			}
			So(deref(c, cfgs), ShouldBeNil)
			So(cfgs.Vms, ShouldHaveLength, 1)
			So(cfgs.Vms[0].GetAttributes().Metadata, ShouldHaveLength, 2)
			So(cfgs.Vms[0].Attributes.Metadata[0].Metadata, ShouldHaveSameTypeAs, &gce.Metadata_FromText{})
			So(cfgs.Vms[0].Attributes.Metadata[0].Metadata.(*gce.Metadata_FromText).FromText, ShouldEqual, "key:val1")
			So(cfgs.Vms[0].Attributes.Metadata[1].Metadata, ShouldHaveSameTypeAs, &gce.Metadata_FromText{})
			So(cfgs.Vms[0].Attributes.Metadata[1].Metadata.(*gce.Metadata_FromText).FromText, ShouldEqual, "key:val2")
		})
	})
}

func TestFetch(t *testing.T) {
	t.Parallel()

	Convey("fetch", t, func() {
		Convey("missing", func() {
			c := withInterface(gae.Use(context.Background()), memory.New(nil))
			k, v, err := fetch(c)
			So(err, ShouldErrLike, "failed to fetch")
			So(k, ShouldBeNil)
			So(v, ShouldBeNil)
		})

		Convey("invalid", func() {
			Convey("kinds", func() {
				c := withInterface(gae.UseWithAppID(context.Background(), "gce"), memory.New(map[config.Set]memory.Files{
					"services/gce": map[string]string{
						kindsFile: "invalid",
						vmsFile:   "",
					},
				}))
				k, v, err := fetch(c)
				So(err, ShouldErrLike, "failed to load")
				So(k, ShouldBeNil)
				So(v, ShouldBeNil)
			})

			Convey("vms", func() {
				c := withInterface(gae.UseWithAppID(context.Background(), "gce"), memory.New(map[config.Set]memory.Files{
					"services/gce": map[string]string{
						kindsFile: "",
						vmsFile:   "invalid",
					},
				}))
				k, v, err := fetch(c)
				So(err, ShouldErrLike, "failed to load")
				So(k, ShouldBeNil)
				So(v, ShouldBeNil)
			})
		})

		Convey("empty", func() {
			c := withInterface(gae.UseWithAppID(context.Background(), "example.com:gce"), memory.New(map[config.Set]memory.Files{
				"services/gce": {
					kindsFile: "",
					vmsFile:   "",
				},
			}))
			k, v, err := fetch(c)
			So(err, ShouldBeNil)
			So(k, ShouldResemble, &gce.Kinds{})
			So(v, ShouldResemble, &gce.Configs{})
		})
	})
}

func TestMerge(t *testing.T) {
	t.Parallel()

	Convey("merge", t, func() {
		c := context.Background()

		Convey("empty", func() {
			kinds := &gce.Kinds{
				Kind: []*gce.Kind{
					{
						Name: "kind",
						Attributes: &gce.VM{
							Project: "project",
							Zone:    "zone",
						},
					},
				},
			}
			cfgs := &gce.Configs{}
			So(merge(c, kinds, cfgs), ShouldBeNil)
			So(cfgs.GetVms(), ShouldHaveLength, 0)
		})

		Convey("unknown kind", func() {
			kinds := &gce.Kinds{}
			cfgs := &gce.Configs{
				Vms: []*gce.Config{
					{
						Kind: "kind",
					},
				},
			}
			So(merge(c, kinds, cfgs), ShouldErrLike, "unknown kind")
		})

		Convey("merged", func() {
			kinds := &gce.Kinds{
				Kind: []*gce.Kind{
					{
						Name: "kind",
						Attributes: &gce.VM{
							Disk: []*gce.Disk{
								{
									Image: "image 1",
								},
							},
							Project: "project 1",
							Zone:    "zone",
						},
					},
				},
			}
			cfgs := &gce.Configs{
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
							Project: "project 2",
						},
					},
				},
			}
			So(merge(c, kinds, cfgs), ShouldBeNil)
			So(cfgs.GetVms(), ShouldResemble, []*gce.Config{
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
						Project: "project 2",
						Zone:    "zone",
					},
				},
			})
		})
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()

	Convey("validate", t, func() {
		c := context.Background()

		Convey("invalid", func() {
			Convey("kinds", func() {
				kinds := &gce.Kinds{
					Kind: []*gce.Kind{
						{},
					},
				}
				err := validate(c, kinds, &gce.Configs{})
				So(err, ShouldErrLike, "is required")
			})

			Convey("configs", func() {
				cfgs := &gce.Configs{
					Vms: []*gce.Config{
						{},
					},
				}
				err := validate(c, &gce.Kinds{}, cfgs)
				So(err, ShouldErrLike, "is required")
			})
		})

		Convey("valid", func() {
			Convey("empty", func() {
				err := validate(c, &gce.Kinds{}, &gce.Configs{})
				So(err, ShouldBeNil)
			})
		})
	})
}
