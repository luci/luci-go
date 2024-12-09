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
	"crypto/sha256"
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

	rbeInstance            string // the RBE instance for tasks in this pool
	rbeBotsSwarmingPercent int    // percent of bots using Swarming scheduler
	rbeBotsHybridPercent   int    // percent of bots using both schedulers
	rbeBotsRBEPercent      int    // percent of bots using RBE scheduler

	// TODO(vadimsh): Implement task templates.
}

// RBEConfig are RBE-related parameters applied to a bot in a pool.
type RBEConfig struct {
	// Mode defines how the bot should be interacting with the RBE.
	Mode configpb.Pool_RBEMigration_BotModeAllocation_BotMode
	// Instance is a full RBE instance name to poll tasks from, if any.
	Instance string
}

// rbeConfig returns RBE-related configuration for a bot in this pool.
func (cfg *Pool) rbeConfig(botID string) RBEConfig {
	if cfg.rbeInstance == "" {
		return RBEConfig{
			Mode: configpb.Pool_RBEMigration_BotModeAllocation_SWARMING,
		}
	}

	// Get a quasi random integer in range [0; 100) by hashing the bot ID.
	// Do exactly what the Python code is doing to avoid bots flapping between
	// modes when migrating Python => Go.
	sum := sha256.Sum256([]byte(botID))
	num := float32(sum[0]) + float32(sum[1])*256.0
	rnd := int(num * 99.9 / (256.0 + 256.0*256.0))

	// Pick the mode depending on what subrange the bot falls into in
	// [---SWARMING---|---HYBRID---|---RBE---].
	switch {
	case rnd < cfg.rbeBotsSwarmingPercent:
		return RBEConfig{
			Mode:     configpb.Pool_RBEMigration_BotModeAllocation_SWARMING,
			Instance: "", // no RBE instance in Swarming mode
		}
	case rnd < cfg.rbeBotsSwarmingPercent+cfg.rbeBotsHybridPercent:
		return RBEConfig{
			Mode:     configpb.Pool_RBEMigration_BotModeAllocation_HYBRID,
			Instance: cfg.rbeInstance,
		}
	default:
		return RBEConfig{
			Mode:     configpb.Pool_RBEMigration_BotModeAllocation_RBE,
			Instance: cfg.rbeInstance,
		}
	}
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
	poolCfg := &Pool{
		Realm:            pb.Realm,
		DefaultTaskRealm: pb.DefaultTaskRealm,
	}

	if rbeCfg := pb.RbeMigration; rbeCfg != nil {
		// The config should have been validated already. Recheck only assumptions
		// needed for correctness of rbeConfig(...).
		allocs := map[configpb.Pool_RBEMigration_BotModeAllocation_BotMode]int{}
		for _, alloc := range rbeCfg.BotModeAllocation {
			allocs[alloc.Mode] = int(alloc.Percent)
			if alloc.Percent < 0 {
				return nil, errors.Reason("unexpectedly incorrect RBE migration config").Err()
			}
		}
		poolCfg.rbeInstance = rbeCfg.RbeInstance
		poolCfg.rbeBotsSwarmingPercent = allocs[configpb.Pool_RBEMigration_BotModeAllocation_SWARMING]
		poolCfg.rbeBotsHybridPercent = allocs[configpb.Pool_RBEMigration_BotModeAllocation_HYBRID]
		poolCfg.rbeBotsRBEPercent = allocs[configpb.Pool_RBEMigration_BotModeAllocation_RBE]
		if poolCfg.rbeBotsSwarmingPercent+poolCfg.rbeBotsHybridPercent+poolCfg.rbeBotsRBEPercent != 100 {
			return nil, errors.Reason("unexpectedly incorrect RBE migration config").Err()
		}
	}

	return poolCfg, nil
}

// validatePoolsCfg validates pools.cfg, writing errors into `ctx`.
func validatePoolsCfg(ctx *validation.Context, cfg *configpb.PoolsCfg) {
	pools := stringset.New(0)

	// task_template
	ctx.Enter("task_template")
	tmpMap := make(map[string]*configpb.TaskTemplate, len(cfg.TaskTemplate))
	for i, tmp := range cfg.TaskTemplate {
		ctx.Enter("#%d (%s)", i+1, tmp.Name)
		if tmp.Name == "" {
			ctx.Errorf("name is empty")
		}
		if _, ok := tmpMap[tmp.Name]; ok {
			ctx.Errorf("template %q was already declared", tmp.Name)
		}
		validateTemplate(ctx, tmp, true)
		tmpMap[tmp.Name] = tmp
		ctx.Exit()
	}
	ctx.Exit()

	// task_template_deployment
	ctx.Enter("task_template_deployment")
	dplMap := make(map[string]*configpb.TaskTemplateDeployment, len(cfg.TaskTemplateDeployment))
	for i, dpl := range cfg.TaskTemplateDeployment {
		ctx.Enter("#%d (%s)", i+1, dpl.Name)
		if dpl.Name == "" {
			ctx.Errorf("name is empty")
		}
		if _, ok := dplMap[dpl.Name]; ok {
			ctx.Errorf("deployment %q was already declared", dpl.Name)
		}
		validateDeployment(ctx, dpl)
		dplMap[dpl.Name] = dpl
		ctx.Exit()
	}
	ctx.Exit()

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

		// task template settings.
		if pb.GetTaskTemplateDeployment() != "" {
			if _, ok := dplMap[pb.GetTaskTemplateDeployment()]; !ok {
				ctx.Errorf("unknown `task_template_deployment`: %q", pb.GetTaskTemplateDeployment())
			}
		}

		if pb.GetTaskTemplateDeploymentInline() != nil {
			ctx.Enter("task_template_deployment_inline")
			dpl := pb.GetTaskTemplateDeploymentInline()
			if dpl.Name != "" {
				ctx.Errorf("name cannot be specified")
			}
			validateDeployment(ctx, dpl)
			ctx.Exit()
		}

		if pb.RbeMigration != nil {
			ctx.Enter("rbe_migration")
			validateRBEMigration(ctx, pb.RbeMigration)
			ctx.Exit()
		}

		// Silently skip remaining fields that are used by the Python implementation
		// but ignored by the Go implementation:
		//	ExternalSchedulers: external schedulers not supported in Go.
		//	EnforcedRealmPermissions: realm permissions are always enforced.
		//	SchedulingAlgorithm: only FIFO is supported.
	}

	// pool
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

func validateTemplate(ctx *validation.Context, tmp *configpb.TaskTemplate, hasName bool) {
	if !hasName && tmp.Name != "" {
		ctx.Errorf("name cannot be specified")
	}

	ctx.Enter("cache")
	cachesPathSet, merr := validate.Caches(tmp.Cache)
	for _, err := range merr {
		ctx.Error(err)
	}
	ctx.Exit()

	ctx.Enter("cipd_package")
	merr = validate.CIPDPackages(tmp.CipdPackage, false, cachesPathSet)
	for _, err := range merr {
		ctx.Error(err)
	}
	ctx.Exit()

	ctx.Enter("env")
	validateEnv(ctx, tmp.Env)
	ctx.Exit()

	// TODO(chanli): validate inclusion
}

func validateEnv(ctx *validation.Context, envs []*configpb.TaskTemplate_Env) {
	if len(envs) > validate.MaxEnvVarCount {
		ctx.Errorf("can have up to %d env", validate.MaxEnvVarCount)
	}

	envKeys := stringset.New(len(envs))
	for i, env := range envs {
		ctx.Enter("#%d %s", i+1, env.Var)
		if env.Var == "" {
			ctx.Errorf("var is empty")
		}
		if !envKeys.Add(env.Var) {
			ctx.Errorf("env %q was already declared", env.Var)
		}
		if env.Value != "" {
			if err := validate.Length(env.Value, validate.MaxEnvValueLength); err != nil {
				ctx.Errorf("value: %s", err)
			}
		}
		for _, prefix := range env.Prefix {
			if err := validate.Path(prefix, validate.MaxEnvValueLength); err != nil {
				ctx.Errorf("prefix: %s", err)
			}
		}
		ctx.Exit()
	}
}

func validateDeployment(ctx *validation.Context, dpl *configpb.TaskTemplateDeployment) {
	if dpl.CanaryChance < 0 || dpl.CanaryChance > 9999 {
		ctx.Errorf("canary_chance out of range [0,9999]")
	}
	if dpl.CanaryChance > 0 && dpl.Canary == nil {
		ctx.Errorf("canary_chance specified without a canary")
	}

	if dpl.Prod != nil {
		ctx.Enter("prod")
		validateTemplate(ctx, dpl.Prod, false)
		ctx.Exit()
	}
	if dpl.Canary != nil {
		ctx.Enter("canary")
		validateTemplate(ctx, dpl.Canary, false)
		ctx.Exit()
	}
}

func validateRBEMigration(ctx *validation.Context, pb *configpb.Pool_RBEMigration) {
	if pb.RbeInstance == "" {
		ctx.Errorf("rbe_instance is required")
	}
	if pb.RbeModePercent < 0 || pb.RbeModePercent > 100 {
		ctx.Errorf("rbe_mode_percent should be in [0; 100]")
	}

	allocs := map[configpb.Pool_RBEMigration_BotModeAllocation_BotMode]int{}
	broken := false
	for i, alloc := range pb.BotModeAllocation {
		ctx.Enter("bot_mode_allocation #%d", i)
		if alloc.Mode == configpb.Pool_RBEMigration_BotModeAllocation_UNKNOWN {
			ctx.Errorf("mode is required")
			broken = true
		} else if _, haveIt := allocs[alloc.Mode]; haveIt {
			ctx.Errorf("allocation for mode %s was already defined", alloc.Mode)
			broken = true
		} else {
			allocs[alloc.Mode] = int(alloc.Percent)
			if alloc.Percent < 0 || alloc.Percent > 100 {
				ctx.Errorf("percent should be in [0; 100]")
				broken = true
			}
		}
		ctx.Exit()
	}

	if broken {
		return
	}

	total := 0
	for _, percent := range allocs {
		total += percent
	}
	if total != 100 {
		ctx.Errorf("bot_mode_allocation percents should sum up to 100")
	}
}
