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
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("ComputeAmount", t, func(t *ftt.Test) {
		cfg := &Config{
			Amount: &Amount{
				Min: 1,
				Max: 3,
			},
		}
		amt, err := cfg.ComputeAmount(2, time.Time{})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, amt, should.Equal(2))
	})

	ftt.Run("Validate", t, func(t *ftt.Test) {
		c := &validation.Context{Context: context.Background()}

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				cfg := &Config{}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("at least one disk is required"))
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("prefix is required"))
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("duration or seconds is required"))
			})

			t.Run("current amount", func(t *ftt.Test) {
				cfg := &Config{
					CurrentAmount: 1,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("current amount must not be specified"))
			})

			t.Run("revision", func(t *ftt.Test) {
				cfg := &Config{
					Revision: "revision",
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("revision must not be specified"))
			})

			t.Run("unknown recycle strategy", func(t *ftt.Test) {
				cfg := &Config{
					RecycleStrategy: RecycleStrategy(99),
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("unknown recycle strategy"))
			})

			t.Run("reimage in place is not implemented yet", func(t *ftt.Test) {
				cfg := &Config{
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("RECYCLE_STRATEGY_REIMAGE_IN_PLACE is not implemented yet"))
			})

			t.Run("reimage in place with local ssd", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk: []*Disk{
							{Image: "global/images/image"},
							{Type: "zones/zone/diskTypes/local-ssd", Interface: DiskInterface_NVME},
						},
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("disk 1: local-ssd scratch disks are not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE"))
			})

			t.Run("reimage in place without boot disk image on hyperdisk", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk: []*Disk{
							{Type: "zones/zone/diskTypes/hyperdisk-balanced", Interface: DiskInterface_NVME, Size: 100},
						},
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("disk 0: boot disk image \"\" must match projects/<project>/global/images/[family/]<image> or global/images/[family/]<image> with RECYCLE_STRATEGY_REIMAGE_IN_PLACE"))
			})

			t.Run("reimage in place with invalid secondary disk image", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk: []*Disk{
							{Image: "global/images/image"},
							{Type: "zones/zone/diskTypes/hyperdisk-balanced", Interface: DiskInterface_NVME, Image: "invalid-image"},
						},
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("disk 1: image \"invalid-image\" must match projects/<project>/global/images/[family/]<image> or global/images/[family/]<image> with RECYCLE_STRATEGY_REIMAGE_IN_PLACE"))
			})

			t.Run("reimage in place with zero-size blank secondary disk", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk: []*Disk{
							{Image: "global/images/image"},
							{Type: "zones/zone/diskTypes/hyperdisk-extreme", Interface: DiskInterface_NVME},
						},
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("disk 1: blank secondary disk without image requires size > 0 with RECYCLE_STRATEGY_REIMAGE_IN_PLACE"))
			})

			t.Run("reimage in place with lssd machine type", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk:        []*Disk{{Image: "global/images/image"}},
						MachineType: "zones/zone/machineTypes/c4d-standard-8-lssd",
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("has built-in local SSDs, which are not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE"))
			})

			t.Run("reimage in place with bundled-lssd machine family", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk:        []*Disk{{Image: "global/images/image"}},
						MachineType: "zones/{{.Zone}}/machineTypes/z3-highmem-88",
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("has built-in local SSDs, which are not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE"))
			})

			t.Run("reimage in place with bare-metal machine type", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk:        []*Disk{{Image: "global/images/image"}},
						MachineType: "zones/zone/machineTypes/c3-standard-192-metal",
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("bare-metal machine_type \"zones/zone/machineTypes/c3-standard-192-metal\" is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE"))
			})

			t.Run("reimage in place with sole-tenant node affinity", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk: []*Disk{{Image: "global/images/image"}},
						Scheduling: &Scheduling{
							NodeAffinity: []*NodeAffinity{
								{Key: "node-group", Operator: NodeAffinityOperator_IN, Values: []string{"group-a"}},
							},
						},
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("scheduling.node_affinity (sole-tenant instances) is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE"))
			})

			t.Run("reimage in place with confidential compute", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk:                      []*Disk{{Image: "global/images/image"}},
						EnableConfidentialCompute: true,
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("enable_confidential_compute is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE"))
			})

			t.Run("reimage in place with alpha channel", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk:       []*Disk{{Image: "global/images/image"}},
						GcpChannel: GCPChannel_GCP_CHANNEL_ALPHA,
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("gcp_channel GCP_CHANNEL_ALPHA is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE; only GCP_CHANNEL_STABLE is supported"))
			})

			t.Run("reimage in place with beta channel", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk:       []*Disk{{Image: "global/images/image"}},
						GcpChannel: GCPChannel_GCP_CHANNEL_BETA,
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("gcp_channel GCP_CHANNEL_BETA is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE; only GCP_CHANNEL_STABLE is supported"))
			})

			t.Run("reimage in place with performance monitoring unit", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk:                      []*Disk{{Image: "global/images/image"}},
						PerformanceMonitoringUnit: PerformanceMonitoringUnit_ARCHITECTURAL,
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("performance_monitoring_unit ARCHITECTURAL is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE"))
			})

			t.Run("reimage in place with force send fields", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk:            []*Disk{{Image: "global/images/image"}},
						ForceSendFields: []string{"MinCpuPlatform"},
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("force_send_fields is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE"))
			})

			t.Run("reimage in place with null fields", func(t *ftt.Test) {
				cfg := &Config{
					Attributes: &VM{
						Disk:       []*Disk{{Image: "global/images/image"}},
						NullFields: []string{"MinCpuPlatform"},
					},
					RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
				}
				cfg.Validate(c, true)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("null_fields is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			cfg := &Config{
				Attributes: &VM{
					Disk: []*Disk{
						{
							Image: "global/images/image",
						},
					},
					MachineType: "type",
					NetworkInterface: []*NetworkInterface{
						{},
					},
					Project: "project",
					Zone:    "zone",
				},
				Lifetime: &TimePeriod{
					Time: &TimePeriod_Seconds{
						Seconds: 3600,
					},
				},
				Prefix:          "prefix",
				RecycleStrategy: RecycleStrategy_RECYCLE_STRATEGY_DELETE_AND_RECREATE,
			}
			cfg.Validate(c, true)
			assert.Loosely(t, c.Finalize(), should.BeNil)
		})
	})
}
