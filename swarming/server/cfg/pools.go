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
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

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

	// Deployment contains the resolved task templates.
	Deployment *configpb.TaskTemplateDeployment

	// RBEInstance is the RBE instance for tasks in this pool.
	RBEInstance string

	// RBEModePercent is the percent of tasks targeting this pool to send to RBE.
	RBEModePercent int

	rbeBotsSwarmingPercent int // percent of bots using Swarming scheduler
	rbeBotsHybridPercent   int // percent of bots using both schedulers
	rbeBotsRBEPercent      int // percent of bots using RBE scheduler
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
	if cfg.RBEInstance == "" {
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
			Instance: cfg.RBEInstance,
		}
	default:
		return RBEConfig{
			Mode:     configpb.Pool_RBEMigration_BotModeAllocation_RBE,
			Instance: cfg.RBEInstance,
		}
	}
}

// newPoolsConfig converts pools.cfg proto to a queryable map.
//
// pools.cfg here already passed the validation when it was first ingested. It
// is possible the server code itself changed and the existing config is no
// longer correct in some bad way. An error is returned in that case.
func newPoolsConfig(cfg *configpb.PoolsCfg) (map[string]*Pool, error) {
	graph, merr := newInclusionGraph(cfg.TaskTemplate)
	if len(merr) > 0 {
		return nil, merr.AsError()
	}
	if err := graph.flattenTaskTemplates(); err != nil {
		return nil, err
	}

	dplMap := make(map[string]*configpb.TaskTemplateDeployment, len(cfg.TaskTemplateDeployment))
	for _, dpl := range cfg.TaskTemplateDeployment {
		resolved, err := resolveDeployment(dpl, graph)
		if err != nil {
			return nil, err
		}
		dplMap[dpl.Name] = resolved
	}

	poolMap := map[string]*Pool{}
	for _, pb := range cfg.Pool {
		cfg, err := newPool(pb, dplMap, graph)
		if err != nil {
			return nil, errors.Annotate(err, "broken pools.cfg entry: %s", pb).Err()
		}
		for _, name := range pb.Name {
			poolMap[name] = cfg
		}
	}
	return poolMap, nil
}

// newPool processes a single configpb.Pool definition.
func newPool(pb *configpb.Pool, dplMap map[string]*configpb.TaskTemplateDeployment, graph *inclusionGraph) (*Pool, error) {
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
		poolCfg.RBEInstance = rbeCfg.RbeInstance
		poolCfg.rbeBotsSwarmingPercent = allocs[configpb.Pool_RBEMigration_BotModeAllocation_SWARMING]
		poolCfg.rbeBotsHybridPercent = allocs[configpb.Pool_RBEMigration_BotModeAllocation_HYBRID]
		poolCfg.rbeBotsRBEPercent = allocs[configpb.Pool_RBEMigration_BotModeAllocation_RBE]
		if poolCfg.rbeBotsSwarmingPercent+poolCfg.rbeBotsHybridPercent+poolCfg.rbeBotsRBEPercent != 100 {
			return nil, errors.Reason("unexpectedly incorrect RBE migration config").Err()
		}
		poolCfg.RBEModePercent = int(rbeCfg.RbeModePercent)
		if poolCfg.RBEModePercent < 0 || poolCfg.RBEModePercent > 100 {
			return nil, errors.Reason("unexpectedly incorrect RBE migration config").Err()
		}
	}

	if pb.GetTaskTemplateDeployment() != "" {
		namedDpl, ok := dplMap[pb.GetTaskTemplateDeployment()]
		if !ok {
			return nil, errors.Reason("unknown `task_template_deployment`: %q", pb.GetTaskTemplateDeployment()).Err()
		}
		poolCfg.Deployment = namedDpl
		return poolCfg, nil
	}

	if pb.GetTaskTemplateDeploymentInline() != nil {
		resolved, err := resolveDeployment(pb.GetTaskTemplateDeploymentInline(), graph)
		if err != nil {
			return nil, err
		}
		poolCfg.Deployment = resolved
	}
	return poolCfg, nil
}

// validatePoolsCfg validates pools.cfg, writing errors into `ctx`.
func validatePoolsCfg(ctx *validation.Context, cfg *configpb.PoolsCfg) {
	pools := stringset.New(0)

	validateDefaultCIPD(ctx, cfg.DefaultExternalServices.GetCipd(), len(cfg.Pool))

	// task_template
	ctx.Enter("task_template")
	for i, tmp := range cfg.TaskTemplate {
		ctx.Enter("#%d (%s)", i+1, tmp.Name)
		if tmp.Name == "" {
			ctx.Errorf("name is empty")
		}
		validateTemplate(ctx, tmp, true)
		ctx.Exit()
	}

	// reslove the templates with their inclusions.
	ctx.Enter("resolve inclusion")

	// Should only flatten the templates if they pass the preliminary validation.
	shouldFlatten := !ctx.HasPendingErrors()

	var graph *inclusionGraph
	var merr errors.MultiError
	if shouldFlatten {
		graph, merr = newInclusionGraph(cfg.TaskTemplate)
		if len(merr) > 0 {
			for _, err := range merr {
				ctx.Error(err)
			}
			shouldFlatten = false
		} else {
			if err := graph.flattenTaskTemplates(); err != nil {
				ctx.Error(err)
				shouldFlatten = false
			}
		}
	}

	if shouldFlatten {
		// The templates are flattened.
		// Validate them again to check if any conflicts are brought in
		// by the included templates.
		for i, tmp := range cfg.TaskTemplate {
			ctx.Enter("#%d (%s)", i+1, tmp.Name)
			validateTemplate(ctx, graph.flattened[tmp.Name].tmp, true)
			ctx.Exit()
		}
	}
	ctx.Exit() // exit "resolve inclusion"
	ctx.Exit() // exit task_template

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
		validateDeployment(ctx, dpl, graph, shouldFlatten)
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
			validateDeployment(ctx, dpl, graph, shouldFlatten)
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

func validateDefaultCIPD(ctx *validation.Context, pb *configpb.ExternalServices_CIPD, numPools int) {
	ctx.Enter("default_cipd")
	defer ctx.Exit()

	if pb == nil {
		// Only require defaultCIPD if there are pools.
		// Without pools it's fine to not set defaultCIPD, since there's nowhere
		// to use defaultCIPD anyway.
		if numPools > 0 {
			ctx.Errorf("required")
		}
		return
	}

	ctx.Enter("server")
	if err := validate.CIPDServer(pb.Server); err != nil {
		ctx.Error(err)
	}
	ctx.Exit()

	ctx.Enter("client_package")
	defer ctx.Exit()
	clientPkg := pb.ClientPackage
	if clientPkg == nil {
		ctx.Errorf("required")
		return
	}
	ctx.Enter("name")
	if err := validate.CIPDPackageName(clientPkg.PackageName, true); err != nil {
		ctx.Error(err)
	}
	ctx.Exit()

	ctx.Enter("version")
	if err := validate.CIPDPackageVersion(clientPkg.Version); err != nil {
		ctx.Error(err)
	}
	ctx.Exit()
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

func validateTemplate(ctx *validation.Context, tmp *configpb.TaskTemplate, hasName bool) {
	if !hasName && tmp.Name != "" {
		ctx.Errorf("name cannot be specified")
	}

	ctx.Enter("cache")
	doc, merr := validate.Caches(tmp.Cache, "task_template_cache")
	for _, err := range merr {
		ctx.Error(err)
	}
	ctx.Exit()

	ctx.Enter("cipd_package")
	merr = validate.CIPDPackages(tmp.CipdPackage, false, doc, "task_template_cipd_package")
	for _, err := range merr {
		ctx.Error(err)
	}
	ctx.Exit()

	ctx.Enter("env")
	validateEnv(ctx, tmp.Env)
	ctx.Exit()
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

type templateResolvement struct {
	tmp      *configpb.TaskTemplate
	resolved bool
}
type inclusionGraph struct {
	// A map of flattened templates. For each template:
	// * Include contains the full transitively included templates, including self
	// * Other fields have been fully resloved by applying the fields depth-first
	//  then upwards.
	flattened map[string]*templateResolvement

	// A map of the original task templates from pools config, to help calculate
	// the transitively inclusions and flatten the templates.
	original map[string]*configpb.TaskTemplate
}

// newInclusionGraph builds the graph, checking there are no duplicate
// nodes and no dangling edges.
func newInclusionGraph(tmps []*configpb.TaskTemplate) (*inclusionGraph, errors.MultiError) {
	graph := &inclusionGraph{
		flattened: make(map[string]*templateResolvement, len(tmps)),
		original:  make(map[string]*configpb.TaskTemplate, len(tmps)),
	}

	var merr errors.MultiError
	for i, tmp := range tmps {
		if tmp.Name == "" {
			merr.MaybeAdd(errors.Reason("template %d: name is empty", i).Err())
			continue
		}
		if _, ok := graph.original[tmp.Name]; ok {
			merr.MaybeAdd(errors.Reason("template %q was already declared", tmp.Name).Err())
			continue
		}
		graph.original[tmp.Name] = tmp
	}

	if len(merr) != 0 {
		return nil, merr
	}

	for _, tmp := range tmps {
		_, err := graph.includes(tmp.Name, stringset.New(len(tmps)))
		if err != nil {
			merr.MaybeAdd(err)
			return nil, merr
		}
	}

	return graph, nil
}

// includes returns all task templates transitively included by
// `root` in order of their inclusion "deepness". The returned list
// always starts with `root` itself.
//
// Also add a node for root in g.flattened to keep the inclusion list if there
// is not one yet.
//
// Returns an error if this part of the graph has a cycle or `root`
// is an unknown node.
func (g *inclusionGraph) includes(root string, visited stringset.Set) ([]string, error) {
	if node, ok := g.flattened[root]; ok {
		return node.tmp.Include, nil
	}

	if !visited.Add(root) {
		return nil, errors.Reason("encounter inclusion cycle for template %q", root).Err()
	}

	unflattened, ok := g.original[root]
	if !ok {
		return nil, errors.Reason("unknown template %q", root).Err()
	}

	allIncs := []string{root}
	incSet := stringset.NewFromSlice(allIncs...)
	for _, sub := range unflattened.Include {
		if sub == root {
			return nil, errors.Reason("template %q includes self", root).Err()
		}

		subIncludes, err := g.includes(sub, visited)
		if err != nil {
			return nil, err
		}

		for _, subInc := range subIncludes {
			if !incSet.Add(subInc) {
				return nil, errors.Reason("template %q already includes %q", root, subInc).Err()
			}
			allIncs = append(allIncs, subInc)
		}

	}

	g.flattened[root] = &templateResolvement{
		tmp: &configpb.TaskTemplate{Include: allIncs},
	}

	return allIncs, nil
}

func (g *inclusionGraph) flattenTaskTemplates() error {
	for name := range g.original {
		_, err := g.flattenTaskTemplate(name)
		if err != nil {
			return err
		}
	}
	return nil
}

// flattenTaskTemplate returns a flattened TaskTemplate for root.
//
// If g.flattened[root] has not resolved, this function will also update
// g.flattened[root] and resolve it.
func (g *inclusionGraph) flattenTaskTemplate(root string) (*templateResolvement, error) {
	tmp, ok := g.flattened[root]
	if !ok {
		return nil, errors.Reason("unknown template %q", root).Err()
	}

	// Directly reture the already resolved template.
	if tmp.resolved {
		return tmp, nil
	}

	// Root needs to resolve, do it now.
	includes := tmp.tmp.Include
	originalTmp := g.original[root]
	if originalTmp == nil {
		panic(fmt.Sprintf("unknown template %q", root))
	}

	if len(includes) == 0 {
		panic(fmt.Sprintf("template %q should at least have itself in transitive inclusions", root))
	}

	if includes[0] != root {
		panic(fmt.Sprintf("templates %q includes %q without self", root, includes))
	}

	// Flatten.
	flattened := proto.Clone(originalTmp).(*configpb.TaskTemplate)
	for i := 1; i < len(includes); i++ {
		inc := includes[i]

		sub, err := g.flattenTaskTemplate(inc)
		if err != nil {
			return nil, err
		}

		flattened, err = mergeTaskTemplate(flattened, sub.tmp)
		if err != nil {
			return nil, err
		}
	}
	tmp.tmp = flattened
	tmp.tmp.Include = includes
	tmp.resolved = true
	return tmp, nil
}

// mergeTaskTemplate returns a TaskTemplate with contents merged from base and sub.
func mergeTaskTemplate(base, sub *configpb.TaskTemplate) (*configpb.TaskTemplate, error) {
	// When merge, applies sub fields firstly, then apply the ones from base.
	// So effectively the newTmp should have all fields from base, plus the ones
	// that are only from sub.
	newTmp := proto.Clone(base).(*configpb.TaskTemplate)

	// cache
	baseCacheMap := make(map[string]*configpb.TaskTemplate_CacheEntry, len(newTmp.Cache))
	for _, c := range newTmp.Cache {
		baseCacheMap[c.Name] = c
	}
	for _, sc := range sub.Cache {
		if _, ok := baseCacheMap[sc.Name]; !ok {
			newTmp.Cache = append(newTmp.Cache, sc)
		}
	}

	// cipd_package
	type pkgKey struct {
		pkg  string
		path string
	}
	pk := func(p *configpb.TaskTemplate_CipdPackage) pkgKey {
		return pkgKey{
			pkg:  p.Pkg,
			path: p.Path,
		}
	}
	basePkgMap := make(map[pkgKey]*configpb.TaskTemplate_CipdPackage, len(newTmp.CipdPackage))
	for _, p := range newTmp.CipdPackage {
		basePkgMap[pk(p)] = p
	}
	for _, sp := range sub.CipdPackage {
		if _, ok := basePkgMap[pk(sp)]; !ok {
			newTmp.CipdPackage = append(newTmp.CipdPackage, sp)
		}
	}

	// env
	baseEnvMap := make(map[string]*configpb.TaskTemplate_Env, len(newTmp.Env))
	for _, e := range newTmp.Env {
		baseEnvMap[e.Var] = e
	}
	for _, e := range sub.Env {
		if be, ok := baseEnvMap[e.Var]; ok {
			// append prefix from base, instead of overriding.
			be.Prefix = append(be.Prefix, e.Prefix...)
		} else {
			newTmp.Env = append(newTmp.Env, e)
		}
	}
	return newTmp, nil
}

func validateDeployment(ctx *validation.Context, dpl *configpb.TaskTemplateDeployment, graph *inclusionGraph, shouldFlatten bool) {
	if dpl.CanaryChance < 0 || dpl.CanaryChance > 9999 {
		ctx.Errorf("canary_chance out of range [0,9999]")
	}
	if dpl.CanaryChance > 0 && dpl.Canary == nil {
		ctx.Errorf("canary_chance specified without a canary")
	}

	// validate the templates in deployment, flatten them before validation if
	// needed.
	flattenAndValidate := func(tmp *configpb.TaskTemplate) {
		if !shouldFlatten {
			validateTemplate(ctx, tmp, false)
			return
		}

		newTmp, err := flattenInlinedTaskTemplate(tmp, graph)
		if err != nil {
			ctx.Error(err)
			return
		}
		validateTemplate(ctx, newTmp, false)
	}

	ctx.Enter("prod")
	if dpl.Prod == nil {
		ctx.Errorf("required")
	} else {
		flattenAndValidate(dpl.Prod)
	}
	ctx.Exit()

	if dpl.Canary != nil {
		ctx.Enter("canary")
		flattenAndValidate(dpl.Canary)
		ctx.Exit()
	}
}

// flattenInlinedTaskTemplate returns a flattened template that is inlined in a deployment.
func flattenInlinedTaskTemplate(tmp *configpb.TaskTemplate, graph *inclusionGraph) (*configpb.TaskTemplate, error) {
	if len(tmp.Include) == 0 {
		return tmp, nil
	}

	// Check the inclusions to detect unknowns or inclusion diamonds.
	incSet := stringset.New(len(tmp.Include))
	for _, inc := range tmp.Include {
		sub, ok := graph.flattened[inc]
		if !ok {
			return nil, errors.Reason("includes unknown template %q", inc).Err()
		}
		for _, subInc := range sub.tmp.Include {
			if !incSet.Add(subInc) {
				return nil, errors.Reason("template already includes %q", subInc).Err()
			}
		}
	}

	// But only need to merge first level already flattened templates included
	// in tmp.Include.
	newTmp := proto.Clone(tmp).(*configpb.TaskTemplate)
	var err error
	for _, inc := range tmp.Include {
		sub := graph.flattened[inc]
		newTmp, err = mergeTaskTemplate(newTmp, sub.tmp)
		if err != nil {
			return nil, err
		}
	}

	// No need to keep includes in inlined template
	newTmp.Include = nil
	return newTmp, nil
}

func resolveDeployment(dpl *configpb.TaskTemplateDeployment, graph *inclusionGraph) (*configpb.TaskTemplateDeployment, error) {
	var err error
	resolved := proto.Clone(dpl).(*configpb.TaskTemplateDeployment)
	if resolved.Prod != nil {
		if resolved.Prod, err = flattenInlinedTaskTemplate(resolved.Prod, graph); err != nil {
			return nil, err
		}
	}

	if resolved.Canary != nil {
		if resolved.Canary, err = flattenInlinedTaskTemplate(resolved.Canary, graph); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}
