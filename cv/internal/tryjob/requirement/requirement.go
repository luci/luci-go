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
	"fmt"
	"regexp"
	"runtime"
	"sync/atomic"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/buildbucket/bbperms"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/buildbucket"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// Input contains all info needed to compute the Tryjob Requirement.
type Input struct {
	GFactory    gerrit.Factory
	ConfigGroup *cfgpb.ConfigGroup
	RunOwner    identity.Identity
	CLs         []*run.RunCL
	RunOptions  *run.Options
	RunMode     run.Mode
}

func (i Input) allCLOwnersSorted() []string {
	ownersSet := stringset.New(len(i.CLs))
	for _, cl := range i.CLs {
		curr := cl.Detail.GetGerrit().GetInfo().GetOwner().GetEmail()
		if curr != "" {
			ownersSet.Add(curr)
		}
	}
	return ownersSet.ToSortedSlice()
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

// Compute computes the Tryjob Requirement to verify the run.
func Compute(ctx context.Context, in Input) (*ComputationResult, error) {
	hasIncluded := len(in.RunOptions.GetIncludedTryjobs()) > 0
	hasOverridden := len(in.RunOptions.GetOverriddenTryjobs()) > 0
	var explicitlyIncluded stringset.Set
	switch {
	case hasIncluded && hasOverridden:
		// Only one of the two Tryjob related options can be specified.
		return &ComputationResult{
			ComputationFailure: &incompatibleTryjobOptions{
				hasIncludedTryjobs:   hasIncluded,
				hasOverriddenTryjobs: hasOverridden,
			},
		}, nil
	case hasOverridden:
		return handleOverriddenTryjobs(ctx, in)
	case hasIncluded && in.RunMode != run.NewPatchsetRun:
		// Ignore tryjobs included by the cq-include-trybots: footer for new
		// patchset run, these are intended for CQ-vote runs.
		var compFail ComputationFailure
		explicitlyIncluded, compFail = calculateExplicitlyIncluded(in)
		if compFail != nil {
			return &ComputationResult{
				ComputationFailure: compFail,
			}, nil
		}
	}

	// Make 2 independent generators so that e.g. the addition of optional
	// tryjobs during a Run's lifetime does not cause it to change its use of
	// equivalent builders upon recomputation.
	rands := makeRands(in, 2)
	optionalRand, equivalentBuilderRand := rands[0], rands[1]
	builders := in.ConfigGroup.GetVerifiers().GetTryjob().GetBuilders()
	allOwners := in.allCLOwnersSorted()
	experiments, err := computeEnabledExperiments(ctx, in, allOwners)
	if err != nil {
		return nil, err
	}

	definitions := make([]*tryjob.Definition, len(builders))
	var computationFailureHolder atomic.Value
	// Utilize multiple cores.
	err = parallel.WorkPool(min(len(builders), runtime.NumCPU()), func(work chan<- func() error) {
		for i, builder := range builders {
			i, builder := i, builder
			var isOptional bool
			var useEquivalent bool
			if expPercentage := builder.GetExperimentPercentage(); expPercentage != 0 {
				isOptional = optionalRand.Float32()*100 <= expPercentage
			}
			if equiPercentage := builder.GetEquivalentTo().GetPercentage(); equiPercentage != 0 {
				useEquivalent = equivalentBuilderRand.Float32()*100 <= equiPercentage
			}
			work <- func() error {
				dm := &definitionMaker{
					builder:     builder,
					criticality: builder.GetExperimentPercentage() == 0,
					experiments: experiments,
				}
				if in.RunOptions.GetAvoidCancellingTryjobs() {
					dm.skipStaleCheck = true
				} else {
					dm.skipStaleCheck = builder.GetCancelStale() == cfgpb.Toggle_NO
				}
				switch r, compFail, err := shouldInclude(ctx, in, dm, isOptional, useEquivalent, builder, explicitlyIncluded, allOwners); {
				case err != nil:
					return err
				case compFail != nil:
					computationFailureHolder.Store(compFail)
				case r != skipBuilder:
					definitions[i] = dm.make()
				}
				return nil
			}
		}
	})

	switch failure := computationFailureHolder.Load(); {
	case err != nil:
		return nil, err
	case failure != nil:
		return &ComputationResult{
			ComputationFailure: failure.(ComputationFailure),
		}, nil
	default:
		ret := &ComputationResult{Requirement: &tryjob.Requirement{
			RetryConfig: in.ConfigGroup.GetVerifiers().GetTryjob().GetRetryConfig(),
		}}
		for _, def := range definitions {
			if def != nil {
				ret.Requirement.Definitions = append(ret.Requirement.Definitions, def)
			}
		}
		return ret, nil
	}
}

func handleOverriddenTryjobs(ctx context.Context, in Input) (*ComputationResult, error) {
	override, compFail := parseTryjobDirectives(in.RunOptions.GetOverriddenTryjobs())
	if compFail != nil {
		return &ComputationResult{ComputationFailure: compFail}, nil
	}
	allBuilders := getAllBuilders(in.ConfigGroup.GetVerifiers().GetTryjob().GetBuilders())
	if notDefined := override.Difference(allBuilders); len(notDefined) > 0 {
		return &ComputationResult{
			ComputationFailure: &buildersNotDefined{Builders: notDefined.ToSlice()},
		}, nil
	}

	allOwners := in.allCLOwnersSorted()
	experiments, err := computeEnabledExperiments(ctx, in, allOwners)
	if err != nil {
		return nil, err
	}
	ret := &ComputationResult{
		Requirement: &tryjob.Requirement{
			RetryConfig: in.ConfigGroup.GetVerifiers().GetTryjob().GetRetryConfig(),
		},
	}
	for _, b := range in.ConfigGroup.GetVerifiers().GetTryjob().GetBuilders() {
		if !override.Has(b.GetName()) && !override.Has(b.GetEquivalentTo().GetName()) {
			continue
		}
		switch skip, err := skipByFooters(in, b); {
		case err != nil:
			return nil, err
		case skip:
			continue
		}

		builderName := b.Name
		equivalence := mainOnly
		allowlist := b.GetOwnerWhitelistGroup()
		if equi := b.GetEquivalentTo(); override.Has(equi.GetName()) {
			builderName = equi.GetName()
			equivalence = equivalentOnly
			if equi.GetOwnerWhitelistGroup() != "" {
				allowlist = []string{equi.GetOwnerWhitelistGroup()}
			}
		}
		switch disallowedOwners, err := getDisallowedOwners(ctx, in, allOwners, allowlist...); {
		case err != nil:
			return nil, err
		case len(disallowedOwners) != 0:
			return &ComputationResult{
				ComputationFailure: &unauthorizedIncludedTryjob{
					Users:   disallowedOwners,
					Builder: builderName,
				},
			}, nil
		default:
			dm := &definitionMaker{
				builder:        b,
				criticality:    true,
				equivalence:    equivalence,
				skipStaleCheck: b.GetCancelStale() == cfgpb.Toggle_NO,
				experiments:    experiments,
			}
			if in.RunOptions.GetAvoidCancellingTryjobs() {
				dm.skipStaleCheck = true
			}
			ret.Requirement.Definitions = append(ret.Requirement.Definitions, dm.make())
		}
	}
	return ret, nil
}

type inclusionResult bool

const (
	skipBuilder    inclusionResult = false
	includeBuilder                 = true
)

// shouldInclude decides based on the configuration whether a given builder
// should be skipped in generating the Requirement.
func shouldInclude(ctx context.Context, in Input, dm *definitionMaker, isOptional, useEquivalent bool, b *cfgpb.Verifiers_Tryjob_Builder, incl stringset.Set, owners []string) (inclusionResult, ComputationFailure, error) {
	switch skip, err := skipByFooters(in, b); {
	case err != nil:
		return skipBuilder, nil, err
	case skip:
		return skipBuilder, nil, nil
	}

	if incl.Has(b.Name) {
		switch disallowedOwners, err := canTriggerIncludedBuilder(ctx, in, owners, b.Name, b.GetOwnerWhitelistGroup()); {
		case err != nil:
			return skipBuilder, nil, err
		case len(disallowedOwners) != 0:
			// The requested builder is defined, but the owner is not allowed to
			// include it.
			return skipBuilder, &unauthorizedIncludedTryjob{
				Users:   disallowedOwners,
				Builder: b.Name,
			}, nil
		}
		dm.equivalence = mainOnly
		dm.criticality = true // Explicitly included builder is always critical.
		return includeBuilder, nil, nil
	}

	if b.GetEquivalentTo() != nil && incl.Has(b.GetEquivalentTo().GetName()) {
		if builderName, ownerAllowGroup := b.GetEquivalentTo().GetName(), b.GetEquivalentTo().GetOwnerWhitelistGroup(); ownerAllowGroup != "" {
			switch disallowedOwners, err := canTriggerIncludedBuilder(ctx, in, owners, builderName, []string{ownerAllowGroup}); {
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
		dm.equivalence = equivalentOnly
		dm.criticality = true // explicitly included builder is always critical
		return includeBuilder, nil, nil
	}

	if b.GetExperimentPercentage() != 0 && !isOptional {
		return skipBuilder, nil, nil
	}

	if b.IncludableOnly {
		return skipBuilder, nil, nil
	}

	if !isModeAllowed(in.RunMode, b.ModeAllowlist) {
		return skipBuilder, nil, nil
	}

	// Check for location filter match to decide whether to conditionally skip
	// the builder based on location.
	if len(b.LocationFilters) > 0 {
		matched, err := locationFilterMatch(ctx, b.LocationFilters, in.CLs)
		if !matched || err != nil {
			return skipBuilder, nil, err
		}
	}

	switch allowed, err := isBuilderAllowed(ctx, in, owners, b); {
	case err != nil:
		return skipBuilder, nil, err
	case allowed:
		// Decide whether CV should use main builder or equivalent builder.
		dm.equivalence = mainOnly
		if b.GetEquivalentTo() != nil && !in.RunOptions.GetSkipEquivalentBuilders() {
			dm.equivalence = bothMainAndEquivalent
			switch allowed, err := isEquiBuilderAllowed(ctx, in, owners, b.GetEquivalentTo()); {
			case err != nil:
				return skipBuilder, nil, err
			case allowed:
				if useEquivalent {
					// Invert equivalence: trigger equivalent, but accept reusing original too.
					dm.equivalence = flipMainAndEquivalent
				}
			default:
				// Not allowed to use equivalent.
				dm.equivalence = mainOnly
			}
		}
		return includeBuilder, nil, nil
	case b.GetEquivalentTo() != nil && !in.RunOptions.GetSkipEquivalentBuilders():
		// See if the owners can trigger the equivalent builder instead.
		switch equiAllowed, err := isEquiBuilderAllowed(ctx, in, owners, b.GetEquivalentTo()); {
		case err != nil:
			return skipBuilder, nil, err
		case equiAllowed:
			dm.equivalence = equivalentOnly
			return includeBuilder, nil, err
		default:
			return skipBuilder, nil, err
		}
	default:
		return skipBuilder, nil, nil
	}
}

func calculateExplicitlyIncluded(in Input) (stringset.Set, ComputationFailure) {
	explicitlyIncluded, compFail := parseTryjobDirectives(in.RunOptions.GetIncludedTryjobs())
	if compFail != nil {
		return nil, compFail
	}
	allBuilders := getAllBuilders(in.ConfigGroup.GetVerifiers().GetTryjob().GetBuilders())
	if undefined := explicitlyIncluded.Difference(allBuilders); len(undefined) > 0 {
		return nil, &buildersNotDefined{Builders: undefined.ToSlice()}
	}
	return explicitlyIncluded, nil
}

// getDisallowedOwners checks which of the owner emails given belong
// to none of the given allowLists.
//
// NOTE: If no allowLists are given, the return value defaults to nil. This may
// be unexpected, and has security implications. This behavior is valid in
// regards to checking whether the owners are allowed to use a certain builder,
// if no allowList groups are defined, then the expectation is that the action
// is allowed by default.
func getDisallowedOwners(ctx context.Context, in Input, allOwnerEmails []string, allowLists ...string) ([]string, error) {
	switch {
	case len(allOwnerEmails) == 0:
		panic(fmt.Errorf("cannot check membership of nil user"))
	case len(allowLists) == 0:
		return nil, nil
	}

	gerritHost := in.CLs[0].Detail.GetGerrit().GetHost()
	luciProject := in.CLs[0].Detail.GetLuciProject()
	var disallowed []string
	for _, userEmail := range allOwnerEmails {
		id, err := identity.MakeIdentity(fmt.Sprintf("user:%s", userEmail))
		if err != nil {
			return nil, err
		}
		switch allowed, err := acls.IsMember(ctx, in.GFactory, gerritHost, luciProject, id, allowLists); {
		case err != nil:
			return nil, err
		case !allowed:
			disallowed = append(disallowed, userEmail)
		}
	}
	return disallowed, nil
}

// If a builder is included via `Cq-Include-Trybots` footer, owners must either
// be a member of provided allowlist groups or has direct schedule permission
// to this builder.
func canTriggerIncludedBuilder(ctx context.Context, in Input, clOwners []string, builderName string, allowlistGroups []string) (disallowedOwners []string, err error) {
	disallowed, err := getDisallowedOwners(ctx, in, clOwners, allowlistGroups...)
	switch {
	case err != nil:
		return nil, err
	case len(disallowed) == 0:
		return nil, nil // allowed
	}

	builderID, err := buildbucket.ParseBuilderID(builderName)
	if err != nil {
		return nil, err
	}
	realm := realms.Join(builderID.GetProject(), builderID.GetBucket())
	for _, user := range disallowed {
		id, err := identity.MakeIdentity(fmt.Sprintf("user:%s", user))
		if err != nil {
			return nil, err
		}
		switch allowed, err := auth.GetState(ctx).DB().HasPermission(ctx, id, bbperms.BuildsAdd, realm, nil); {
		case err != nil:
			return nil, err
		case !allowed:
			disallowedOwners = append(disallowedOwners, user)
		}
	}
	return disallowedOwners, nil
}

func isBuilderAllowed(ctx context.Context, in Input, allOwners []string, b *cfgpb.Verifiers_Tryjob_Builder) (bool, error) {
	if len(b.GetOwnerWhitelistGroup()) > 0 {
		switch disallowedOwners, err := getDisallowedOwners(ctx, in, allOwners, b.GetOwnerWhitelistGroup()...); {
		case err != nil:
			return false, err
		case len(disallowedOwners) > 0:
			return false, nil
		}
	}
	return true, nil
}

func isEquiBuilderAllowed(ctx context.Context, in Input, allOwners []string, b *cfgpb.Verifiers_Tryjob_EquivalentBuilder) (bool, error) {
	if b.GetOwnerWhitelistGroup() != "" {
		switch disallowedOwners, err := getDisallowedOwners(ctx, in, allOwners, b.GetOwnerWhitelistGroup()); {
		case err != nil:
			return false, err
		case len(disallowedOwners) > 0:
			return false, nil
		}
	}
	return true, nil
}

func computeEnabledExperiments(ctx context.Context, in Input, allOwners []string) ([]string, error) {
	experiments := in.ConfigGroup.GetTryjobExperiments()
	if len(experiments) == 0 {
		return nil, nil
	}
	ret := make([]string, 0, len(experiments))
	for _, exp := range experiments {
		switch disallowedOwners, err := getDisallowedOwners(ctx, in, allOwners, exp.GetCondition().GetOwnerGroupAllowlist()...); {
		case err != nil:
			return nil, err
		case len(disallowedOwners) == 0:
			ret = append(ret, exp.GetName())
		}
	}
	return ret, nil
}

// makeBuildbucketDefinition converts a builder name to a minimal Definition.
func makeBuildbucketDefinition(host, builderName string) *tryjob.Definition {
	if builderName == "" {
		panic(fmt.Errorf("builderName unexpectedly empty"))
	}
	builderID, err := buildbucket.ParseBuilderID(builderName)
	if err != nil {
		panic(err)
	}
	if host == "" {
		host = chromeinfra.BuildbucketHost
	}
	return &tryjob.Definition{
		Backend: &tryjob.Definition_Buildbucket_{
			Buildbucket: &tryjob.Definition_Buildbucket{
				Host:    host,
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

type equivalentUsage int8

const (
	mainOnly equivalentUsage = iota + 1
	equivalentOnly
	bothMainAndEquivalent
	flipMainAndEquivalent
)

type definitionMaker struct {
	builder        *cfgpb.Verifiers_Tryjob_Builder
	equivalence    equivalentUsage
	criticality    criticality
	skipStaleCheck bool
	experiments    []string
}

func (dm *definitionMaker) make() *tryjob.Definition {
	var definition *tryjob.Definition
	switch dm.equivalence {
	case mainOnly:
		definition = makeBuildbucketDefinition(dm.builder.GetHost(), dm.builder.GetName())
	case equivalentOnly:
		definition = makeBuildbucketDefinition(dm.builder.GetHost(), dm.builder.GetEquivalentTo().GetName())
	case bothMainAndEquivalent:
		definition = makeBuildbucketDefinition(dm.builder.GetHost(), dm.builder.GetName())
		definition.EquivalentTo = makeBuildbucketDefinition(dm.builder.GetHost(), dm.builder.GetEquivalentTo().GetName())
	case flipMainAndEquivalent:
		definition = makeBuildbucketDefinition(dm.builder.GetHost(), dm.builder.GetEquivalentTo().GetName())
		definition.EquivalentTo = makeBuildbucketDefinition(dm.builder.GetHost(), dm.builder.GetName())
	default:
		panic(fmt.Errorf("unknown equivalentUsage(%d)", dm.equivalence))
	}
	definition.DisableReuse = dm.builder.GetDisableReuse()
	definition.Critical = bool(dm.criticality)
	definition.Optional = dm.builder.GetExperimentPercentage() > 0
	definition.ResultVisibility = dm.builder.GetResultVisibility()
	definition.SkipStaleCheck = dm.skipStaleCheck
	definition.Experiments = dm.experiments
	return definition
}

func isModeAllowed(mode run.Mode, allowedModes []string) bool {
	if len(allowedModes) == 0 {
		allowedModes = run.DefaultAllowedModes()
	}
	for _, allowed := range allowedModes {
		if string(mode) == allowed {
			return true
		}
	}
	return false
}

func skipByFooters(in Input, b *cfgpb.Verifiers_Tryjob_Builder) (bool, error) {
	// Handle No-Try and No-Presubmit footer
	// TODO: crbug/40621912 - Move No-Try and No-Presubmit footer to use
	// skip_footers in the config.
	switch ps := isPresubmit(b); {
	case in.RunOptions.GetSkipTryjobs() && !ps:
		return true, nil
	case in.RunOptions.GetSkipPresubmit() && ps:
		return true, nil
	}

	if len(b.GetSkipFooters()) > 0 {
		skipKeyToValueRegexp := make(map[string]*regexp.Regexp, len(b.GetSkipFooters()))
		for _, sf := range b.GetSkipFooters() {
			pattern, err := regexp.Compile(sf.GetValueRegexp())
			if err != nil {
				return false, fmt.Errorf("failed to compile value pattern %q: %w", sf.GetValueRegexp(), err)
			}
			skipKeyToValueRegexp[sf.GetKey()] = pattern
		}

		for _, cl := range in.CLs {
			for _, footer := range cl.Detail.GetMetadata() {
				if valueRegexp, ok := skipKeyToValueRegexp[footer.GetKey()]; ok && valueRegexp.MatchString(footer.GetValue()) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func isPresubmit(builder *cfgpb.Verifiers_Tryjob_Builder) bool {
	// TODO(crbug.com/1292195): Implement a different way of deciding that a
	// builder is presubmit.
	return builder.DisableReuse
}

func getAllBuilders(builders []*cfgpb.Verifiers_Tryjob_Builder) stringset.Set {
	ret := stringset.New(len(builders))
	for _, b := range builders {
		ret.Add(b.Name)
		if b.EquivalentTo != nil {
			ret.Add(b.EquivalentTo.Name)
		}
	}
	return ret
}
