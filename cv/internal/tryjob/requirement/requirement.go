// Copyright 2021 The LUCI Authors.
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
	"crypto"
	"encoding/binary"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/server/auth"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/buildbucket"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// Input contains all info needed to compute the Tryjob Requirement.
type Input struct {
	ConfigGroup *cfgpb.ConfigGroup
	RunOwner    identity.Identity
	CLs         []*run.RunCL
	RunOptions  *run.Options
	RunMode     run.Mode
}

// ComputationFailure is what fails the Tryjob Requirement computation.
type ComputationFailure interface {
	// Reason returns a human-readable string that explains what fails the
	// requirement computation.
	//
	// This will be shown directly to users. Make sure it doesn't leak any
	// information.
	Reason() string
}

// ComputationResult is the result of Tryjob Requirement computation.
type ComputationResult struct {
	// Requirement is the derived Tryjob Requirement to verify the Run.
	//
	// Mutually exclusive with `ComputationFailure`.
	Requirement *tryjob.Requirement
	// ComputationFailure is what fails the Requirement computation.
	//
	// This is different from the returned error from the `Compute` function.
	// This failure is typically caused by invalid directive from users or
	// Project Config (e.g. including a builder that is not defined in Project
	// config via git-footer). It should be reported back to the user to decide
	// the next step. On the other hand, the returned error from the `Compute`
	// function is typically caused by internal errors, like remote RPC call
	// failure, and should generally be retried.
	//
	// Mutually exclusive with `Requirement`.
	ComputationFailure ComputationFailure
}

// OK returns true if the Tryjob Requirement is successfully computed.
//
// `ComputationFailure` MUST be present if false is returned.
func (r ComputationResult) OK() bool {
	switch {
	case r.ComputationFailure != nil:
		if r.Requirement != nil {
			panic(fmt.Errorf("both Requirement and ComputationFailure are present"))
		}
		return false
	case r.Requirement == nil:
		panic(fmt.Errorf("neither Requirement nor ComputationFailure is present"))
	default:
		return true
	}
}

// getDisallowedOwners checks which of the owner emails given belong
// to none of the given allowLists.
//
// NOTE: If no allowLists are given, the return value defaults to nil. This may
// be unexpected, and has security implications. This behavior is valid in
// regards to checking whether the owners are allowed to use a certain builder,
// if no allowList groups are defined, then the expectation is that the action
// is allowed by default.
func getDisallowedOwners(ctx context.Context, allOwnerEmails []string, allowLists ...string) ([]string, error) {
	switch {
	case len(allOwnerEmails) == 0:
		panic(fmt.Errorf("cannot check membership of nil user"))
	case len(allowLists) == 0:
		return nil, nil
	}
	var disallowed []string
	for _, userEmail := range allOwnerEmails {
		id, err := identity.MakeIdentity(fmt.Sprintf("user:%s", userEmail))
		if err != nil {
			return nil, err
		}
		switch allowed, err := auth.GetState(ctx).DB().IsMember(ctx, id, allowLists); {
		case err != nil:
			return nil, err
		case !allowed:
			disallowed = append(disallowed, userEmail)
		}
	}
	return disallowed, nil
}

func isBuilderAllowed(ctx context.Context, allOwners []string, b *cfgpb.Verifiers_Tryjob_Builder) (bool, error) {
	if len(b.GetOwnerWhitelistGroup()) > 0 {
		switch disallowedOwners, err := getDisallowedOwners(ctx, allOwners, b.GetOwnerWhitelistGroup()...); {
		case err != nil:
			return false, err
		case len(disallowedOwners) > 0:
			return false, nil
		}
	}
	return true, nil

}

func isEquiBuilderAllowed(ctx context.Context, allOwners []string, b *cfgpb.Verifiers_Tryjob_EquivalentBuilder) (bool, error) {
	if b.GetOwnerWhitelistGroup() != "" {
		switch disallowedOwners, err := getDisallowedOwners(ctx, allOwners, b.GetOwnerWhitelistGroup()); {
		case err != nil:
			return false, err
		case len(disallowedOwners) > 0:
			return false, nil
		}
	}
	return true, nil
}

var (
	// Reference: https://chromium.googlesource.com/infra/luci/luci-py/+/a6655aa3/appengine/components/components/config/proto/service_config.proto#87
	projectRE = `[a-z0-9\-]+`
	// Reference: https://chromium.googlesource.com/infra/luci/luci-go/+/6b8fdd66/buildbucket/proto/project_config.proto#482
	bucketRE = `[a-z0-9\-_.]+`
	// Reference: https://chromium.googlesource.com/infra/luci/luci-go/+/6b8fdd66/buildbucket/proto/project_config.proto#220
	builderRE          = `[a-zA-Z0-9\-_.\(\) ]+`
	modernProjBucketRe = fmt.Sprintf(`%s/%s`, projectRE, bucketRE)
	legacyProjBucketRe = fmt.Sprintf(`luci\.%s\.%s`, projectRE, bucketRE)
	buildersRE         = fmt.Sprintf(`((%s)|(%s))\s*:\s*%s(\s*,\s*%s)*`, modernProjBucketRe, legacyProjBucketRe, builderRE, builderRE)
	includeLineRegexp  = regexp.MustCompile(fmt.Sprintf(`^\s*%s(\s*;\s*%s)*\s*$`, buildersRE, buildersRE))
)

// TODO(robertocn): Consider moving the parsing of the Cq-Include-Trybots
// directives to the place where the footer values are extracted, and refactor
// RunOptions.IncludeTrybots accordingly (e.g. to be a list of builder ids).
func parseBuilderStrings(line string) ([]string, ComputationFailure) {
	if !includeLineRegexp.MatchString(line) {
		return nil, &invalidCQIncludedTryjobs{line}
	}
	var ret []string
	for _, bucketSegment := range strings.Split(strings.TrimSpace(line), ";") {
		parts := strings.Split(strings.TrimSpace(bucketSegment), ":")
		if len(parts) != 2 {
			panic(fmt.Errorf("impossible; expected %q separated by exactly one \":\", got %d", bucketSegment, len(parts)-1))
		}
		projectBucket, builders := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		var project, bucket string
		if strings.HasPrefix(projectBucket, "luci.") {
			// Legacy style. Example: luci.chromium.try: builder_a
			parts := strings.SplitN(projectBucket, ".", 3)
			project, bucket = parts[1], parts[2]
		} else {
			// Modern style. Example: chromium/try: builder_a
			parts := strings.SplitN(projectBucket, "/", 2)
			project, bucket = parts[0], parts[1]
		}
		for _, builderName := range strings.Split(builders, ",") {
			ret = append(ret, fmt.Sprintf("%s/%s/%s", strings.TrimSpace(project), strings.TrimSpace(bucket), strings.TrimSpace(builderName)))
		}
	}
	return ret, nil
}

// getIncludedTryjobs checks the given CLs for footers and tags that request
// specific builders to be included in the tryjobs to require.
func getIncludedTryjobs(directives []string) (stringset.Set, ComputationFailure) {
	ret := make(stringset.Set)
	for _, d := range directives {
		builderStrings, compFail := parseBuilderStrings(d)
		if compFail != nil {
			return nil, compFail
		}
		for _, builderString := range builderStrings {
			ret.Add(builderString)
		}
	}
	return ret, nil
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

// makeBuildbucketDefinition converts a builder name to a minimal Definition.
func makeBuildbucketDefinition(builderName string) *tryjob.Definition {
	if builderName == "" {
		panic(fmt.Errorf("builderName unexpectedly empty"))
	}
	builderID, err := buildbucket.ParseBuilderID(builderName)
	if err != nil {
		panic(err)
	}

	return &tryjob.Definition{
		Backend: &tryjob.Definition_Buildbucket_{
			Buildbucket: &tryjob.Definition_Buildbucket{
				Host:    chromeinfra.BuildbucketHost,
				Builder: builderID,
			},
		},
	}
}

type criticality bool

const (
	critical    criticality = true
	nonCritical criticality = false
)

type equivalentUsage int

const (
	mainOnly equivalentUsage = iota
	equivalentOnly
	bothMainAndEquivalent
	flipMainAndEquivalent
)

// makeDefinition creates a Tryjob Definition for the given builder names and
// reuse flag.
func makeDefinition(builder *cfgpb.Verifiers_Tryjob_Builder, useEquivalent equivalentUsage, isCritical criticality) *tryjob.Definition {
	var definition *tryjob.Definition
	switch useEquivalent {
	case mainOnly:
		definition = makeBuildbucketDefinition(builder.Name)
	case equivalentOnly:
		definition = makeBuildbucketDefinition(builder.EquivalentTo.Name)
	case bothMainAndEquivalent:
		definition = makeBuildbucketDefinition(builder.Name)
		definition.EquivalentTo = makeBuildbucketDefinition(builder.EquivalentTo.Name)
	case flipMainAndEquivalent:
		definition = makeBuildbucketDefinition(builder.EquivalentTo.Name)
		definition.EquivalentTo = makeBuildbucketDefinition(builder.Name)
	default:
		panic(fmt.Errorf("unknown useEquivalent(%d)", useEquivalent))
	}
	definition.DisableReuse = builder.DisableReuse
	definition.Critical = bool(isCritical)
	definition.Experimental = builder.ExperimentPercentage > 0
	definition.ResultVisibility = builder.ResultVisibility
	definition.SkipStaleCheck = builder.GetCancelStale() == cfgpb.Toggle_NO
	return definition
}

func isModeAllowed(mode run.Mode, allowedModes []string) bool {
	if len(allowedModes) == 0 {
		// If allowedModes is unspecified, a builder is allowed in all modes.
		return true
	}
	for _, allowed := range allowedModes {
		if string(mode) == allowed {
			return true
		}
	}
	return false
}

// makeRands makes `n` new pseudo-random generators to be used for the tryjob
// requirement computation's random selections (experiment, equivalentBuilder)
//
// The generators are seeded deterministically based on the set of CLs (their
// IDs, specifically) and their trigger times.
// We do it this way so that recomputing the requirement for the same run yields
// the same result, but triggering the same set of CLs subsequent times has a
// chance of generating a different set of random selections.
func makeRands(in Input, n int) []*rand.Rand {
	// Though MD5 is cryptographically broken, it's not being used here for
	// security purposes, and it's faster than SHA.
	h := crypto.MD5.New()
	buf := make([]byte, 8)
	cls := make([]*run.RunCL, 0, len(in.CLs))
	copy(cls, in.CLs)
	sort.Slice(cls, func(i, j int) bool { return cls[i].ID < cls[j].ID })
	var err error
	for _, cl := range cls {
		binary.LittleEndian.PutUint64(buf, uint64(cl.ID))
		_, err = h.Write(buf)
		if err == nil && cl.Trigger != nil {
			binary.LittleEndian.PutUint64(buf, uint64(cl.Trigger.Time.AsTime().UTC().Unix()))
			_, err = h.Write(buf)
		}
		if err != nil {
			panic(err)
		}
	}
	digest := h.Sum(nil)
	// Use the first eight bytes of the digest to seed a new rand, and use such
	// rand's generated values to seed the requested number of generators.
	baseRand := rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(digest[:8]))))
	ret := make([]*rand.Rand, n)
	for i := range ret {
		ret[i] = rand.New(rand.NewSource(baseRand.Int63()))
	}
	return ret
}

func isPresubmit(builder *cfgpb.Verifiers_Tryjob_Builder) bool {
	// TODO(crbug.com/1292195): Implement a different way of deciding that a
	// builder is presubmit.
	return builder.DisableReuse
}

type inclusionResult byte

const (
	skipBuilder inclusionResult = iota
	includeBuilder
	includeMainBuilderOnly
	includeEquiBuilderOnly
)

// shouldInclude decides based on the configuration whether a given builder
// should be skipped in generating the requirement.
func shouldInclude(ctx context.Context, in Input, er *rand.Rand, b *cfgpb.Verifiers_Tryjob_Builder, incl stringset.Set, owners []string) (inclusionResult, ComputationFailure, error) {
	switch ps := isPresubmit(b); {
	case in.RunOptions.GetSkipTryjobs() && !ps:
		return skipBuilder, nil, nil
	// TODO(crbug.com/950074): Remove this clause.
	case in.RunOptions.GetSkipPresubmit() && ps:
		return skipBuilder, nil, nil
	}

	// If the builder is triggered by another builder, it does not need to
	// be considered as a required tryjob.
	if b.TriggeredBy != "" {
		return skipBuilder, nil, nil
	}

	if incl.Del(b.Name) {
		switch disallowedOwners, err := getDisallowedOwners(ctx, owners, b.GetOwnerWhitelistGroup()...); {
		case err != nil:
			return skipBuilder, nil, err
		case len(disallowedOwners) != 0:
			// The requested builder is defined, but the owner is not allowed to include it.
			return skipBuilder, &unauthorizedIncludedTryjob{
				Users:   disallowedOwners,
				Builder: b.Name,
			}, nil
		}
		return includeMainBuilderOnly, nil, nil
	}

	if b.GetEquivalentTo() != nil && incl.Del(b.GetEquivalentTo().GetName()) {
		if ownerAllowGroup := b.GetEquivalentTo().GetOwnerWhitelistGroup(); ownerAllowGroup != "" {
			switch disallowedOwners, err := getDisallowedOwners(ctx, owners, ownerAllowGroup); {
			case err != nil:
				return skipBuilder, nil, err
			case len(disallowedOwners) != 0:
				// The requested builder is defined, but the owner is not allowed to include it.
				return skipBuilder, &unauthorizedIncludedTryjob{
					Users:   disallowedOwners,
					Builder: b.EquivalentTo.Name,
				}, nil
			}
		}
		return includeEquiBuilderOnly, nil, nil
	}

	if b.IncludableOnly {
		return skipBuilder, nil, nil
	}

	if !isModeAllowed(in.RunMode, b.ModeAllowlist) {
		return skipBuilder, nil, nil
	}

	// Check for LocationRegexp match to decide whether to skip the builder.
	// Also evaluate with LocationFilter (if it is set) to assess the
	// correctness of LocationFilter matching.
	locationRegexpMatched, err := locationMatch(ctx, b.LocationRegexp, b.LocationRegexpExclude, in.CLs)
	if err != nil {
		return skipBuilder, nil, err
	}
	locationFilterMatched, err := locationFilterMatch(ctx, b.LocationFilters, in.CLs)
	if err != nil {
		return skipBuilder, nil, err
	}
	locationFilterSpecified := len(b.LocationFilters) > 0
	locationRegexpSpecified := len(b.LocationRegexp)+len(b.LocationRegexpExclude) > 0
	switch {
	case locationRegexpSpecified && locationFilterSpecified:
		if locationRegexpMatched != locationFilterMatched {
			// If the result using LocationRegexp is not the same as LocationFilter,
			// we want to know about it because this means that locationFilterMatch
			// is not correct.
			logging.Fields{
				"location_regexp":         b.LocationRegexp,
				"location_regexp_exclude": b.LocationRegexpExclude,
				"location_regexp result":  locationRegexpMatched,
				"location_filters result": locationFilterMatched,
				"builder name":            b.Name,
			}.Errorf(ctx, "LocationFilters and LocationRegexp did not give the same result. LocationFilters: %+v", b.LocationFilters, in.CLs)

		}
		fallthrough
	case locationRegexpSpecified:
		// If LocationRegexp was specified, use it as the source of truth.
		if !locationRegexpMatched {
			return skipBuilder, nil, nil
		}
	case locationFilterSpecified:
		// If only LocationFilters was specified, use it as the source of truth.
		if !locationFilterMatched {
			return skipBuilder, nil, nil
		}
	}

	if b.ExperimentPercentage != 0 && er.Float32()*100 > b.ExperimentPercentage {
		return skipBuilder, nil, nil
	}

	switch allowed, err := isBuilderAllowed(ctx, owners, b); {
	case err != nil:
		return skipBuilder, nil, err
	case allowed:
		return includeBuilder, nil, nil
	case b.GetEquivalentTo() != nil:
		// See if the owners can trigger the equivalent builder instead.
		switch equiAllowed, err := isEquiBuilderAllowed(ctx, owners, b.GetEquivalentTo()); {
		case err != nil:
			return skipBuilder, nil, err
		case equiAllowed:
			return includeEquiBuilderOnly, nil, err
		default:
			return skipBuilder, nil, err
		}
	default:
		return skipBuilder, nil, nil
	}
}

// getIncludablesAndTriggeredBy computes the set of builders that it is valid to
// include as trybots in a CL, and those that cannot be included due to their
// being triggered by another builder.
func getIncludablesAndTriggeredBy(builders []*cfgpb.Verifiers_Tryjob_Builder) (includable, triggeredByOther stringset.Set) {
	includable = stringset.New(len(builders))
	triggeredByOther = stringset.New(len(builders))
	for _, b := range builders {
		switch {
		case b.TriggeredBy != "":
			triggeredByOther.Add(b.Name)
		case b.EquivalentTo != nil:
			includable.Add(b.EquivalentTo.Name)
			fallthrough
		default:
			includable.Add(b.Name)
		}

	}
	return includable, triggeredByOther
}

// Compute computes the tryjob requirement to verify the run.
func Compute(ctx context.Context, in Input) (*ComputationResult, error) {
	ret := &ComputationResult{Requirement: &tryjob.Requirement{
		RetryConfig: in.ConfigGroup.GetVerifiers().GetTryjob().GetRetryConfig(),
	}}

	explicitlyIncluded, compFail := getIncludedTryjobs(in.RunOptions.GetIncludedTryjobs())
	if compFail != nil {
		return &ComputationResult{ComputationFailure: compFail}, nil
	}
	includableBuilders, triggeredByBuilders := getIncludablesAndTriggeredBy(in.ConfigGroup.GetVerifiers().GetTryjob().GetBuilders())
	unincludable := explicitlyIncluded.Difference(includableBuilders)
	undefined := unincludable.Difference(triggeredByBuilders)
	switch {
	case len(undefined) != 0:
		return &ComputationResult{ComputationFailure: &buildersNotDefined{Builders: undefined.ToSlice()}}, nil
	case len(unincludable) != 0:
		return &ComputationResult{ComputationFailure: &buildersNotDirectlyIncludable{Builders: unincludable.ToSlice()}}, nil
	}

	ownersSet := stringset.New(len(in.CLs))
	for _, cl := range in.CLs {
		curr := cl.Detail.GetGerrit().GetInfo().GetOwner().GetEmail()
		if curr != "" {
			ownersSet.Add(curr)
		}
	}
	allOwnerEmails := ownersSet.ToSortedSlice()
	// Make 2 independent generators so that e.g. the addition of experimental
	// tryjobs during a Run's lifetime does not cause it to change its use of
	// equivalent builders upon recomputation.
	rands := makeRands(in, 2)
	experimentRand, equivalentBuilderRand := rands[0], rands[1]
	for _, builder := range in.ConfigGroup.GetVerifiers().GetTryjob().GetBuilders() {
		r, compFail, err := shouldInclude(ctx, in, experimentRand, builder, explicitlyIncluded, allOwnerEmails)
		switch {
		case err != nil:
			return nil, err
		case compFail != nil:
			return &ComputationResult{ComputationFailure: compFail}, nil
		case r == skipBuilder:
		case r == includeMainBuilderOnly:
			ret.Requirement.Definitions = append(ret.Requirement.Definitions, makeDefinition(builder, mainOnly, critical))
		case r == includeEquiBuilderOnly:
			ret.Requirement.Definitions = append(ret.Requirement.Definitions, makeDefinition(builder, equivalentOnly, critical))
		default:
			equivalence := mainOnly
			if builder.GetEquivalentTo() != nil && !in.RunOptions.GetSkipEquivalentBuilders() {
				equivalence = bothMainAndEquivalent
				switch allowed, err := isEquiBuilderAllowed(ctx, allOwnerEmails, builder.GetEquivalentTo()); {
				case err != nil:
					return nil, err
				case allowed:
					if equivalentBuilderRand.Float32()*100 <= builder.EquivalentTo.Percentage {
						// Invert equivalence: trigger equivalent, but accept reusing original too.
						equivalence = flipMainAndEquivalent
					}
				default:
					// Not allowed to use equivalent.
					equivalence = mainOnly
				}
			}
			ret.Requirement.Definitions = append(ret.Requirement.Definitions, makeDefinition(builder, equivalence, builder.ExperimentPercentage == 0))
		}
	}
	return ret, nil
}
