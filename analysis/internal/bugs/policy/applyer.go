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

package policy

import (
	"fmt"
	"sort"
	"strings"

	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	configpb "go.chromium.org/luci/analysis/proto/config"
	"go.chromium.org/luci/common/errors"
)

// Applyer provides methods to apply bug managment policies
// in a manner that is generic to the bug management system being used.
type Applyer struct {
	// policies are the configured bug management policies for the project.
	policiesByDescendingPriority []*configpb.BugManagementPolicy

	// templates are the compiled templates for each bug management policy.
	//
	// Maintained in 1:1 correspondance to the `policiesByDescendingPriority` slice,
	// so policiesByDescendingPriority[i] corresponds to templates[i].
	templates []Template

	// floorPriority is the lowest priority level supported by the
	// bug system. Priorities below this will be rounded up to
	// this floor level.
	// Invariant: not BUGANIZER_PRIORITY_UNSPECIFIED.
	floorPriority configpb.BuganizerPriority
}

// NewApplyer initialises a new Applyer.
func NewApplyer(policies []*configpb.BugManagementPolicy, floorPriority configpb.BuganizerPriority) (Applyer, error) {
	if floorPriority == configpb.BuganizerPriority_BUGANIZER_PRIORITY_UNSPECIFIED {
		panic("floorPriority must be specified")
	}
	policiesByDescendingPriority := sortPoliciesByDescendingPriority(policies)

	templates := make([]Template, 0, len(policiesByDescendingPriority))
	for _, p := range policiesByDescendingPriority {
		template, err := ParseTemplate(p.BugTemplate.CommentTemplate)
		if err != nil {
			return Applyer{}, errors.Annotate(err, "parsing comment template for policy %q", p.Id).Err()
		}
		templates = append(templates, template)
	}

	return Applyer{
		policiesByDescendingPriority: policiesByDescendingPriority,
		templates:                    templates,
		floorPriority:                floorPriority,
	}, nil
}

// applyPriorityFloor returns the maximum of the given priority
// and the priority floor. For example, if the provided priority
// is P4 and the floor is P3, this methods returns P3.
func (p Applyer) applyPriorityFloor(priority configpb.BuganizerPriority) configpb.BuganizerPriority {
	// A lower number indicates a higher priority.
	if p.floorPriority < priority {
		return p.floorPriority
	}
	return priority
}

// RecommendedPriorityAndVerified identifies the priority and verification state
// recommended for a bug with the given set of policies active.
func (p Applyer) RecommendedPriorityAndVerified(activePolicyIDs map[string]struct{}) (priority configpb.BuganizerPriority, verified bool) {
	result := configpb.BuganizerPriority_BUGANIZER_PRIORITY_UNSPECIFIED
	for _, policy := range p.policiesByDescendingPriority {
		_, ok := activePolicyIDs[policy.Id]
		if !ok {
			// Policy not active.
			continue
		}
		// Note that policy.Priority is never UNSPECIFIED, because
		// of config validation.
		priority := p.applyPriorityFloor(policy.Priority)

		// Keep the track of the highest priority we have seen so far.
		// This is the priority with the lowest number, i.e. P0 < P1 < P2 < P3.
		if result == configpb.BuganizerPriority_BUGANIZER_PRIORITY_UNSPECIFIED || priority < result {
			result = priority
		}
	}
	isVerified := result == configpb.BuganizerPriority_BUGANIZER_PRIORITY_UNSPECIFIED
	return result, isVerified
}

type BugOptions struct {
	// The current bug management state.
	State *bugspb.BugManagementState
	// Whether we are managing the priority of the bug.
	IsManagingPriority bool
	// The current priority of the bug.
	ExistingPriority configpb.BuganizerPriority
	// Whether the bug is currently verified.
	ExistingVerified bool
}

// NeedsPriorityOrVerifiedUpdate returns whether a bug needs to have its
// priority or verified status updated, based on the current active policies.
func (p Applyer) NeedsPriorityOrVerifiedUpdate(opts BugOptions) bool {
	recommendedPriority, recommendedVerified := p.RecommendedPriorityAndVerified(activePolicies(opts.State))

	// Priority updates are only considered if:
	// - We are managing the bug priority
	// - The bug is not verified / transitioning to verified.
	needsPriorityUpdate := opts.IsManagingPriority && !recommendedVerified && recommendedPriority != opts.ExistingPriority

	needsVerifiedUpdate := recommendedVerified != opts.ExistingVerified
	return needsPriorityUpdate || needsVerifiedUpdate
}

type BugChange struct {
	// The human-readable justification of the change.
	// This will be blank if no change is proposed.
	Justification bugs.Commentary

	// Whether the bug priority should be updated.
	UpdatePriority bool
	// The new bug priority.
	Priority configpb.BuganizerPriority

	// Whether the bug verified status should be changed.
	UpdateVerified bool
	// Whether the bug should be verified now.
	ShouldBeVerified bool
}

// PreparePriorityAndVerifiedChange generates the changes to apply
// to a bug's priority and verified fields, based on the the
// current active policies.
//
// A human readable explanation of the changes to include in a comment
// is also returned.
func (p Applyer) PreparePriorityAndVerifiedChange(opts BugOptions, uiBaseURL string) (BugChange, error) {
	currentActive := activePolicies(opts.State)
	changes := lastPolicyActivationChanges(opts.State)
	previousActive := previouslyActivePolicies(opts.State)

	recommendedPriority, recommendedVerified := p.RecommendedPriorityAndVerified(currentActive)
	previousRecommendedPriority, previousRecommendedVerified := p.RecommendedPriorityAndVerified(previousActive)

	isChangingPriority := opts.IsManagingPriority && !recommendedVerified && recommendedPriority != opts.ExistingPriority
	isChangingVerified := recommendedVerified != opts.ExistingVerified

	if !isChangingPriority && !isChangingVerified {
		// No change is required.
		return BugChange{
			Justification:    bugs.Commentary{},
			UpdatePriority:   false,
			Priority:         recommendedPriority,
			UpdateVerified:   false,
			ShouldBeVerified: recommendedVerified,
		}, nil
	}

	// We generalise the notion of priority here to be over both bug
	// priority and verified status.
	// The priority ranking then is as follows:
	// - (Verified, Any bug priority) [lowest priority level]
	// - (Not verified, P4)
	// - (Not verified, P3)
	// ..
	// - (Not verified, P0)           [highest priority level]
	//
	// For example, going from (Verified, P1) to
	// (Not verified, P2) is a priority increase, as is
	// going from (Not verified, P2) to (Not verified, P1).
	isPriorityIncreasing := (isChangingPriority && !opts.ExistingVerified && recommendedPriority < opts.ExistingPriority) || (isChangingVerified && !recommendedVerified)
	isPriorityDecreasing := (isChangingPriority && !opts.ExistingVerified && recommendedPriority > opts.ExistingPriority) || (isChangingVerified && recommendedVerified)

	if isPriorityIncreasing == isPriorityDecreasing {
		// This should never happen. Exactly one of
		// isPriorityIncreasing and isPriorityDecreasing
		// should be true.
		return BugChange{}, errors.New("logic error: the priority has changed, but it cannot be determined if the priority is increasing or decreasing")
	}

	// Builder for comment body.
	var body strings.Builder

	// If the previous recommendations match the current bug state, then the changes in policy activation explains updates to the bug.
	if (!opts.IsManagingPriority || previousRecommendedVerified || !previousRecommendedVerified && previousRecommendedPriority == opts.ExistingPriority) &&
		(previousRecommendedVerified == opts.ExistingVerified) {
		// We want to show policy activations and deactivations that are:
		// - Consistent with the direction of the policy change (e.g. if we are dropping the
		//   policy priority, we only care about policies which deactivated).
		// - Relevant to the change (e.g. if we dropped the priority from P1 to P2, we only
		//   want the P1 problems that deactivated, not P2 or P3 problems that deactivated).
		explanationFound := false
		if isPriorityIncreasing {
			body.WriteString("Because the following problem(s) have started:\n")
			for _, policy := range p.policiesByDescendingPriority {
				_, isActivating := changes.activatedPolicyIDs[policy.Id]
				priority := p.applyPriorityFloor(policy.Priority)
				// The policy is activating, and
				// - We are changing the priority, and the priority of the policy is higher than the existing priority OR
				// - We are recommending unverification of a bug that was previously verified.
				if isActivating && ((isChangingPriority && priority < opts.ExistingPriority) || isChangingVerified && !recommendedVerified) {
					body.WriteString(fmt.Sprintf("- %s (%s)\n", policy.HumanReadableName, priority))
					explanationFound = true
				}
			}
		} else {
			body.WriteString("Because the following problem(s) have stopped:\n")
			for _, policy := range p.policiesByDescendingPriority {
				_, isDeactivating := changes.deactivatedPolicyIDs[policy.Id]
				priority := p.applyPriorityFloor(policy.Priority)
				// The policy is deactivating, and
				// - We are changing the priority, and the priority of the policy is higher than the priority we are recommending now OR
				// - We are recommending verification of a bug that was previously not verified.
				if isDeactivating && ((isChangingPriority && priority < recommendedPriority) || isChangingVerified && recommendedVerified) {
					body.WriteString(fmt.Sprintf("- %s (%s)\n", policy.HumanReadableName, priority))
					explanationFound = true
				}
			}
		}
		if !explanationFound {
			// This should never happen. If the bug's priority/verified status is consistent
			// with the previous bug managment state, then the changes in that state should
			// explain the recommendation.
			return BugChange{}, errors.New("logic error: no explanation could be found for the priority change")
		}

		if isChangingPriority && isChangingVerified {
			// This case only happens when we are re-opening a bug to a new priority.
			// We never verify a bug and drop its priority at the same time.
			body.WriteString(fmt.Sprintf("The bug has been re-opened as %s.", recommendedPriority))
		} else if isChangingVerified {
			if recommendedVerified {
				body.WriteString("The bug has been verified.")
			} else {
				body.WriteString("The bug has been re-opened.")
			}
		} else if isChangingPriority {
			if recommendedPriority < opts.ExistingPriority {
				body.WriteString(fmt.Sprintf("The bug priority has been increased from %s to %s.", opts.ExistingPriority, recommendedPriority))
			} else {
				body.WriteString(fmt.Sprintf("The bug priority has been decreased from %s to %s.", opts.ExistingPriority, recommendedPriority))
			}
		} else {
			// This code should never be reached.
			return BugChange{}, errors.New("logic error: no priority/verified change being made in a section of code expecting one")
		}
	} else {

		// Otherwise, the recent changes to active policies do not explain the change in priority / verification.
		// We should justify the bug priority from first principles, based on the policies which are active now.

		if recommendedVerified {
			if isChangingPriority {
				// This code should never be reached.
				return BugChange{}, errors.New("logic error: priority change being recommended at some time as verification is recommended")
			}
			if !isChangingVerified {
				// This code should never be reached, as we should have exited early above.
				return BugChange{}, errors.New("logic error: no verified change being made in a section of code expecting one")
			}
			// We know !isChangingPriority && isChangingVerified.
			body.WriteString("Because all problems have stopped, the bug has been verified.")
		} else {
			// We are not recommending verification, so some (non-empty) set of problems must be active.
			body.WriteString("Because the following problem(s) are active:\n")
			for _, policy := range p.policiesByDescendingPriority {
				_, isActive := currentActive[policy.Id]
				if isActive {
					priority := p.applyPriorityFloor(policy.Priority)
					body.WriteString(fmt.Sprintf("- %s (%s)\n", policy.HumanReadableName, priority))
				}
			}

			body.WriteString("\n")
			if isChangingPriority && isChangingVerified {
				body.WriteString(fmt.Sprintf("The bug has been opened and set to %s.", recommendedPriority))
			} else if isChangingVerified {
				if recommendedVerified {
					body.WriteString("The bug has been verified.")
				} else {
					body.WriteString("The bug has been opened.")
				}
			} else if isChangingPriority {
				body.WriteString(fmt.Sprintf("The bug priority has been set to %s.", recommendedPriority))
			} else {
				// This code should never be reached, as we should have exited early above.
				return BugChange{}, errors.New("logic error: no priority/verified change being made in a section of code expecting one")
			}
		}
	}

	var footers []string
	if isChangingPriority {
		footers = append(footers, fmt.Sprintf("Why priority is updated: %s", PriorityUpdatedHelpURL(uiBaseURL)))
	}
	if isChangingVerified {
		if recommendedVerified {
			footers = append(footers, fmt.Sprintf("Why issues are verified: %s", BugVerifiedHelpURL(uiBaseURL)))
		} else {
			footers = append(footers, fmt.Sprintf("Why issues are re-opened: %s", BugReopenedHelpURL(uiBaseURL)))
		}
	}

	return BugChange{
		Justification: bugs.Commentary{
			Bodies:  []string{body.String()},
			Footers: footers,
		},
		UpdatePriority:   isChangingPriority,
		Priority:         recommendedPriority,
		UpdateVerified:   isChangingVerified,
		ShouldBeVerified: recommendedVerified,
	}, nil
}

// SortPolicyIDsByPriorityDescending sorts policy IDs in descending
// priority order (i.e. P0 policies first, then P1, then P2, ...).
// Where multiple policies have the same priority, they are sorted by
// policy ID.
// Only policies which are configured are returned.
func (p Applyer) SortPolicyIDsByPriorityDescending(policyIDs map[string]struct{}) []string {
	var result []string
	for _, policy := range p.policiesByDescendingPriority {
		if _, ok := policyIDs[policy.Id]; ok {
			result = append(result, policy.Id)
		}
	}
	return result
}

// sortPolicies sorts policies in descending priority order. Where
// multiple policies have the same priority, they are sorted by
// policy ID.
func sortPoliciesByDescendingPriority(policies []*configpb.BugManagementPolicy) []*configpb.BugManagementPolicy {
	// Sort policies by priority, then ID.
	var sortedPolicies []*configpb.BugManagementPolicy
	sortedPolicies = append(sortedPolicies, policies...)
	sort.Slice(sortedPolicies, func(i, j int) bool {
		if sortedPolicies[i].Priority != sortedPolicies[j].Priority {
			return sortedPolicies[i].Priority < sortedPolicies[j].Priority
		}
		return sortedPolicies[i].Id < sortedPolicies[j].Id
	})
	return sortedPolicies
}

func (p Applyer) problemsDescription(activatedPolicyIDs map[string]struct{}) string {

	var policyHumanNames []string
	for _, p := range p.policiesByDescendingPriority {
		if _, isActive := activatedPolicyIDs[p.Id]; isActive {
			policyHumanNames = append(policyHumanNames, p.HumanReadableName)
		}
	}

	var result strings.Builder
	result.WriteString("These test failures are causing problem(s) which require your attention, including:\n")
	for _, policyName := range policyHumanNames {
		result.WriteString(fmt.Sprintf("- %s\n", policyName))
	}
	return result.String()
}

// NewIssueDescription returns the issue description for a new bug.
// uiBaseURL is the URL of the UI base, without trailing slash, e.g. "https://luci-analysis.appspot.com".
func (p Applyer) NewIssueDescription(description *clustering.ClusterDescription, activatedPolicyIDs map[string]struct{}, uiBaseURL, ruleURL string) string {
	var problemDescription strings.Builder
	problemDescription.WriteString(p.problemsDescription(activatedPolicyIDs))
	if ruleURL != "" {
		problemDescription.WriteString(fmt.Sprintf("\nSee current problems, failure examples and more in LUCI Analysis at: %s", ruleURL))
	}

	bodies := []string{
		description.Description,
		problemDescription.String(),
	}

	footers := []string{
		fmt.Sprintf("How to action this bug: %s", BugFiledHelpURL(uiBaseURL)),
		fmt.Sprintf("Provide feedback: %s", FeedbackURL(uiBaseURL)),
		fmt.Sprintf("Was this bug filed in the wrong component? See: %s", ComponentSelectionHelpURL(uiBaseURL)),
	}
	return bugs.Commentary{
		Bodies:  bodies,
		Footers: footers,
	}.ToComment()
}

// PolicyActivatedComment returns a comment used to notify a bug that a policy
// has activated on a bug for the first time.
func (p Applyer) PolicyActivatedComment(policyID, uiBaseURL string, input TemplateInput) (string, error) {
	var template *Template
	for i, policy := range p.policiesByDescendingPriority {
		if policy.Id == policyID {
			template = &p.templates[i]
			break
		}
	}
	if template == nil {
		return "", errors.Reason("configuration for policy %q not found", policyID).Err()
	}
	templatedContent, err := template.Execute(input)
	if err != nil {
		return "", errors.Annotate(err, "execute").Err()
	}
	if templatedContent == "" {
		return "", nil
	}
	commentary := bugs.Commentary{
		Bodies:  []string{templatedContent},
		Footers: []string{fmt.Sprintf("Why LUCI Analysis posted this comment: %s (Policy ID: %s)", PolicyActivatedHelpURL(uiBaseURL), policyID)},
	}

	return commentary.ToComment(), nil
}

func ManualPriorityUpdateCommentary() bugs.Commentary {
	c := bugs.Commentary{
		Bodies: []string{"The bug priority has been manually set. To re-enable automatic priority updates by LUCI Analysis, enable the update priority flag on the rule."},
	}
	return c
}
