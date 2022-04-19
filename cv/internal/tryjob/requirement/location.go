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

	"go.chromium.org/luci/common/errors"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
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
	compiled, err := compileLocationFilters(ctx, locationFilters)
	if err != nil {
		return false, err
	}

	for _, cl := range cls {
		gerrit := cl.Detail.GetGerrit()
		switch {
		case gerrit == nil:
			// Could result from error or if there is an non-Gerrit backend.
			return false, errors.New("empty Gerrit detail")
		case len(gerrit.GetFiles()) == 0:
			// Gerrit treats CLs representing merged commits (i.e. CLs with with a
			// git commit with multiple parents) as having no file diff. In this
			// case, the builder is included. See crbug/1006534.
			return true, nil
		}
		host := gerrit.GetHost()
		project := gerrit.GetInfo().GetProject()

		// If the first filter is an exclude filter, then include by default, and
		// vice versa.
		included := locationFilters[0].Exclude

		for _, path := range gerrit.GetFiles() {
			// Whether the file is included is determined by the last filter to match.
			// So we can iterate through the filters backwards and break when we have
			// a match.
			for i := len(compiled) - 1; i >= 0; i-- {
				f := compiled[i]
				// Check for inclusion; if it matches then this is the filter
				// that applies.
				if match(f.hostRE, host) && match(f.projectRE, project) && match(f.pathRE, path) {
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

	// After looping through all files in all CLs, all were considered excluded.
	return false, nil
}

// match is like re.MatchString, but also matches if the regex is nil.
func match(re *regexp.Regexp, str string) bool {
	return re == nil || re.MatchString(str)
}

// compileLocationFilters precompiles regexes in a LocationFilter.
//
// Returns an error if a regex is invalid.
func compileLocationFilters(ctx context.Context, locationFilters []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter) ([]compiledLocationFilter, error) {
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
	hostRE, projectRE, pathRE *regexp.Regexp
	// Whether this filter is an exclude filter.
	exclude bool
}
