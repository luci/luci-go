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
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"
)

// Ensure Config implements datastore.PropertyConverter.
// This allows VMs blocks to be read from and written to the datastore.
var _ datastore.PropertyConverter = &Config{}

// FromProperty implements datastore.PropertyConverter.
func (cfg *Config) FromProperty(p datastore.Property) error {
	if p.Value() == nil {
		cfg = &Config{}
		return nil
	}
	return proto.Unmarshal(p.Value().([]byte), cfg)
}

// ToProperty implements datastore.PropertyConverter.
func (cfg *Config) ToProperty() (datastore.Property, error) {
	p := datastore.Property{}
	bytes, err := proto.Marshal(cfg)
	if err != nil {
		return datastore.Property{}, err
	}
	// noindex is not respected in the tags in the model.
	return p, p.SetValue(bytes, datastore.NoIndex)
}

// ComputeAmount returns the amount to use given the proposed amount and time.
// Assumes this config has been validated.
func (cfg *Config) ComputeAmount(proposed int32, now time.Time) (int32, error) {
	return cfg.GetAmount().getAmount(proposed, now)
}

// Validate validates this config.
func (cfg *Config) Validate(c *validation.Context, metadataFromFileResolved bool) {
	c.Enter("amount")
	cfg.GetAmount().Validate(c)
	c.Exit()
	c.Enter("attributes")
	cfg.GetAttributes().Validate(c, metadataFromFileResolved)
	c.Exit()
	if cfg.GetCurrentAmount() != 0 {
		c.Errorf("current amount must not be specified")
	}
	c.Enter("lifetime")
	cfg.GetLifetime().Validate(c)
	switch n, err := cfg.Lifetime.ToSeconds(); {
	case err != nil:
		c.Errorf("%s", err)
	case n == 0:
		c.Errorf("duration or seconds is required")
	}
	c.Exit()
	if cfg.GetCurrentAmount() != 0 {
		c.Errorf("current amount must not be specified")
	}
	if cfg.GetPrefix() == "" {
		c.Errorf("prefix is required")
	}
	if cfg.GetRevision() != "" {
		c.Errorf("revision must not be specified")
	}
	c.Enter("timeout")
	cfg.GetTimeout().Validate(c)
	c.Exit()
	switch cfg.GetRecycleStrategy() {
	case RecycleStrategy_RECYCLE_STRATEGY_UNSPECIFIED, RecycleStrategy_RECYCLE_STRATEGY_DELETE_AND_RECREATE:
	case RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE:
		c.Enter("recycle_strategy")
		// TODO(b/581087465): Remove once in-place reimaging is implemented.
		c.Errorf("RECYCLE_STRATEGY_REIMAGE_IN_PLACE is not implemented yet")
		cfg.validateRecycleStrategy(c)
		c.Exit()
	default:
		c.Enter("recycle_strategy")
		c.Errorf("unknown recycle strategy %v", cfg.GetRecycleStrategy())
		c.Exit()
	}
}

// validateRecycleStrategy rejects VM fields and shapes that cannot be recreated
// in place via Stable Compute v1 instances.update (RECREATE_DISK + RESTART).
func (cfg *Config) validateRecycleStrategy(c *validation.Context) {
	attrs := cfg.GetAttributes()
	for i, d := range attrs.GetDisk() {
		if d == nil {
			continue
		}
		// Reject local-ssd scratch disks because GCE instances.update disallows
		// RESTART with attached local SSDs and cannot RECREATE_DISK scratch disks.
		if d.IsScratchDisk() {
			c.Errorf("disk %d: local-ssd scratch disks are not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE", i)
			continue
		}
		// Require a valid boot image on disks[0] because Disk.Validate only checks
		// images on pd-standard/pd-ssd and skips hyperdisk/pd-balanced types.
		if i == 0 && !isValidResolvableImage(d.GetImage()) {
			c.Errorf("disk 0: boot disk image %q must match projects/<project>/global/images/[family/]<image> or global/images/[family/]<image> with RECYCLE_STRATEGY_REIMAGE_IN_PLACE", d.GetImage())
		} else if i > 0 && d.GetImage() != "" && !isValidResolvableImage(d.GetImage()) {
			// Secondary disks with an explicit image must use a valid image path.
			c.Errorf("disk %d: image %q must match projects/<project>/global/images/[family/]<image> or global/images/[family/]<image> with RECYCLE_STRATEGY_REIMAGE_IN_PLACE", i, d.GetImage())
		} else if i > 0 && d.GetImage() == "" && d.GetSize() <= 0 {
			// Blank secondary disks without a source image require an explicit size > 0.
			c.Errorf("disk %d: blank secondary disk without image requires size > 0 with RECYCLE_STRATEGY_REIMAGE_IN_PLACE", i)
		}
	}
	mt := attrs.GetMachineType()[strings.LastIndex(attrs.GetMachineType(), "/")+1:]
	// Reject machine types that bundle built-in local SSDs even without disk entries.
	if hasBundledLocalSSDs(mt) {
		c.Errorf("machine_type %q has built-in local SSDs, which are not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE", attrs.GetMachineType())
	}
	// Reject bare-metal machine types because host reprovisioning exceeds VM reimage SLAs.
	if strings.HasSuffix(mt, "-metal") || strings.Contains(mt, "-metal-") {
		c.Errorf("bare-metal machine_type %q is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE", attrs.GetMachineType())
	}
	// Reject sole-tenant node affinity because GCE disallows RESTART updates on sole-tenant VMs.
	if len(attrs.GetScheduling().GetNodeAffinity()) > 0 {
		c.Errorf("scheduling.node_affinity (sole-tenant instances) is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE")
	}
	// Reject Confidential Compute because SEV/TDX instances do not support in-place disk reimaging.
	if attrs.GetEnableConfidentialCompute() {
		c.Errorf("enable_confidential_compute is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE")
	}
	// Reject non-stable GCP channels because in-place reimaging calls Stable Compute v1 exclusively.
	if ch := attrs.GetGcpChannel(); ch != GCPChannel_GCP_CHANNEL_UNSPECIFIED && ch != GCPChannel_GCP_CHANNEL_STABLE {
		c.Errorf("gcp_channel %s is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE; only GCP_CHANNEL_STABLE is supported", ch)
	}
	// Reject PMU settings because they require the Alpha Compute API and would be dropped by Stable v1 updates.
	if pmu := attrs.GetPerformanceMonitoringUnit(); pmu != PerformanceMonitoringUnit_PERFORMANCE_MONITORING_UNIT_UNSPECIFIED {
		c.Errorf("performance_monitoring_unit %s is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE", pmu)
	}
	// Reject raw API field overrides because sendReimage updates the live Instance struct without re-applying them.
	if len(attrs.GetForceSendFields()) > 0 {
		c.Errorf("force_send_fields is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE")
	}
	// Reject raw API null overrides because sendReimage does not re-apply NullFields on instances.update.
	if len(attrs.GetNullFields()) > 0 {
		c.Errorf("null_fields is not supported with RECYCLE_STRATEGY_REIMAGE_IN_PLACE")
	}
}

// isValidResolvableImage reports whether s is a non-empty global or project
// image reference with non-empty path segments.
func isValidResolvableImage(s string) bool {
	return isValidImage(s) && !strings.Contains(s, "//") && !strings.HasSuffix(s, "/")
}

// hasBundledLocalSSDs reports whether base GCE machine type mt automatically
// attaches built-in local SSDs even when Attributes.Disk lists no local-ssd.
func hasBundledLocalSSDs(mt string) bool {
	if strings.HasSuffix(mt, "-lssd") {
		return true
	}
	for _, p := range []string{"z3-", "a2-ultragpu-", "a3-highgpu-", "a3-megagpu-", "a3-ultragpu-", "a4-"} {
		if strings.HasPrefix(mt, p) {
			return true
		}
	}
	return false
}
