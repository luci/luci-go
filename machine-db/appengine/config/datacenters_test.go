// Copyright 2017 The LUCI Authors.
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
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/machine-db/api/config/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateDatacentersConfig(t *testing.T) {
	t.Parallel()

	Convey("Initialize validation context", t, func() {
		context := &validation.Context{Context: context.Background()}

		Convey("validateDatacentersConfig: empty config", func() {
			datacentersConfig := &config.DatacentersConfig{}
			validateDatacentersConfig(context, datacentersConfig)
			So(context.Finalize(), ShouldBeNil)
		})

		Convey("validateDatacentersConfig: unnamed file", func() {
			datacentersConfig := &config.DatacentersConfig{
				Datacenter: []string{""},
			}
			validateDatacentersConfig(context, datacentersConfig)
			So(context.Finalize(), ShouldErrLike, "datacenter filenames are required and must be non-empty")
		})

		Convey("validateDatacentersConfig: ok1", func() {
			datacentersConfig := &config.DatacentersConfig{
				Datacenter: []string{
					"duplicate.cfg",
					"datacenter.cfg",
					"duplicate.cfg",
				},
			}
			validateDatacentersConfig(context, datacentersConfig)
			So(context.Finalize(), ShouldErrLike, "duplicate filename")
		})

		Convey("validateDatacentersConfig: ok2", func() {
			datacentersConfig := &config.DatacentersConfig{
				Datacenter: []string{
					"datacenter1.cfg",
					"datacenter2.cfg",
				},
			}
			validateDatacentersConfig(context, datacentersConfig)
			So(context.Finalize(), ShouldBeNil)
		})
	})
}

func TestValidateDatacenters(t *testing.T) {
	t.Parallel()

	Convey("Initialize validation context", t, func() {
		context := &validation.Context{Context: context.Background()}

		Convey("validateDatacenters: empty datacenters config", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldBeNil)
		})

		Convey("validateDatacenters: unnamed datacenter", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"unnamed.cfg": {},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldErrLike, "datacenter names are required and must be non-empty")
		})

		Convey("validateDatacenters: duplicate datacenters", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"datacenter1.cfg": {
					Name: "duplicate",
				},
				"datacenter2.cfg": {
					Name: "unique",
				},
				"datacenter3.cfg": {
					Name: "duplicate",
				},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldErrLike, "duplicate datacenter")
		})

		Convey("validateDatacenters: unnamed rack", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"datacenter.cfg": {
					Name: "datacenter",
					Rack: []*config.RackConfig{
						{},
					},
				},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldErrLike, "rack names are required and must be non-empty")
		})

		Convey("validateDatacenters: duplicate racks in the same datacenter", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"datacenter.cfg": {
					Name: "datacenter",
					Rack: []*config.RackConfig{
						{
							Name: "duplicate",
						},
						{
							Name: "unique",
						},
						{
							Name: "duplicate",
						},
					},
				},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldErrLike, "duplicate rack")
		})

		Convey("validateDatacenters: duplicate racks in different datacenters", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"datacenter1.cfg": {
					Name: "datacenter 1",
					Rack: []*config.RackConfig{
						{
							Name: "duplicate",
						},
						{
							Name: "unique",
						},
					},
				},
				"datacenter2.cfg": {
					Name: "datacenter 2",
					Rack: []*config.RackConfig{
						{
							Name: "duplicate",
						},
					},
				},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldErrLike, "duplicate rack")
		})

		Convey("validateDatacenters: unnamed switch", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"datacenter.cfg": {
					Name: "datacenter",
					Rack: []*config.RackConfig{
						{
							Name: "rack",
							Switch: []*config.SwitchConfig{
								{},
							},
						},
					},
				},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldErrLike, "switch names are required and must be non-empty")
		})

		Convey("validateDatacenters: duplicate switches in the same rack", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"datacenter.cfg": {
					Name: "datacenter",
					Rack: []*config.RackConfig{
						{
							Name: "rack",
							Switch: []*config.SwitchConfig{
								{
									Name:  "duplicate",
									Ports: 2,
								},
								{
									Name:  "unique",
									Ports: 4,
								},
								{
									Name:  "duplicate",
									Ports: 8,
								},
							},
						},
					},
				},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldErrLike, "duplicate switch")
		})

		Convey("validateDatacenters: duplicate switches in different racks", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"datacenter1.cfg": {
					Name: "datacenter 1",
					Rack: []*config.RackConfig{
						{
							Name: "rack 1",
							Switch: []*config.SwitchConfig{
								{
									Name:  "duplicate",
									Ports: 2,
								},
								{
									Name:  "unique",
									Ports: 4,
								},
							},
						},
					},
				},
				"datacenter2.cfg": {
					Name: "datacenter 2",
					Rack: []*config.RackConfig{
						{
							Name: "rack 2",
							Switch: []*config.SwitchConfig{
								{
									Name:  "duplicate",
									Ports: 8,
								},
							},
						},
					},
				},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldErrLike, "duplicate switch")
		})

		Convey("validateDatacenters: missing switch ports", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"datacenter.cfg": {
					Name: "datacenter",
					Rack: []*config.RackConfig{
						{
							Name: "rack",
							Switch: []*config.SwitchConfig{
								{
									Name: "switch",
								},
							},
						},
					},
				},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldErrLike, "must have at least one port")
		})

		Convey("validateDatacenters: negative switch ports", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"datacenter.cfg": {
					Name: "datacenter",
					Rack: []*config.RackConfig{
						{
							Name: "rack",
							Switch: []*config.SwitchConfig{
								{
									Name:  "switch",
									Ports: -1,
								},
							},
						},
					},
				},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldErrLike, "must have at least one port")
		})

		Convey("validateDatacenters: zero switch ports", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"datacenter.cfg": {
					Name: "datacenter",
					Rack: []*config.RackConfig{
						{
							Name: "rack",
							Switch: []*config.SwitchConfig{
								{
									Name:  "switch",
									Ports: 0,
								},
							},
						},
					},
				},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldErrLike, "must have at least one port")
		})

		Convey("validateDatacenters: excessive switch ports", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"datacenter.cfg": {
					Name: "datacenter",
					Rack: []*config.RackConfig{
						{
							Name: "rack",
							Switch: []*config.SwitchConfig{
								{
									Name:  "switch",
									Ports: 65536,
								},
							},
						},
					},
				},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldErrLike, "must have at most 65535 ports")
		})

		Convey("validateDatacenters: ok", func() {
			datacenterConfigs := map[string]*config.DatacenterConfig{
				"datacenter1.cfg": {
					Name:        "datacenter 1",
					Description: "A description of datacenter 1",
					Rack: []*config.RackConfig{
						{
							Name:        "rack 1",
							Description: "A description of rack 1",
							Switch: []*config.SwitchConfig{
								{
									Name:        "switch 1",
									Description: "A description of switch 1",
									Ports:       4,
								},
							},
						},
						{
							Name: "rack 2",
						},
					},
				},
				"datacenter2.cfg": {
					Name: "datacenter 2",
				},
				"datacenter3.cfg": {
					Name: "datacenter 3",
					Rack: []*config.RackConfig{
						{
							Name: "rack 3",
							Switch: []*config.SwitchConfig{
								{
									Name:  "switch 2",
									Ports: 8,
								},
								{
									Name:  "switch 3",
									Ports: 16,
								},
							},
						},
					},
				},
			}
			validateDatacenters(context, datacenterConfigs)
			So(context.Finalize(), ShouldBeNil)
		})
	})
}
