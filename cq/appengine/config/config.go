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

// package config implements validation and common manipulation of CQ config
// files.
package config

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"

	v1 "go.chromium.org/luci/cq/api/config/v1"
	v2 "go.chromium.org/luci/cq/api/config/v2"
)

// Config validation rules go here.

func init() {
	validation.Rules.Add("regex:projects/[^/]+", "cq.cfg", validateProject)
	validation.Rules.Add("regex:projects/[^/]+/refs/.+", "cq.cfg", validateRef)
}

// validateRefCfg validates legacy ref-specific cq.cfg.
// Validation result is returned via validation ctx,
// while error returned directly implies only a bug in this code.
func validateRef(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := v1.Config{}
	if err := proto.UnmarshalText(string(content), &cfg); err != nil {
		ctx.Error(err)
		return nil
	}
	validateV1(ctx, &cfg)
	return nil
}

// validateProjectCfg validates project-level cq.cfg.
// Validation result is returned via validation ctx,
// while error returned directly implies only a bug in this code.
func validateProject(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := v2.Config{}
	if err := proto.UnmarshalText(string(content), &cfg); err != nil {
		ctx.Error(err)
	} else {
		validateProjectConfig(ctx, &cfg)
	}
	return nil
}

func validateProjectConfig(ctx *validation.Context, cfg *v2.Config) {
	if cfg.DrainingStartTime != "" {
		if _, err := time.Parse(time.RFC3339, cfg.DrainingStartTime); err != nil {
			ctx.Errorf("failed to parse draining_start_time %q as RFC3339 format: %s", cfg.DrainingStartTime, err)
		}
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
		if cfg.SubmitOptions.BurstDelay != nil {
			switch d, err := ptypes.Duration(cfg.SubmitOptions.BurstDelay); {
			case err != nil:
				ctx.Errorf("invalid burst_delay: %s", err)
			case d.Seconds() < 0.0:
				ctx.Errorf("burst_delay must be positive or 0")
			}
		}
		ctx.Exit()
	}
	if len(cfg.ConfigGroups) == 0 {
		ctx.Errorf("at least 1 config_group is required")
		return
	}
	for i, g := range cfg.ConfigGroups {
		ctx.Enter("config_group #%d", i+1)
		validateConfigGroup(ctx, g)
		ctx.Exit()
	}
	ensureAtMost1Project(ctx, cfg)
	bestEffortDisjointGroups(ctx, cfg)
}

// ensureAtMost1Project enforces temporary CQ limitation of supporting only a
// single Gerrit url/project pair.
// TODO(tandrii): remove the single project single gerrit limitation.
func ensureAtMost1Project(ctx *validation.Context, cfg *v2.Config) {
	gerritURLs := stringset.Set{}
	projectNames := stringset.Set{}
	for _, gr := range cfg.ConfigGroups {
		for _, g := range gr.Gerrit {
			gerritURLs.Add(g.Url)
			for _, p := range g.Projects {
				projectNames.Add(p.Name)
			}
		}
	}
	// Ignore empty URLs and project names, because those already resulted in
	// error earlier.
	gerritURLs.Del("")
	projectNames.Del("")
	if gerritURLs.Len() > 1 {
		urls := gerritURLs.ToSlice()
		sort.Strings(urls)
		ctx.Errorf("more than 1 different gerrit url not **yet** allowed (given: %q)", urls)
	}
	if projectNames.Len() > 1 {
		names := projectNames.ToSlice()
		sort.Strings(names)
		ctx.Errorf("more than 1 different gerrit project names not **yet** allowed (given: %q)", names)
	}
}

// bestEffortDisjointGroups errors out on easy to spot overlaps between
// configGroups.
//
// It is non-trivial if it all possible to ensure that regexp across
// config_groups don't overlap. But, we can catch typical copy-pasta mistakes
// early on by checking for equality of regexps.
func bestEffortDisjointGroups(ctx *validation.Context, cfg *v2.Config) {
	defaultRefRegexps := []string{"refs/heads/master"}
	key := func(g, p, r string) string { return strings.Join([]string{g, p, r}, "\x00") }
	seen := map[string]int{}
	for grIdx, gr := range cfg.ConfigGroups {
		for gIdx, g := range gr.Gerrit {
			for pIdx, p := range g.Projects {
				refRegexps := p.RefRegexp
				if len(p.RefRegexp) == 0 {
					refRegexps = defaultRefRegexps
				}
				for rIdx, refRegexp := range refRegexps {
					k := key(g.Url, p.Name, refRegexp)
					if seenIdx, aliasing := seen[k]; !aliasing {
						seen[k] = grIdx
					} else if seenIdx != grIdx {
						// NOTE: we have already emitted error on duplicate gerrit URL,
						// project name, or ref_regexp within their own respective
						// container, so only error here is cases when these span multiple
						// config_groups.
						ctx.Enter("config_group #%d", grIdx+1)
						ctx.Enter("gerrit #%d", gIdx+1)
						ctx.Enter("project #%d", pIdx+1)
						ctx.Enter("ref_regexp #%d", rIdx+1)
						ctx.Errorf("aliases config_group #%d", seenIdx+1)
						ctx.Exit()
						ctx.Exit()
						ctx.Exit()
						ctx.Exit()
					}
				}
			}
		}
	}
}

func validateConfigGroup(ctx *validation.Context, group *v2.ConfigGroup) {
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

	if group.Verifiers == nil {
		ctx.Errorf("verifiers are required")
	} else {
		ctx.Enter("verifiers")
		validateVerifiers(ctx, group.Verifiers)
		ctx.Exit()
	}
}

func validateGerrit(ctx *validation.Context, g *v2.ConfigGroup_Gerrit) {
	validateGerritURL(ctx, g.Url)
	if len(g.Projects) == 0 {
		ctx.Errorf("at least 1 project is required")
	}
	projectNames := stringset.Set{}
	for i, p := range g.Projects {
		ctx.Enter("projects #%d", i+1)
		validateGerritProject(ctx, p)
		if p.Name != "" && !projectNames.Add(p.Name) {
			ctx.Errorf("duplicate project in the same gerrit: %q", p.Name)
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

func validateGerritProject(ctx *validation.Context, gp *v2.ConfigGroup_Gerrit_Project) {
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
		if _, err := regexp.Compile(r); err != nil {
			ctx.Error(err)
		}
		if !regexps.Add(r) {
			ctx.Errorf("duplicate regexp: %q", r)
		}
		ctx.Exit()
	}
}

func validateVerifiers(ctx *validation.Context, v *v2.Verifiers) {
	if v.Deprecator != nil {
		ctx.Errorf("deprecator verifier is not allowed (internal use only)")
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
		if v.GerritCqAbility.CommitterList == "" {
			ctx.Errorf("committer_list is required")
		}
		ctx.Exit()
	}
	if v.Tryjob != nil {
		ctx.Enter("tryjob")
		validateTryjobVerifier(ctx, v.Tryjob)
		ctx.Exit()
	}
}

func validateTryjobVerifier(ctx *validation.Context, v *v2.Verifiers_Tryjob) {
	// TODO(tandrii): implement.
}
