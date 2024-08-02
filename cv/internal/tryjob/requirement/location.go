// Copyright 2022 The LUCI Authors.
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

package requirement

import (
	"context"
	"fmt"
	"regexp"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/run"
)

// locationFilterMatch returns true if a builder is included, given
// the location filters and CLs (and their file paths).
func locationFilterMatch(ctx context.Context, locationFilters []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter, cls []*run.RunCL) (bool, error) {
	if len(locationFilters) == 0 {
		// If there are no location filters, the builder is included.
		return true, nil
	}

	// For efficiency, pre-compile all regexes here. This also checks whether
	// the regexes are valid.
	compiled, err := compileLocationFilters(locationFilters)
	if err != nil {
		return false, err
	}

	for _, cl := range cls {
		gerrit := cl.Detail.GetGerrit()
		if gerrit == nil {
			// Could result from error or if there is an non-Gerrit backend.
			return false, errors.New("empty Gerrit detail")
		}
		host := gerrit.GetHost()
		project := gerrit.GetInfo().GetProject()
		ref := gerrit.GetInfo().GetRef()

		if isMergeCommit(ctx, gerrit) {
			if hostProjectAndRefMatch(compiled, host, project, ref) {
				// Gerrit treats CLs representing merged commits (i.e. CLs with with a
				// git commit with multiple parents) as having no file diff. There may
				// also be no file diff if there is no longer a diff after rebase.
				// For merge commits, we want to avoid inadvertently landing
				// such a CLs without triggering any builders.
				//
				// If there is a CL which is a merge commit, and the builder would
				// be triggered for some files in that repo, then trigger the builder.
				// See crbug/1006534.
				return true, nil
			}
			continue
		}
		// Iterate through all files to try to find a match.
		// If there are no files, but this is not a merge commit, then do
		// nothing for this CL.
		for _, path := range gerrit.GetFiles() {
			// If the first filter is an exclude filter, then include by default, and
			// vice versa.
			included := locationFilters[0].Exclude
			// Whether the file is included is determined by the last filter to match.
			// So we can iterate through the filters backwards and break when we have
			// a match.
			for i := len(compiled) - 1; i >= 0; i-- {
				f := compiled[i]
				// Check for inclusion; if it matches then this is the filter
				// that applies.
				if match(f.hostRE, host) && match(f.projectRE, project) && match(f.refRE, ref) && match(f.pathRE, path) {
					included = !f.exclude
					break
				}
			}
			// If at least one file in one CL is included, then the builder is included.
			if included {
				return true, nil
			}
		}
	}

	// After looping through all files in all CLs, all were considered
	// excluded, so the builder should not be triggered.
	return false, nil
}

// match is like re.MatchString, but also matches if the regex is nil.
func match(re *regexp.Regexp, str string) bool {
	return re == nil || re.MatchString(str)
}

// compileLocationFilters precompiles regexes in a LocationFilter.
//
// Returns an error if a regex is invalid.
func compileLocationFilters(locationFilters []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter) ([]compiledLocationFilter, error) {
	ret := make([]compiledLocationFilter, len(locationFilters))
	for i, lf := range locationFilters {
		var err error
		if lf.GerritHostRegexp != "" {
			ret[i].hostRE, err = regexp.Compile(fmt.Sprintf("^%s$", lf.GerritHostRegexp))
			if err != nil {
				return nil, err
			}
		}
		if lf.GerritProjectRegexp != "" {
			ret[i].projectRE, err = regexp.Compile(fmt.Sprintf("^%s$", lf.GerritProjectRegexp))
			if err != nil {
				return nil, err
			}
		}
		if lf.GerritRefRegexp != "" {
			ret[i].refRE, err = regexp.Compile(fmt.Sprintf("^%s$", lf.GerritRefRegexp))
			if err != nil {
				return nil, err
			}
		}
		if lf.PathRegexp != "" {
			ret[i].pathRE, err = regexp.Compile(fmt.Sprintf("^%s$", lf.PathRegexp))
			if err != nil {
				return nil, err
			}
		}
		ret[i].exclude = lf.Exclude
	}
	return ret, nil
}

// compiledLocationFilter stores the same information as the LocationFilter
// message in the config proto, but with compiled regexes.
type compiledLocationFilter struct {
	// Compiled regexes; nil if the regex is nil in the LocationFilter.
	hostRE, projectRE, refRE, pathRE *regexp.Regexp
	// Whether this filter is an exclude filter.
	exclude bool
}

// hostProjectAndRefMatch returns true if the Gerrit host, project, ref could
// match the filters (for any possible files).
func hostProjectAndRefMatch(compiled []compiledLocationFilter, host, project, ref string) bool {
	for _, f := range compiled {
		if !f.exclude && match(f.hostRE, host) && match(f.projectRE, project) && match(f.refRE, ref) {
			return true
		}
	}
	// If the first filter is an exclude filter, we include by default;
	// exclude by default.
	return compiled[0].exclude
}

// isMergeCommit checks whether the current revision of the change is a merge
// commit based on available Gerrit information.
func isMergeCommit(ctx context.Context, g *changelist.Gerrit) bool {
	i := g.GetInfo()
	rev, ok := i.GetRevisions()[i.GetCurrentRevision()]
	if !ok {
		logging.Errorf(ctx, "No current revision in ChangeInfo when checking isMergeCommit, got %+v", i)
		return false
	}
	return len(rev.GetCommit().GetParents()) > 1 && len(g.GetFiles()) == 0
}

// locationMatch returns true if the builder should be included given the
// location regexp fields and CLs.
//
// The builder is included if at least one file from at least one CL matches
// a locationRegexp pattern and does not match any locationRegexpExclude
// patterns.
//
// Note that an empty locationRegexp is treated equivalently to a `.*` value.
//
// Panics if any regex is invalid.
func locationMatch(ctx context.Context, locationRegexp, locationRegexpExclude []string, cls []*run.RunCL) (bool, error) {
	if len(locationRegexp) == 0 {
		locationRegexp = append(locationRegexp, ".*")
	}
	changedLocations := make(stringset.Set)
	for _, cl := range cls {
		if isMergeCommit(ctx, cl.Detail.GetGerrit()) {
			// Merge commits have zero changed files. We don't want to land such
			// changes without triggering any builders, so we ignore location filters
			// if there are any CLs that are merge commits. See crbug/1006534.
			return true, nil
		}
		host, project := cl.Detail.GetGerrit().GetHost(), cl.Detail.GetGerrit().GetInfo().GetProject()
		for _, f := range cl.Detail.GetGerrit().GetFiles() {
			changedLocations.Add(fmt.Sprintf("https://%s/%s/+/%s", host, project, f))
		}
	}
	// First remove from the list the files that match locationRegexpExclude, and
	for _, lre := range locationRegexpExclude {
		re := regexp.MustCompile(fmt.Sprintf("^%s$", lre))
		changedLocations.Iter(func(loc string) bool {
			if re.MatchString(loc) {
				changedLocations.Del(loc)
			}
			return true
		})
	}
	if changedLocations.Len() == 0 {
		// Any locations touched by the change have been excluded by the
		// builder's config.
		return false, nil
	}
	// Matching the remainder against locationRegexp, returning true if there's
	// a match.
	for _, lri := range locationRegexp {
		re := regexp.MustCompile(fmt.Sprintf("^%s$", lri))
		found := false
		changedLocations.Iter(func(loc string) bool {
			if re.MatchString(loc) {
				found = true
				return false
			}
			return true
		})
		if found {
			return true, nil
		}
	}
	return false, nil
}
