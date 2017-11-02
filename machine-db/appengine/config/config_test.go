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

	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/machine-db/api/config/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateDatacentersConfig(t *testing.T) {
	t.Parallel()

	Convey("validateDatacentersConfig: empty config", t, func() {
		context := &validation.Context{}
		datacentersConfig := &config.DatacentersConfig{}
		validateDatacentersConfig(context, datacentersConfig)
		So(context.Finalize(), ShouldBeNil)
	})

	Convey("validateDatacentersConfig: unnamed file", t, func() {
		context := &validation.Context{}
		datacentersConfig := &config.DatacentersConfig{
			Datacenter: []string{""},
		}
		validateDatacentersConfig(context, datacentersConfig)
		So(context.Finalize(), ShouldErrLike, "datacenter filenames are required and must be non-empty")
	})

	Convey("validateDatacentersConfig: ok", t, func() {
		context := &validation.Context{}
		datacentersConfig := &config.DatacentersConfig{
			Datacenter: []string{
				"duplicate.cfg",
				"datacenter.cfg",
				"duplicate.cfg",
			},
		}
		validateDatacentersConfig(context, datacentersConfig)
		So(context.Finalize(), ShouldErrLike, "duplicate filename: duplicate.cfg")
	})

	Convey("validateDatacentersConfig: ok", t, func() {
		context := &validation.Context{}
		datacentersConfig := &config.DatacentersConfig{
			Datacenter: []string{
				"datacenter1.cfg",
				"datacenter2.cfg",
			},
		}
		validateDatacentersConfig(context, datacentersConfig)
		So(context.Finalize(), ShouldBeNil)
	})
}

func TestValidateDatacenters(t *testing.T) {
	t.Parallel()

	Convey("validateDatacenters: empty datacenters config", t, func() {
		context := &validation.Context{}
		datacenterConfigs := map[string]*config.DatacenterConfig{}
		validateDatacenters(context, datacenterConfigs)
		So(context.Finalize(), ShouldBeNil)
	})

	Convey("validateDatacenters: unnamed datacenter", t, func() {
		context := &validation.Context{}
		datacenterConfigs := map[string]*config.DatacenterConfig{
			"unnamed.cfg": {},
		}
		validateDatacenters(context, datacenterConfigs)
		So(context.Finalize(), ShouldErrLike, "datacenter names are required and must be non-empty")
	})

	Convey("validateDatacenters: duplicate datacenters", t, func() {
		context := &validation.Context{}
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
		So(context.Finalize(), ShouldErrLike, "duplicate datacenter: duplicate")
	})

	Convey("validateDatacenters: unnamed rack", t, func() {
		context := &validation.Context{}
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

	Convey("validateDatacenters: duplicate racks in the same datacenter", t, func() {
		context := &validation.Context{}
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
		So(context.Finalize(), ShouldErrLike, "duplicate rack: duplicate")
	})

	Convey("validateDatacenters: duplicate racks in different datacenters", t, func() {
		context := &validation.Context{}
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
		So(context.Finalize(), ShouldErrLike, "duplicate rack: duplicate")
	})

	Convey("validateDatacenters: unnamed switch", t, func() {
		context := &validation.Context{}
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

	Convey("validateDatacenters: duplicate switches in the same rack", t, func() {
		context := &validation.Context{}
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
		So(context.Finalize(), ShouldErrLike, "duplicate switch: duplicate")
	})

	Convey("validateDatacenters: duplicate switches in different racks", t, func() {
		context := &validation.Context{}
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
		So(context.Finalize(), ShouldErrLike, "duplicate switch: duplicate")
	})

	Convey("validateDatacenters: missing switch ports", t, func() {
		context := &validation.Context{}
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
		So(context.Finalize(), ShouldErrLike, "switch must have at least one port: switch")
	})

	Convey("validateDatacenters: negative switch ports", t, func() {
		context := &validation.Context{}
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
		So(context.Finalize(), ShouldErrLike, "switch must have at least one port: switch")
	})

	Convey("validateDatacenters: zero switch ports", t, func() {
		context := &validation.Context{}
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
		So(context.Finalize(), ShouldErrLike, "switch must have at least one port: switch")
	})

	Convey("validateDatacenters: excessive switch ports", t, func() {
		context := &validation.Context{}
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
		So(context.Finalize(), ShouldErrLike, "switch must have at most 65535 ports: switch")
	})

	Convey("validateDatacenters: ok", t, func() {
		context := &validation.Context{}
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
}
