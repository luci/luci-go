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

// Package validation implements validation and common manipulation of CQ config
// files.
package validation

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/data/stringset"
	luciconfig "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	migrationpb "go.chromium.org/luci/cv/api/migration"
)

// Config validation rules go here.

func init() {
	addRules(&validation.Rules)
}

// TODO(crbug.com/1252545): Use a dev-specific configuration for a dev instance
// of the service after CQD is deleted.
func addRules(r *validation.RuleSet) {
	r.Add("regex:projects/[^/]+", "commit-queue.cfg", validateProject)
	r.Add("services/commit-queue", "migration-settings.cfg", validateMigrationSettings)
}

// validateProject validates a project-level CQ config.
//
// Validation result is returned via validation ctx, while error returned
// directly implies only a bug in this code.
func validateProject(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := cfgpb.Config{}
	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Error(err)
	} else {
		validateProjectConfig(ctx, &cfg)
	}
	return nil
}

func validateProjectConfig(ctx *validation.Context, cfg *cfgpb.Config) {
	if cfg.ProjectScopedAccount != cfgpb.Toggle_UNSET {
		ctx.Errorf("project_scoped_account for just CQ isn't supported. " +
			"Use project-wide config for all LUCI services in luci-config/projects.cfg")
	}
	if cfg.DrainingStartTime != "" {
		// TODO(crbug/1208569): re-enable or re-design this feature.
		ctx.Errorf("draining_start_time is temporarily not allowed, see https://crbug.com/1208569." +
			"Reach out to LUCI team oncall if you need urgent help")
	}
	if cfg.CqStatusHost != "" {
		switch u, err := url.Parse("https://" + cfg.CqStatusHost); {
		case err != nil:
			ctx.Errorf("failed to parse cq_status_host %q: %s", cfg.CqStatusHost, err)
		case u.Host != cfg.CqStatusHost:
			ctx.Errorf("cq_status_host %q should be just a host %q", cfg.CqStatusHost, u.Host)
		}
	}
	if cfg.SubmitOptions != nil {
		ctx.Enter("submit_options")
		if cfg.SubmitOptions.MaxBurst < 0 {
			ctx.Errorf("max_burst must be >= 0")
		}
		if d := cfg.SubmitOptions.BurstDelay; d != nil && d.AsDuration() < 0 {
			ctx.Errorf("burst_delay must be positive or 0")
		}
		ctx.Exit()
	}
	if len(cfg.ConfigGroups) == 0 {
		ctx.Errorf("at least 1 config_group is required")
		return
	}

	knownNames := make(stringset.Set, len(cfg.ConfigGroups))
	fallbackGroupIdx := -1
	for i, g := range cfg.ConfigGroups {
		ctx.Enter("config_group #%d", i+1)
		validateConfigGroup(ctx, g, knownNames)
		switch {
		case g.Fallback == cfgpb.Toggle_YES && fallbackGroupIdx == -1:
			fallbackGroupIdx = i
		case g.Fallback == cfgpb.Toggle_YES:
			ctx.Errorf("At most 1 config_group with fallback=YES allowed "+
				"(already declared in config_group #%d", fallbackGroupIdx+1)
		}
		ctx.Exit()
	}
}

var (
	configGroupNameRegexp    = regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9_-]{0,39}$")
	modeNameRegexp           = regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9_-]{0,39}$")
	analyzerRun              = "ANALYZER_RUN"
	standardModes            = stringset.NewFromSlice(analyzerRun, "DRY_RUN", "FULL_RUN")
	analyzerLocationReRegexp = regexp.MustCompile(`^(https://([a-z\-]+)\-review\.googlesource\.com/([a-z0-9_\-/]+)+/\[\+\]/)?\.\+(\\\.[a-z]+)?$`)
)

func validateConfigGroup(ctx *validation.Context, group *cfgpb.ConfigGroup, knownNames stringset.Set) {
	switch {
	case group.Name == "":
		// TODO(crbug/1063508): make this an error.
		ctx.Warningf("please, specify `name` for monitoring and analytics")
	case !configGroupNameRegexp.MatchString(group.Name):
		// TODO(crbug/1063508): make this an error.
		ctx.Warningf("`name` must match %q but %q given", configGroupNameRegexp, group.Name)
	case knownNames.Has(group.Name):
		ctx.Errorf("duplicate config_group `name` %q not allowed", group.Name)
	default:
		knownNames.Add(group.Name)
	}

	if len(group.Gerrit) == 0 {
		ctx.Errorf("at least 1 gerrit is required")
	}
	gerritURLs := stringset.Set{}
	for i, g := range group.Gerrit {
		ctx.Enter("gerrit #%d", i+1)
		validateGerrit(ctx, g)
		if g.Url != "" && !gerritURLs.Add(g.Url) {
			ctx.Errorf("duplicate gerrit url in the same config_group: %q", g.Url)
		}
		ctx.Exit()
	}

	if group.CombineCls != nil {
		ctx.Enter("combine_cls")
		switch d := group.CombineCls.StabilizationDelay; {
		case d == nil:
			ctx.Errorf("stabilization_delay is required to enable cl_grouping")
		case d.AsDuration() < 10*time.Second:
			ctx.Errorf("stabilization_delay must be at least 10 seconds")
		}
		if group.GetVerifiers().GetGerritCqAbility().GetAllowSubmitWithOpenDeps() {
			ctx.Errorf("combine_cls can not be used with gerrit_cq_ability.allow_submit_with_open_deps=true.")
		}
		ctx.Exit()
	}

	additionalModes := stringset.New(len(group.AdditionalModes))
	if len(group.AdditionalModes) > 0 {
		ctx.Enter("additional_modes")
		for _, m := range group.AdditionalModes {
			switch name := m.Name; {
			case name == "":
				ctx.Errorf("`name` is required")
			case name == "DRY_RUN" || name == "FULL_RUN":
				ctx.Errorf("`name` MUST not be DRY_RUN or FULL_RUN")
			case !modeNameRegexp.MatchString(name):
				ctx.Errorf("`name` must match %q but %q is given", modeNameRegexp, name)
			case additionalModes.Has(name):
				ctx.Errorf("duplicate `name` %q not allowed", name)
			default:
				additionalModes.Add(name)
			}
			if val := m.CqLabelValue; val < 1 || val > 2 {
				ctx.Errorf("`cq_label_value` must be either 1 or 2, got %d", val)
			}
			switch m.TriggeringLabel {
			case "":
				ctx.Errorf("`triggering_label` is required")
			case "Commit-Queue":
				ctx.Errorf("`triggering_label` MUST not be \"Commit-Queue\"")
			}
			if m.TriggeringValue <= 0 {
				ctx.Errorf("`triggering_value` must be > 0")
			}
		}
		ctx.Exit()
	}

	if group.Verifiers == nil {
		ctx.Errorf("verifiers are required")
	} else {
		ctx.Enter("verifiers")
		validateVerifiers(ctx, group.Verifiers, additionalModes.Union(standardModes))
		ctx.Exit()
	}
}

func validateGerrit(ctx *validation.Context, g *cfgpb.ConfigGroup_Gerrit) {
	validateGerritURL(ctx, g.Url)
	if len(g.Projects) == 0 {
		ctx.Errorf("at least 1 project is required")
	}
	nameToIndex := make(map[string]int, len(g.Projects))
	for i, p := range g.Projects {
		ctx.Enter("projects #%d", i+1)
		validateGerritProject(ctx, p)
		if p.Name != "" {
			if _, dup := nameToIndex[p.Name]; !dup {
				nameToIndex[p.Name] = i
			} else {
				ctx.Errorf("duplicate project in the same gerrit: %q", p.Name)
			}
		}
		ctx.Exit()
	}
}

func validateGerritURL(ctx *validation.Context, gURL string) {
	if gURL == "" {
		ctx.Errorf("url is required")
		return
	}
	u, err := url.Parse(gURL)
	if err != nil {
		ctx.Errorf("failed to parse url %q: %s", gURL, err)
		return
	}
	if u.Path != "" {
		ctx.Errorf("path component not yet allowed in url (%q specified)", u.Path)
	}
	if u.RawQuery != "" {
		ctx.Errorf("query component not allowed in url (%q specified)", u.RawQuery)
	}
	if u.Fragment != "" {
		ctx.Errorf("fragment component not allowed in url (%q specified)", u.Fragment)
	}
	if u.Scheme != "https" {
		ctx.Errorf("only 'https' scheme supported for now (%q specified)", u.Scheme)
	}
	if !strings.HasSuffix(u.Host, ".googlesource.com") {
		// TODO(tandrii): relax this.
		ctx.Errorf("only *.googlesource.com hosts supported for now (%q specified)", u.Host)
	}
}

func validateGerritProject(ctx *validation.Context, gp *cfgpb.ConfigGroup_Gerrit_Project) {
	if gp.Name == "" {
		ctx.Errorf("name is required")
	} else {
		if strings.HasPrefix(gp.Name, "/") || strings.HasPrefix(gp.Name, "a/") {
			ctx.Errorf("name must not start with '/' or 'a/'")
		}
		if strings.HasSuffix(gp.Name, "/") || strings.HasSuffix(gp.Name, ".git") {
			ctx.Errorf("name must not end with '.git' or '/'")
		}
	}

	regexps := stringset.Set{}
	for i, r := range gp.RefRegexp {
		ctx.Enter("ref_regexp #%d", i+1)
		if _, err := regexpCompileCached(r); err != nil {
			ctx.Error(err)
		}
		if !regexps.Add(r) {
			ctx.Errorf("duplicate regexp: %q", r)
		}
		ctx.Exit()
	}
	for i, r := range gp.RefRegexpExclude {
		ctx.Enter("ref_regexp_exclude #%d", i+1)
		if _, err := regexpCompileCached(r); err != nil {
			ctx.Error(err)
		}
		if !regexps.Add(r) {
			// There is no point excluding exact same regexp as including.
			ctx.Errorf("duplicate regexp: %q", r)
		}
		ctx.Exit()
	}
}

func validateVerifiers(ctx *validation.Context, v *cfgpb.Verifiers, supportedModes stringset.Set) {
	if v.Cqlinter != nil {
		ctx.Errorf("cqlinter verifier is not allowed (internal use only)")
	}
	if v.Fake != nil {
		ctx.Errorf("fake verifier is not allowed (internal use only)")
	}
	if v.TreeStatus != nil {
		ctx.Enter("tree_status")
		if v.TreeStatus.Url == "" {
			ctx.Errorf("url is required")
		} else {
			switch u, err := url.Parse(v.TreeStatus.Url); {
			case err != nil:
				ctx.Errorf("failed to parse url %q: %s", v.TreeStatus.Url, err)
			case u.Scheme != "https":
				ctx.Errorf("url scheme must be 'https'")
			}
		}
		ctx.Exit()
	}
	if v.GerritCqAbility == nil {
		ctx.Errorf("gerrit_cq_ability verifier is required")
	} else {
		ctx.Enter("gerrit_cq_ability")
		if len(v.GerritCqAbility.CommitterList) == 0 {
			ctx.Errorf("committer_list is required")
		} else {
			for i, l := range v.GerritCqAbility.CommitterList {
				if l == "" {
					ctx.Enter("committer_list #%d", i+1)
					ctx.Errorf("must not be empty string")
					ctx.Exit()
				}
			}
		}
		for i, l := range v.GerritCqAbility.DryRunAccessList {
			if l == "" {
				ctx.Enter("dry_run_access_list #%d", i+1)
				ctx.Errorf("must not be empty string")
				ctx.Exit()
			}
		}
		ctx.Exit()
	}
	if v.Tryjob != nil {
		ctx.Enter("tryjob")
		validateTryjobVerifier(ctx, v.Tryjob, supportedModes)
		ctx.Exit()
	}
}

func validateTryjobVerifier(ctx *validation.Context, v *cfgpb.Verifiers_Tryjob, supportedModes stringset.Set) {
	if v.RetryConfig != nil {
		ctx.Enter("retry_config")
		validateTryjobRetry(ctx, v.RetryConfig)
		ctx.Exit()
	}

	switch v.CancelStaleTryjobs {
	case cfgpb.Toggle_YES:
		ctx.Errorf("`cancel_stale_tryjobs: YES` matches default CQ behavior now; please remove")
	case cfgpb.Toggle_NO:
		ctx.Errorf("`cancel_stale_tryjobs: NO` is no longer supported, use per-builder `cancel_stale` instead")
	case cfgpb.Toggle_UNSET:
		// OK
	}

	if len(v.Builders) == 0 {
		ctx.Errorf("at least 1 builder required")
		return
	}

	// Validation of builders is done in two passes: local and global.

	visitBuilders := func(cb func(b *cfgpb.Verifiers_Tryjob_Builder)) {
		for i, b := range v.Builders {
			if b.Name != "" {
				ctx.Enter("builder %s", b.Name)
			} else {
				ctx.Enter("builder #%d", i+1)
			}
			cb(b)
			ctx.Exit()
		}
	}

	// Pass 1, local: verify each builder separately.
	// Also, populate data structures for second pass.
	names := stringset.Set{}
	equi := stringset.Set{} // equivalent_to builder names.
	// Subset of builders that can be triggered directly
	// and which can be relied upon to trigger other builders.
	canStartTriggeringTree := make([]string, 0, len(v.Builders))
	triggersMap := map[string][]string{} // who triggers whom.
	// Find config by name.
	cfgByName := make(map[string]*cfgpb.Verifiers_Tryjob_Builder, len(v.Builders))
	hasNonAnalyzerBuilder := false

	visitBuilders(func(b *cfgpb.Verifiers_Tryjob_Builder) {
		validateBuilderName(ctx, b.Name, names)
		cfgByName[b.Name] = b
		if b.TriggeredBy != "" {
			// Don't validate TriggeredBy as builder name, it should just match
			// another main builder name, which will be validated anyway.
			triggersMap[b.TriggeredBy] = append(triggersMap[b.TriggeredBy], b.Name)
			if b.ExperimentPercentage != 0 {
				ctx.Errorf("experiment_percentage is not combinable with triggered_by")
			}
			if b.EquivalentTo != nil {
				ctx.Errorf("equivalent_to is not combinable with triggered_by")
			}
		}
		if b.EquivalentTo != nil {
			validateEquivalentBuilder(ctx, b.EquivalentTo, equi)
			if b.ExperimentPercentage != 0 {
				ctx.Errorf("experiment_percentage is not combinable with equivalent_to")
			}
		}
		if b.ExperimentPercentage != 0 {
			if b.ExperimentPercentage < 0.0 || b.ExperimentPercentage > 100.0 {
				ctx.Errorf("experiment_percentage must between 0 and 100 (%f given)", b.ExperimentPercentage)
			}
			if b.IncludableOnly {
				ctx.Errorf("includable_only is not combinable with experiment_percentage")
			}
		}
		if len(b.LocationRegexp)+len(b.LocationRegexpExclude) > 0 {
			validateRegexp(ctx, "location_regexp", b.LocationRegexp, locationRegexpHeuristic)
			validateRegexp(ctx, "location_regexp_exclude", b.LocationRegexpExclude, locationRegexpHeuristic)
			if b.IncludableOnly {
				ctx.Errorf("includable_only is not combinable with location_regexp[_exclude]")
			}
		}
		if len(b.OwnerWhitelistGroup) > 0 {
			for i, g := range b.OwnerWhitelistGroup {
				if g == "" {
					ctx.Enter("owner_whitelist_group #%d", i+1)
					ctx.Errorf("must not be empty string")
					ctx.Exit()
				}
			}
		}

		var isAnalyzer bool
		if len(b.ModeAllowlist) > 0 {
			for i, m := range b.ModeAllowlist {
				switch {
				case !supportedModes.Has(m):
					ctx.Enter("mode_allowlist #%d", i+1)
					ctx.Errorf("must be one of %s", supportedModes.ToSortedSlice())
					ctx.Exit()
				case m == analyzerRun:
					isAnalyzer = true
				}
			}
			if isAnalyzer {
				// TODO(crbug/1202952): Remove following restrictions after Tricium is
				// folded into CV.
				if len(b.ModeAllowlist) > 1 {
					ctx.Errorf("%s must be the only element in mode_allowlist", analyzerRun)
				}
				for i, r := range b.LocationRegexp {
					// TODO(crbug/1202952): Remove this check after tricium is folded
					// into CV.
					if !analyzerLocationReRegexp.MatchString(r) {
						ctx.Enter("location_regexp #%d", i+1)
						ctx.Errorf(`location_regexp of an analyzer MUST either be in the format of ".+\.extension" (e.g. ".+\.py) or "https://host-review.googlesource.com/project/[+]/.+\.extension" (e.g. "https://chromium-review.googlesource.com/infra/infra/[+]/.+\.py"). Extension is optional.`)
						ctx.Exit()
					}
				}
				if len(b.LocationRegexpExclude) > 0 {
					ctx.Errorf("location_regexp_exclude is not combinable with tryjob run in %s mode", analyzerRun)
				}
			}
			// TODO(crbug/1191855): See if CV should loose the following restrictions.
			if b.TriggeredBy != "" {
				ctx.Errorf("triggered_by is not combinable with mode_allowlist")
			}
			if b.IncludableOnly {
				ctx.Errorf("includable_only is not combinable with mode_allowlist")
			}
		}
		if !isAnalyzer {
			hasNonAnalyzerBuilder = true
		}
		if b.ExperimentPercentage == 0 && b.TriggeredBy == "" && b.EquivalentTo == nil {
			canStartTriggeringTree = append(canStartTriggeringTree, b.Name)
		}
	})

	if !hasNonAnalyzerBuilder {
		// TODO(crbug/1202952): This is for preventing users from defining new
		// config group purely for analyzer purpose that accidentally overlaps
		// with users' main cq config group before Tricium is merged into CV.
		// Current known use cases (i.e. defining Tricium config in auxillary LUCI
		// Projects) will set `lucicfg.config(tracked_files="tricium-prod.cfg")`
		// so that no cq config will be generated. This check should be removed
		// after Tricium is merged into CV so that ANALYZER_RUN is treated the
		// same way as any other modes supported by CV.
		ctx.Errorf("must have at least one non-analyzer tryjob builder")
	}

	// Between passes, do a depth-first search into triggers-whom DAG starting
	// with only those builders which can be triggered directly by CQ.
	q := canStartTriggeringTree
	canBeTriggered := stringset.NewFromSlice(q...)
	for len(q) > 0 {
		var b string
		q, b = q[:len(q)-1], q[len(q)-1]
		for _, whom := range triggersMap[b] {
			if canBeTriggered.Add(whom) {
				q = append(q, whom)
			} else {
				panic("IMPOSSIBLE: builder |b| starting at |canStartTriggeringTree| " +
					"isn't triggered by anyone, so it can't be equal to |whom|, which had triggered_by.")
			}
		}
	}
	// Corollary: all builders with triggered_by but not in canBeTriggered set
	// are not properly configured, either referring to non-existing builder OR
	// forming a loop.

	// Pass 2, global: verify builder relationships.
	visitBuilders(func(b *cfgpb.Verifiers_Tryjob_Builder) {
		switch {
		case b.EquivalentTo != nil && b.EquivalentTo.Name != "" && names.Has(b.EquivalentTo.Name):
			ctx.Errorf("equivalent_to.name must not refer to already defined %q builder", b.EquivalentTo.Name)
		case b.TriggeredBy != "" && !names.Has(b.TriggeredBy):
			ctx.Errorf("triggered_by must refer to an existing builder, but %q given", b.TriggeredBy)
		case b.TriggeredBy != "" && !canBeTriggered.Has(b.TriggeredBy):
			// Although we can detect actual loops and emit better errors,
			// this happens so rarely, it's not yet worth the time.
			ctx.Errorf("triggered_by must refer to an existing builder without "+
				"equivalent_to or experiment_percentage options. triggered_by "+
				"relationships must also not form a loop (given: %q)",
				b.TriggeredBy)
		case b.TriggeredBy != "":
			// Reaching here means parent exists in config.
			parent, _ := cfgByName[b.TriggeredBy]
			validateParentLocationRegexp(ctx, b, parent)
		}
	})
}

func validateBuilderName(ctx *validation.Context, name string, knownNames stringset.Set) {
	if name == "" {
		ctx.Errorf("name is required")
		return
	}
	if !knownNames.Add(name) {
		ctx.Errorf("duplicate name %q", name)
	}
	parts := strings.Split(name, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		ctx.Errorf("name %q doesn't match required format project/short-bucket-name/builder, e.g. 'v8/try/linux'", name)
	}
	for _, part := range parts {
		subs := strings.Split(part, ".")
		if len(subs) >= 3 && subs[0] == "luci" {
			// Technically, this is allowed. However, practically, this is
			// extremely likely to be misunderstanding of project or bucket is.
			ctx.Errorf("name %q is highly likely malformed; it should be project/short-bucket-name/builder, e.g. 'v8/try/linux'", name)
			return
		}
	}
	if err := luciconfig.ValidateProjectName(parts[0]); err != nil {
		ctx.Errorf("first part of %q is not a valid LUCI project name", name)
	}
}

func validateEquivalentBuilder(ctx *validation.Context, b *cfgpb.Verifiers_Tryjob_EquivalentBuilder, equiNames stringset.Set) {
	ctx.Enter("equivalent_to")
	defer ctx.Exit()
	validateBuilderName(ctx, b.Name, equiNames)
	if b.Percentage < 0 || b.Percentage > 100 {
		ctx.Errorf("percentage must be between 0 and 100 (%f given)", b.Percentage)
	}
}

type regexpExtraCheck func(ctx *validation.Context, field string, r *regexp.Regexp, value string)

func validateRegexp(ctx *validation.Context, field string, values []string, extra ...regexpExtraCheck) {
	valid := stringset.New(len(values))
	for i, v := range values {
		if v == "" {
			ctx.Errorf("%s #%d: must not be empty", field, i+1)
			continue
		}
		if !valid.Add(v) {
			ctx.Errorf("duplicate %s: %q", field, v)
			continue
		}
		r, err := regexpCompileCached(v)
		if err != nil {
			ctx.Errorf("%s %q: %s", field, v, err)
			continue
		}
		for _, f := range extra {
			f(ctx, field, r, v)
		}
	}
}

// locationRegexpHeuristic catches common mistakes in location_regexp[_exclude].
func locationRegexpHeuristic(ctx *validation.Context, field string, r *regexp.Regexp, value string) {
	if prefix, _ := r.LiteralPrefix(); !strings.HasPrefix(prefix, "https://") {
		return
	}
	const gsource = ".googlesource.com"
	idx := strings.Index(value, gsource)
	if idx == -1 {
		return
	}
	subdomain := value[len("https://"):idx]
	if strings.HasSuffix(subdomain, "-review") {
		return
	}
	exp := value[:idx] + "-review" + value[idx:]
	ctx.Warningf("%s %q is probably missing '-review' suffix; did you mean %q?", field, value, exp)
}

func validateParentLocationRegexp(ctx *validation.Context, child, parent *cfgpb.Verifiers_Tryjob_Builder) {
	// Child's regexps shouldn't be less restrictive than parent.
	// While general check is not possible, in known so far use-cases, ensuring
	// the regexps are exact same expressions suffices and will prevent
	// accidentally incorrect configs.
	c := stringset.NewFromSlice(child.LocationRegexp...)
	p := stringset.NewFromSlice(parent.LocationRegexp...)
	if !p.Contains(c) {
		// This func is called in the context of a child.
		ctx.Errorf("location_regexp of a triggered builder must be a subset of its parent %q,"+
			" but these are not in parent: %s",
			parent.Name, strings.Join(c.Difference(p).ToSortedSlice(), ", "))
	}
	c = stringset.NewFromSlice(child.LocationRegexpExclude...)
	p = stringset.NewFromSlice(parent.LocationRegexpExclude...)
	if !c.Contains(p) {
		// This func is called in the context of a child.
		ctx.Errorf("location_regexp_exclude of a triggered builder must contain all those of its parent %q,"+
			" but these are only in parent: %s",
			parent.Name, strings.Join(p.Difference(c).ToSortedSlice(), ", "))
	}
}

func validateTryjobRetry(ctx *validation.Context, r *cfgpb.Verifiers_Tryjob_RetryConfig) {
	if r.SingleQuota < 0 {
		ctx.Errorf("negative single_quota not allowed (%d given)", r.SingleQuota)
	}
	if r.GlobalQuota < 0 {
		ctx.Errorf("negative global_quota not allowed (%d given)", r.GlobalQuota)
	}
	if r.FailureWeight < 0 {
		ctx.Errorf("negative failure_weight not allowed (%d given)", r.FailureWeight)
	}
	if r.TransientFailureWeight < 0 {
		ctx.Errorf("negative transitive_failure_weight not allowed (%d given)", r.TransientFailureWeight)
	}
	if r.TimeoutWeight < 0 {
		ctx.Errorf("negative timeout_weight not allowed (%d given)", r.TimeoutWeight)
	}
}

// validateMigrationSettings validates a migration-settings file.
//
// Validation result is returned via validation ctx, while error returned
// directly implies only a bug in this code.
func validateMigrationSettings(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := migrationpb.Settings{}
	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Error(err)
		return nil
	}
	for i, a := range cfg.GetApiHosts() {
		ctx.Enter("api_hosts #%d", i+1)
		switch h := a.GetHost(); h {
		case "luci-change-verifier-dev.appspot.com":
		case "luci-change-verifier.appspot.com":
		default:
			ctx.Errorf("invalid host (given: %q)", h)
		}
		validateRegexp(ctx, "project_regexp", a.GetProjectRegexp())
		validateRegexp(ctx, "project_regexp_exclude", a.GetProjectRegexpExclude())
		ctx.Exit()
	}
	if u := cfg.GetUseCvStatus(); u != nil {
		ctx.Enter("use_cv_status")
		validateRegexp(ctx, "project_regexp", u.GetProjectRegexp())
		validateRegexp(ctx, "project_regexp_exclude", u.GetProjectRegexpExclude())
		ctx.Exit()
	}
	return nil
}

// regexpCompileCached is the caching version of regexp.Compile.
//
// Most config files use the same regexp many times.
func regexpCompileCached(pattern string) (*regexp.Regexp, error) {
	cached, err := regexpCache.GetOrCreate(context.Background(), pattern, func() (interface{}, time.Duration, error) {
		r, err := regexp.Compile(pattern)
		return regexpCacheValue{r, err}, 0, nil
	})
	if err != nil {
		panic(err)
	}
	v := cached.(regexpCacheValue)
	return v.r, v.err
}

var regexpCache = lru.New(1024)

type regexpCacheValue struct {
	r   *regexp.Regexp
	err error
}
