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
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth/identity"
	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/git/footer"
	luciconfig "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	apipb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
)

const (
	// CQStatusHostPublic is the public host of the CQ status app.
	CQStatusHostPublic = "chromium-cq-status.appspot.com"
	// CQStatusHostInternal is the internal host of the CQ status app.
	CQStatusHostInternal = "internal-cq-status.appspot.com"

	dummyProjectSkipListenerValidation = "dummy-project-skip-listener-validation-deadbeaf-abc"
)

var limitNameRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.\-@_+]{0,511}$`)

// ValidateProject validates a project config for a given LUCI project.
//
// Validation result is returned via validation ctx, while error returned
// directly implies internal errors.
func ValidateProject(ctx *validation.Context, cfg *cfgpb.Config, project string) error {
	vd, err := makeProjectConfigValidator(ctx, project)
	if err != nil {
		return errors.Annotate(err, "makeProjectConfigValidator").Err()
	}
	vd.validateProjectConfig(cfg)
	return nil
}

// ValidateProjectConfig validates a given project config.
//
// This is essentially the same as ValidateProject, but skips checking
// GerritSubscriptions in listener.Settings.
func ValidateProjectConfig(ctx *validation.Context, cfg *cfgpb.Config) error {
	return ValidateProject(ctx, cfg, dummyProjectSkipListenerValidation)
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
		return nil
	}
	return ValidateProject(ctx, &cfg, luciconfig.Set(configSet).Project())
}

type projectConfigValidator struct {
	ctx                            *validation.Context
	subscribedGerritHosts          stringset.Set
	projectEnabledInGerritListener bool
}

func makeProjectConfigValidator(ctx *validation.Context, project string) (*projectConfigValidator, error) {
	switch project {
	case "":
		return nil, errors.Reason("empty project").Err()
	case dummyProjectSkipListenerValidation:
		return &projectConfigValidator{ctx: ctx}, nil
	}

	lCfg, err := srvcfg.GetListenerConfig(ctx.Context, nil)
	if err != nil {
		return nil, errors.Annotate(err, "GetListenerConfig").Err()
	}
	isEnabled, err := srvcfg.MakeListenerProjectChecker(lCfg)
	if err != nil {
		return nil, errors.Annotate(err, "MakeListenerProjectChecker").Err()
	}
	ret := &projectConfigValidator{
		ctx:                            ctx,
		subscribedGerritHosts:          stringset.New(len(lCfg.GetGerritSubscriptions())),
		projectEnabledInGerritListener: isEnabled(project),
	}
	for _, sub := range lCfg.GetGerritSubscriptions() {
		ret.subscribedGerritHosts.Add(sub.GetHost())
	}
	return ret, nil
}

func (vd *projectConfigValidator) validateProjectConfig(cfg *cfgpb.Config) {
	if cfg.ProjectScopedAccount != cfgpb.Toggle_UNSET {
		vd.ctx.Errorf("project_scoped_account for just CQ isn't supported. " +
			"Use project-wide config for all LUCI services in luci-config/projects.cfg")
	}
	if cfg.DrainingStartTime != "" {
		// TODO(crbug/1208569): re-enable or re-design this feature.
		vd.ctx.Errorf("draining_start_time is temporarily not allowed, see https://crbug.com/1208569." +
			"Reach out to LUCI team oncall if you need urgent help")
	}
	switch cfg.CqStatusHost {
	case CQStatusHostInternal:
	case CQStatusHostPublic:
	case "":
	default:
		vd.ctx.Errorf("cq_status_host must be either empty or one of %q or %q", CQStatusHostPublic, CQStatusHostInternal)
	}
	if cfg.SubmitOptions != nil {
		vd.ctx.Enter("submit_options")
		if cfg.SubmitOptions.MaxBurst < 0 {
			vd.ctx.Errorf("max_burst must be >= 0")
		}
		if d := cfg.SubmitOptions.BurstDelay; d != nil && d.AsDuration() < 0 {
			vd.ctx.Errorf("burst_delay must be positive or 0")
		}
		vd.ctx.Exit()
	}
	if len(cfg.ConfigGroups) == 0 {
		vd.ctx.Errorf("at least 1 config_group is required")
		return
	}

	knownNames := make(stringset.Set, len(cfg.ConfigGroups))
	fallbackGroupIdx := -1
	for i, g := range cfg.ConfigGroups {
		enter(vd.ctx, "config_group", i, g.Name)
		vd.validateConfigGroup(g, knownNames)
		switch {
		case g.Fallback == cfgpb.Toggle_YES && fallbackGroupIdx == -1:
			fallbackGroupIdx = i
		case g.Fallback == cfgpb.Toggle_YES:
			vd.ctx.Errorf("At most 1 config_group with fallback=YES allowed "+
				"(already declared in config_group #%d", fallbackGroupIdx+1)
		}
		vd.ctx.Exit()
	}
}

var (
	configGroupNameRegexp = regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9_-]{0,39}$")
	modeNameRegexp        = regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9_-]{0,39}$")
	analyzerRun           = "ANALYZER_RUN"
	standardModes         = stringset.NewFromSlice(analyzerRun, "DRY_RUN", "FULL_RUN", "NEW_PATCHSET_RUN")
	analyzerPathReRegexp  = regexp.MustCompile(`^\.\+(\\\.[a-z]+)?$`)
)

func (vd *projectConfigValidator) validateConfigGroup(group *cfgpb.ConfigGroup, knownNames stringset.Set) {
	switch {
	case group.Name == "":
		vd.ctx.Errorf("name is required")
	case !configGroupNameRegexp.MatchString(group.Name):
		vd.ctx.Errorf("name must match %q regexp but %q given", configGroupNameRegexp, group.Name)
	case knownNames.Has(group.Name):
		vd.ctx.Errorf("duplicate config_group name %q not allowed", group.Name)
	default:
		knownNames.Add(group.Name)
	}

	if len(group.Gerrit) == 0 {
		vd.ctx.Errorf("at least 1 gerrit is required")
	}
	gerritURLs := stringset.Set{}
	for i, g := range group.Gerrit {
		enter(vd.ctx, "gerrit", i, g.Url)
		vd.validateGerrit(g)
		if g.Url != "" && !gerritURLs.Add(g.Url) {
			vd.ctx.Errorf("duplicate gerrit url in the same config_group: %q", g.Url)
		}
		vd.ctx.Exit()
	}

	if group.CombineCls != nil {
		vd.ctx.Enter("combine_cls")
		switch d := group.CombineCls.StabilizationDelay; {
		case d == nil:
			vd.ctx.Errorf("stabilization_delay is required to enable cl_grouping")
		case d.AsDuration() < 10*time.Second:
			vd.ctx.Errorf("stabilization_delay must be at least 10 seconds")
		}
		if group.GetVerifiers().GetGerritCqAbility().GetAllowSubmitWithOpenDeps() {
			vd.ctx.Errorf("combine_cls can not be used with gerrit_cq_ability.allow_submit_with_open_deps=true.")
		}
		vd.ctx.Exit()
	}

	additionalModes := stringset.New(len(group.AdditionalModes))
	for i, m := range group.AdditionalModes {
		vd.ctx.Enter("additional_modes #%d", (i + 1))
		if err := m.ValidateAll(); err != nil {
			vd.ctx.Errorf("%s", err)
		}
		vd.ctx.Enter("name")
		if !additionalModes.Add(m.Name) {
			vd.ctx.Errorf("%q is already in use", m.Name)
		}
		vd.ctx.Exit()
		vd.ctx.Exit()
	}

	paNames := stringset.New(len(group.PostActions))
	for i, pa := range group.PostActions {
		vd.ctx.Enter("post_actions #%d", (i + 1))
		if err := pa.ValidateAll(); err != nil {
			vd.ctx.Errorf("%s", err)
		}

		vd.ctx.Enter("name")
		if !paNames.Add(pa.GetName()) {
			vd.ctx.Errorf("name '%q' is already in use", pa.GetName())
		}
		vd.ctx.Exit() // name
		for i, tc := range pa.GetConditions() {
			vd.ctx.Enter("conditions #%d", (i + 1))
			switch m := tc.GetMode(); {
			case m == "DRY_RUN":
			case m == "FULL_RUN":
			case m == "NEW_PATCHSET_RUN":
			case additionalModes.Has(m):
			default:
				vd.ctx.Enter("mode")
				vd.ctx.Errorf("invalid mode %q", m)
				vd.ctx.Exit()
			}

			// pgv's enum.in accepts the numeric representation of enum values,
			// which produces non-so-readable error messages.
			// check the statuses here to produce better error messages.
			sts := stringset.New(len(tc.GetStatuses()))
			for i, st := range tc.GetStatuses() {
				vd.ctx.Enter("statuses #%d", (i + 1))
				switch st {
				case apipb.Run_SUCCEEDED, apipb.Run_FAILED, apipb.Run_CANCELLED:
					if !sts.Add(st.String()) {
						vd.ctx.Errorf("%q was specified already", st)
					}
				default:
					vd.ctx.Errorf("%q is not a terminal status", st)
				}
				vd.ctx.Exit() // statuses #i
			}
			vd.ctx.Exit() // conditions #i
		}
		vd.ctx.Enter("action")
		switch act := pa.GetAction().(type) {
		case nil:
		case *cfgpb.ConfigGroup_PostAction_VoteGerritLabels_:
			vd.validateVoteGerritLabels(act.VoteGerritLabels)
		default:
			vd.ctx.Errorf("action %T is not supported", act)
		}
		vd.ctx.Exit() // action
		vd.ctx.Exit() // post_actions #i
	}

	teNames := stringset.New(len(group.GetTryjobExperiments()))
	for i, te := range group.GetTryjobExperiments() {
		vd.ctx.Enter("tryjob_experiments #%d", (i + 1))
		if err := te.ValidateAll(); err != nil {
			vd.ctx.Errorf("%s", err)
		}

		vd.ctx.Enter("name")
		name := te.GetName()
		if !teNames.Add(te.GetName()) {
			vd.ctx.Errorf("duplicate name %q", name)
		}
		if !bbutil.ExperimentNameRE.MatchString(name) {
			vd.ctx.Errorf("%q does not match %q", name, bbutil.ExperimentNameRE)
		}
		vd.ctx.Exit() // name
		vd.ctx.Exit() // tryjob_experiments #i
	}

	if group.Verifiers == nil {
		vd.ctx.Errorf("verifiers are required")
	} else {
		vd.ctx.Enter("verifiers")
		vd.validateVerifiers(group.Verifiers, additionalModes.Union(standardModes))
		vd.ctx.Exit()
	}
	vd.validateUserLimits(group.GetUserLimits(), group.GetUserLimitDefault())
}

func (vd *projectConfigValidator) validateVoteGerritLabels(work *cfgpb.ConfigGroup_PostAction_VoteGerritLabels) {
	// perform extra validations that are not checked by PGV.
	labels := stringset.New(len(work.Votes))
	for i, vote := range work.Votes {
		if !labels.Add(vote.Name) {
			vd.ctx.Enter("votes #%d", (i + 1))
			vd.ctx.Errorf("label %q already specified", vote.Name)
			vd.ctx.Exit()
		}
	}
}

func (vd *projectConfigValidator) validateGerrit(g *cfgpb.ConfigGroup_Gerrit) {
	vd.validateGerritURL(g.Url)
	if len(g.Projects) == 0 {
		vd.ctx.Errorf("at least 1 project is required")
	}
	nameToIndex := make(map[string]int, len(g.Projects))
	for i, p := range g.Projects {
		enter(vd.ctx, "projects", i, p.Name)
		vd.validateGerritProject(p)
		if p.Name != "" {
			if _, dup := nameToIndex[p.Name]; !dup {
				nameToIndex[p.Name] = i
			} else {
				vd.ctx.Errorf("duplicate project in the same gerrit: %q", p.Name)
			}
		}
		// TODO(crbug.com/1358208): check if listener-settings.cfg has
		// a subscription for all the Gerrit hosts, if the LUCI project is
		// enabled in the pubsub listener.
		vd.ctx.Exit()
	}
}

func (vd *projectConfigValidator) validateGerritURL(gURL string) {
	if gURL == "" {
		vd.ctx.Errorf("url is required")
		return
	}
	u, err := url.Parse(gURL)
	if err != nil {
		vd.ctx.Errorf("failed to parse url %q: %s", gURL, err)
		return
	}
	if u.Path != "" {
		vd.ctx.Errorf("path component not yet allowed in url (%q specified)", u.Path)
	}
	if u.RawQuery != "" {
		vd.ctx.Errorf("query component not allowed in url (%q specified)", u.RawQuery)
	}
	if u.Fragment != "" {
		vd.ctx.Errorf("fragment component not allowed in url (%q specified)", u.Fragment)
	}
	if u.Scheme != "https" {
		vd.ctx.Errorf("only 'https' scheme supported for now (%q specified)", u.Scheme)
	}
	if !strings.HasSuffix(u.Host, ".googlesource.com") {
		// TODO(tandrii): relax this.
		vd.ctx.Errorf("only *.googlesource.com hosts supported for now (%q specified)", u.Host)
	}
	if vd.projectEnabledInGerritListener && !vd.subscribedGerritHosts.Has(u.Host) {
		vd.ctx.Errorf("Gerrit pub/sub for %q is not configured; please visit go/luci/cv/gerrit-pubsub#validation-error", u.Host)
	}
}

func (vd *projectConfigValidator) validateGerritProject(gp *cfgpb.ConfigGroup_Gerrit_Project) {
	if gp.Name == "" {
		vd.ctx.Errorf("name is required")
	} else {
		if strings.HasPrefix(gp.Name, "/") || strings.HasPrefix(gp.Name, "a/") {
			vd.ctx.Errorf("name must not start with '/' or 'a/'")
		}
		if strings.HasSuffix(gp.Name, "/") || strings.HasSuffix(gp.Name, ".git") {
			vd.ctx.Errorf("name must not end with '.git' or '/'")
		}
	}

	regexps := stringset.Set{}
	for i, r := range gp.RefRegexp {
		vd.ctx.Enter("ref_regexp #%d", i+1)
		if _, err := regexpCompileCached(r); err != nil {
			vd.ctx.Error(err)
		}
		if !regexps.Add(r) {
			vd.ctx.Errorf("duplicate regexp: %q", r)
		}
		vd.ctx.Exit()
	}
	for i, r := range gp.RefRegexpExclude {
		vd.ctx.Enter("ref_regexp_exclude #%d", i+1)
		if _, err := regexpCompileCached(r); err != nil {
			vd.ctx.Error(err)
		}
		if !regexps.Add(r) {
			// There is no point excluding exact same regexp as including.
			vd.ctx.Errorf("duplicate regexp: %q", r)
		}
		vd.ctx.Exit()
	}
}

func (vd *projectConfigValidator) validateVerifiers(v *cfgpb.Verifiers, supportedModes stringset.Set) {
	if v.Cqlinter != nil {
		vd.ctx.Errorf("cqlinter verifier is not allowed (internal use only)")
	}
	if v.Fake != nil {
		vd.ctx.Errorf("fake verifier is not allowed (internal use only)")
	}
	if v.TreeStatus != nil {
		vd.ctx.Enter("tree_status")
		switch {
		case v.TreeStatus.GetTreeName() == "" && v.TreeStatus.GetUrl() == "":
			vd.ctx.Errorf("tree name is required")
		case v.TreeStatus.Url != "":
			switch u, err := url.Parse(v.TreeStatus.Url); {
			case err != nil:
				vd.ctx.Errorf("failed to parse url %q: %s", v.TreeStatus.Url, err)
			case u.Scheme != "https":
				vd.ctx.Errorf("url scheme must be 'https'")
			}
		}
		vd.ctx.Exit()
	}
	if v.GerritCqAbility == nil {
		vd.ctx.Errorf("gerrit_cq_ability verifier is required")
	} else {
		vd.ctx.Enter("gerrit_cq_ability")
		if len(v.GerritCqAbility.CommitterList) == 0 {
			vd.ctx.Errorf("committer_list is required")
		} else {
			for i, l := range v.GerritCqAbility.CommitterList {
				if l == "" {
					vd.ctx.Enter("committer_list #%d", i+1)
					vd.ctx.Errorf("must not be empty string")
					vd.ctx.Exit()
				}
			}
		}
		for i, l := range v.GerritCqAbility.DryRunAccessList {
			if l == "" {
				vd.ctx.Enter("dry_run_access_list #%d", i+1)
				vd.ctx.Errorf("must not be empty string")
				vd.ctx.Exit()
			}
		}
		for i, l := range v.GerritCqAbility.NewPatchsetRunAccessList {
			if l == "" {
				vd.ctx.Enter("new_patchset_run_access_list #%d", i+1)
				vd.ctx.Errorf("must not be empty string")
				vd.ctx.Exit()
			}
		}
		vd.ctx.Exit()
	}
	if v.Tryjob != nil && len(v.Tryjob.Builders) > 0 {
		vd.ctx.Enter("tryjob")
		vd.validateTryjobVerifier(v, supportedModes)
		vd.ctx.Exit()
	}
}

// validateTryjobVerifier validates the tryjob verifier in a config.
//
// The tryjob verifier generally includes multiple builders.
func (vd *projectConfigValidator) validateTryjobVerifier(v *cfgpb.Verifiers, supportedModes stringset.Set) {
	vt := v.Tryjob
	if vt.RetryConfig != nil {
		vd.ctx.Enter("retry_config")
		vd.validateTryjobRetry(vt.RetryConfig)
		vd.ctx.Exit()
	}

	switch vt.CancelStaleTryjobs {
	case cfgpb.Toggle_YES:
		vd.ctx.Errorf("`cancel_stale_tryjobs: YES` matches default CQ behavior now; please remove")
	case cfgpb.Toggle_NO:
		vd.ctx.Errorf("`cancel_stale_tryjobs: NO` is no longer supported, use per-builder `cancel_stale` instead")
	case cfgpb.Toggle_UNSET:
		// OK
	}

	visitBuilders := func(cb func(b *cfgpb.Verifiers_Tryjob_Builder)) {
		for i, b := range vt.Builders {
			enter(vd.ctx, "builders", i, b.Name)
			cb(b)
			vd.ctx.Exit()
		}
	}

	// Here we iterate through all builders here and accumulate a set that
	// contains all builder names. Names are validated when added to the set.
	// This includes checking for duplicates. This is done before the main
	// verification pass below so that all builder names are available below.
	builderNames := stringset.Set{}
	equiBuilderNames := stringset.Set{}
	visitBuilders(func(b *cfgpb.Verifiers_Tryjob_Builder) {
		vd.validateBuilderName(b.Name, builderNames)
	})

	visitBuilders(func(b *cfgpb.Verifiers_Tryjob_Builder) {
		if b.EquivalentTo != nil {
			vd.validateEquivalentBuilder(b.EquivalentTo, equiBuilderNames)
			if b.ExperimentPercentage != 0 {
				vd.ctx.Errorf("experiment_percentage is not combinable with equivalent_to")
			}
			if b.EquivalentTo.Name != "" && builderNames.Has(b.EquivalentTo.Name) {
				vd.ctx.Errorf("equivalent_to.name must not refer to already defined %q builder", b.EquivalentTo.Name)
			}
		}
		if b.ExperimentPercentage != 0 {
			if b.ExperimentPercentage < 0.0 || b.ExperimentPercentage > 100.0 {
				vd.ctx.Errorf("experiment_percentage must between 0 and 100 (%f given)", b.ExperimentPercentage)
			}
			if b.IncludableOnly {
				vd.ctx.Errorf("includable_only is not combinable with experiment_percentage")
			}
		}
		if len(b.LocationFilters) > 0 {
			vd.validateLocationFilters(b.GetLocationFilters())
			if b.IncludableOnly {
				vd.ctx.Errorf("includable_only is not combinable with location_filters")
			}
		}

		if len(b.OwnerWhitelistGroup) > 0 {
			for i, g := range b.OwnerWhitelistGroup {
				if g == "" {
					vd.ctx.Enter("owner_whitelist_group #%d", i+1)
					vd.ctx.Errorf("must not be empty string")
					vd.ctx.Exit()
				}
			}
		}

		var isAnalyzer bool
		if len(b.ModeAllowlist) > 0 {
			for i, m := range b.ModeAllowlist {
				switch {
				case !supportedModes.Has(m):
					vd.ctx.Enter("mode_allowlist #%d", i+1)
					vd.ctx.Errorf("must be one of %s", supportedModes.ToSortedSlice())
					vd.ctx.Exit()
				case m == "NEW_PATCHSET_RUN" && len(v.GetGerritCqAbility().GetNewPatchsetRunAccessList()) == 0:
					vd.ctx.Enter("mode_allowlist #%d", i+1)
					vd.ctx.Errorf("mode NEW_PATCHSET_RUN cannot be used unless a new_patchset_run_access_list is set")
					vd.ctx.Exit()
				case m == analyzerRun:
					isAnalyzer = true
				}
			}
			if isAnalyzer {
				// TODO(crbug/1202952): Remove the following check after Tricium is folded into CV.
				for i, f := range b.LocationFilters {
					vd.ctx.Enter("location_filters #%d", i+1)
					if !analyzerPathReRegexp.MatchString(f.PathRegexp) {
						vd.ctx.Errorf(`analyzer location filter path pattern must match %q.`, analyzerPathReRegexp)
					}
					if (!matchAll(f.GerritProjectRegexp) && matchAll(f.GerritHostRegexp)) ||
						(matchAll(f.GerritProjectRegexp) && !matchAll(f.GerritHostRegexp)) {
						vd.ctx.Errorf(`analyzer location filter must include both host and project or neither.`)
					}
					if f.Exclude {
						vd.ctx.Errorf(`location_filters exclude filters are not combinable with analyzer mode`)
					}
					vd.ctx.Exit()
				}
			}
			// TODO(crbug/1191855): See if CV should loosen the following restrictions.
			if b.IncludableOnly {
				vd.ctx.Errorf("includable_only is not combinable with mode_allowlist")
			}
		}

		for i, skipFooter := range b.GetSkipFooters() {
			vd.ctx.Enter("skip_footers #%d", i+1)
			switch footerKey := skipFooter.GetKey(); {
			case footerKey == "":
				vd.ctx.Errorf("footer key is required")
			case footerKey != footer.NormalizeKey(footerKey):
				vd.ctx.Errorf("footer key is not normalized")
			}
			if valueRegexp := skipFooter.GetValueRegexp(); valueRegexp != "" {
				validateRegexp(vd.ctx, "value_regexp", []string{valueRegexp})
			}
			vd.ctx.Exit()
		}
	})
}

func matchAll(re string) bool {
	return re == "" || re == ".*" || re == ".+"
}

// Validate a builder name. If knownNames is non-nil, then add it to the set.
func (vd *projectConfigValidator) validateBuilderName(name string, knownNames stringset.Set) {
	if name == "" {
		vd.ctx.Errorf("name is required")
		return
	}
	if knownNames != nil {
		if !knownNames.Add(name) {
			vd.ctx.Errorf("duplicate name %q", name)
		}
	}
	parts := strings.Split(name, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		vd.ctx.Errorf("name %q doesn't match required format project/short-bucket-name/builder, e.g. 'v8/try/linux'", name)
	}
	for _, part := range parts {
		subs := strings.Split(part, ".")
		if len(subs) >= 3 && subs[0] == "luci" {
			// Technically, this is allowed. However, practically, this is
			// extremely likely to be misunderstanding of project or bucket is.
			vd.ctx.Errorf("name %q is highly likely malformed; it should be project/short-bucket-name/builder, e.g. 'v8/try/linux'", name)
			return
		}
	}
	if err := luciconfig.ValidateProjectName(parts[0]); err != nil {
		vd.ctx.Errorf("first part of %q is not a valid LUCI project name", name)
	}
}

func (vd *projectConfigValidator) validateEquivalentBuilder(b *cfgpb.Verifiers_Tryjob_EquivalentBuilder, equiNames stringset.Set) {
	vd.ctx.Enter("equivalent_to")
	defer vd.ctx.Exit()
	vd.validateBuilderName(b.Name, equiNames)
	if b.Percentage < 0 || b.Percentage > 100 {
		vd.ctx.Errorf("percentage must be between 0 and 100 (%f given)", b.Percentage)
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

// validateLocationFilters validates that all location filters have valid
// regular expressions.
func (vd *projectConfigValidator) validateLocationFilters(filters []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter) {
	for i, filter := range filters {
		vd.ctx.Enter("location_filters #%d", i+1)
		if filter == nil {
			vd.ctx.Errorf("must not be nil")
			continue
		}

		if hostRE := filter.GetGerritHostRegexp(); hostRE != "" {
			vd.ctx.Enter("gerrit_host_regexp")
			if strings.HasPrefix(hostRE, "http") {
				vd.ctx.Errorf("scheme (http:// or https://) is not needed")
			}
			if _, err := regexpCompileCached(hostRE); err != nil {
				vd.ctx.Errorf("invalid regexp: %q; error: %s", hostRE, err)
			}
			vd.ctx.Exit()
		}

		if repoRE := filter.GetGerritProjectRegexp(); repoRE != "" {
			vd.ctx.Enter("gerrit_project_regexp")
			if _, err := regexpCompileCached(repoRE); err != nil {
				vd.ctx.Errorf("invalid regexp: %q; error: %s", repoRE, err)
			}
			vd.ctx.Exit()
		}

		if pathRE := filter.GetPathRegexp(); pathRE != "" {
			vd.ctx.Enter("path_regexp")
			if _, err := regexpCompileCached(pathRE); err != nil {
				vd.ctx.Errorf("invalid regexp: %q; error: %s", pathRE, err)
			}
			vd.ctx.Exit()
		}
		vd.ctx.Exit()
	}
}

func (vd *projectConfigValidator) validateTryjobRetry(r *cfgpb.Verifiers_Tryjob_RetryConfig) {
	if r.SingleQuota < 0 {
		vd.ctx.Errorf("negative single_quota not allowed (%d given)", r.SingleQuota)
	}
	if r.GlobalQuota < 0 {
		vd.ctx.Errorf("negative global_quota not allowed (%d given)", r.GlobalQuota)
	}
	if r.FailureWeight < 0 {
		vd.ctx.Errorf("negative failure_weight not allowed (%d given)", r.FailureWeight)
	}
	if r.TransientFailureWeight < 0 {
		vd.ctx.Errorf("negative transitive_failure_weight not allowed (%d given)", r.TransientFailureWeight)
	}
	if r.TimeoutWeight < 0 {
		vd.ctx.Errorf("negative timeout_weight not allowed (%d given)", r.TimeoutWeight)
	}
}

func (vd *projectConfigValidator) validateUserLimits(limits []*cfgpb.UserLimit, def *cfgpb.UserLimit) {
	names := stringset.New(len(limits))
	for i, l := range limits {
		vd.ctx.Enter("user_limits #%d", i+1)
		if l == nil {
			vd.ctx.Errorf("cannot be nil")
		} else {
			vd.validateUserLimit(l, names, true)
		}
		vd.ctx.Exit()
	}

	if def != nil {
		vd.ctx.Enter("user_limit_default")
		vd.validateUserLimit(def, names, false)
		vd.ctx.Exit()
	}
}

// validateUserLimit validates one cfgpb.UserLimit.
func (vd *projectConfigValidator) validateUserLimit(limit *cfgpb.UserLimit, namesSeen stringset.Set, principalsRequired bool) {
	vd.ctx.Enter("name")
	if !namesSeen.Add(limit.GetName()) {
		vd.ctx.Errorf("duplicate name %q", limit.GetName())
	}
	if !limitNameRe.MatchString(limit.GetName()) {
		vd.ctx.Errorf("%q does not match %q", limit.GetName(), limitNameRe)
	}
	vd.ctx.Exit()

	vd.ctx.Enter("principals")
	switch {
	case principalsRequired && len(limit.GetPrincipals()) == 0:
		vd.ctx.Errorf("must have at least one principal")
	case !principalsRequired && len(limit.GetPrincipals()) > 0:
		vd.ctx.Errorf("must not have any principals (%d principal(s) given)", len(limit.GetPrincipals()))
	}
	vd.ctx.Exit()

	for i, id := range limit.GetPrincipals() {
		vd.ctx.Enter("principals #%d", i+1)
		if err := vd.validatePrincipalID(id); err != nil {
			vd.ctx.Errorf("%s", err)
		}
		vd.ctx.Exit()
	}

	vd.ctx.Enter("run")
	switch r := limit.GetRun(); {
	case r == nil:
		vd.ctx.Errorf("missing; set all limits with `unlimited` if there are no limits")
	default:
		vd.ctx.Enter("max_active")
		if err := vd.validateLimit(r.GetMaxActive()); err != nil {
			vd.ctx.Errorf("%s", err)
		}
		vd.ctx.Exit()
	}
	vd.ctx.Exit()
}

func (vd *projectConfigValidator) validatePrincipalID(id string) error {
	chunks := strings.Split(id, ":")
	if len(chunks) != 2 || chunks[0] == "" || chunks[1] == "" {
		return fmt.Errorf("%q doesn't look like a principal id (<type>:<id>)", id)
	}

	switch chunks[0] {
	case "group":
		return nil // Any non-empty group name is OK
	case "user":
		// Should be a valid identity.
		_, err := identity.MakeIdentity(id)
		return err
	}
	return fmt.Errorf("unknown principal type %q", chunks[0])
}

func (vd *projectConfigValidator) validateLimit(l *cfgpb.UserLimit_Limit) error {
	switch l.GetLimit().(type) {
	case *cfgpb.UserLimit_Limit_Unlimited:
	case *cfgpb.UserLimit_Limit_Value:
		if val := l.GetValue(); val < 1 {
			return errors.Reason("invalid limit %d; must be > 0", val).Err()
		}
	case nil:
		return errors.Reason("missing; set `unlimited` if there is no limit").Err()
	default:
		return errors.Reason("unknown limit type %T", l.GetLimit()).Err()
	}
	return nil
}
