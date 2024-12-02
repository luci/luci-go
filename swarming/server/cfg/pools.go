// Copyright 2023 The LUCI Authors.
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

package cfg

import (
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth/realms"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/validate"
)

// Pool is a parsed config of some single pool.
type Pool struct {
	// Realm is a realm with ACLs for this pool's resources.
	//
	// This is a global realm name, e.g. `infra:pool/flex/try`.
	Realm string

	// DefaultTaskRealm is a realm for tasks if they don't have a realm set.
	//
	// It is optional. If not set, all tasks must have the realm specified by
	// the caller when they are created.
	DefaultTaskRealm string

	// TODO(vadimsh): Implement task templates.
}

type pools struct {
	pools       map[string]*Pool // a map "pool name => its config"
	deployments map[string]*configpb.TaskTemplateDeployment
}

// newPoolsConfig converts pools.cfg proto to a queryable map with shared task
// template deployments.
//
// pools.cfg here already passed the validation when it was first ingested. It
// is possible the server code itself changed and the existing config is no
// longer correct in some bad way. An error is returned in that case.
//
// On success returns *pools with
// * map "pool name => its config". For each pool config,
//   - if it has inlined task template deployment, the task templates in the
//     deployment will be resolved.
//   - otherwise it'll just keep the task template deployment name.
//
// * a map of shared task template deployments containing resolved task templates.
func newPoolsConfig(cfg *configpb.PoolsCfg) (*pools, error) {
	poolMap := map[string]*Pool{}
	for _, pb := range cfg.Pool {
		cfg, err := newPool(pb)
		if err != nil {
			return nil, errors.Annotate(err, "broken pools.cfg entry: %s", pb).Err()
		}
		for _, name := range pb.Name {
			poolMap[name] = cfg
		}
	}
	return &pools{pools: poolMap}, nil
}

// newPool processes a single configpb.Pool definition.
func newPool(pb *configpb.Pool) (*Pool, error) {
	// TODO(vadimsh): Process TaskDeploymentScheme.
	return &Pool{
		Realm:            pb.Realm,
		DefaultTaskRealm: pb.DefaultTaskRealm,
	}, nil
}

// validatePoolsCfg validates pools.cfg, writing errors into `ctx`.
func validatePoolsCfg(ctx *validation.Context, cfg *configpb.PoolsCfg) {
	pools := stringset.New(0)

	validatePool := func(pb *configpb.Pool) {
		// Deprecated fields that must not be set.
		if pb.Schedulers != nil {
			ctx.Errorf("setting deprecated field `schedulers`")
		}
		if pb.BotMonitoring != "" {
			ctx.Errorf("setting deprecated field `bot_monitoring`")
		}

		if len(pb.Name) == 0 {
			ctx.Errorf("at least one pool name must be given")
		}
		for _, name := range pb.Name {
			if err := validate.DimensionValue(name); err != nil {
				ctx.Errorf("bad pool name %q: %s", name, err)
			}
			if !pools.Add(name) {
				ctx.Errorf("pool %q was already declared", name)
			}
		}

		// Realm is required.
		if pb.Realm == "" {
			ctx.Errorf("missing required `realm` field")
		} else if err := realms.ValidateRealmName(pb.Realm, realms.GlobalScope); err != nil {
			ctx.Errorf("bad `realm` field: %s", err)
		}

		// DefaultTaskRealm is optional.
		if pb.DefaultTaskRealm != "" {
			if err := realms.ValidateRealmName(pb.DefaultTaskRealm, realms.GlobalScope); err != nil {
				ctx.Errorf("bad `default_task_realm` field: %s", err)
			}
		}

		// TODO(vadimsh): Validate task template settings.

		// Silently skip remaining fields that are used by the Python implementation
		// but ignored by the Go implementation:
		//	ExternalSchedulers: external schedulers not supported in Go.
		//	EnforcedRealmPermissions: realm permissions are always enforced.
		//	RbeMigration: RBE is the only supported scheduler.
		//	SchedulingAlgorithm: RBE doesn't support custom scheduling algorithms.
	}

	for idx, pool := range cfg.Pool {
		title := "unnamed"
		if len(pool.Name) != 0 {
			title = strings.Join(pool.Name, ",")
		}
		ctx.Enter("pool #%d (%s)", idx+1, title)
		validatePool(pool)
		ctx.Exit()
	}
}
