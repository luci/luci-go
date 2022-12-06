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

// Package monorail contains monorail-specific logic for
// creating and updating bugs.
package monorail

import (
	"context"
	"fmt"
	"regexp"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/analysis/internal/bugs"
	mpb "go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
	configpb "go.chromium.org/luci/analysis/proto/config"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// monorailRe matches monorail issue names, like
// "monorail/{monorail_project}/{numeric_id}".
var monorailRe = regexp.MustCompile(`^projects/([a-z0-9\-_]+)/issues/([0-9]+)$`)

// componentRE matches valid full monorail component names.
var componentRE = regexp.MustCompile(`^[a-zA-Z]([-_]?[a-zA-Z0-9])+(\>[a-zA-Z]([-_]?[a-zA-Z0-9])+)*$`)

var textPBMultiline = prototext.MarshalOptions{
	Multiline: true,
}

// monorailPageSize is the maximum number of issues that can be requested
// through GetIssues at a time. This limit is set by monorail.
const monorailPageSize = 100

// BugManager controls the creation of, and updates to, monorail bugs
// for clusters.
type BugManager struct {
	client *Client
	// The GAE APP ID, e.g. "luci-analysis".
	appID string
	// The LUCI Project.
	project string
	// The snapshot of configuration to use for the project.
	projectCfg *configpb.ProjectConfig
	// The generator used to generate updates to monorail bugs.
	generator *Generator
	// Simulate, if set, tells BugManager not to make mutating changes
	// to monorail but only log the changes it would make. Must be set
	// when running locally as RPCs made from developer systems will
	// appear as that user, which breaks the detection of user-made
	// priority changes vs system-made priority changes.
	Simulate bool
}

// NewBugManager initialises a new bug manager, using the specified
// monorail client.
func NewBugManager(client *Client, appID, project string, projectCfg *configpb.ProjectConfig) (*BugManager, error) {
	g, err := NewGenerator(appID, project, projectCfg)
	if err != nil {
		return nil, errors.Annotate(err, "create issue generator").Err()
	}
	return &BugManager{
		client:     client,
		appID:      appID,
		project:    project,
		projectCfg: projectCfg,
		generator:  g,
		Simulate:   false,
	}, nil
}

// Create creates a new bug for the given request, returning its name, or
// any encountered error.
func (m *BugManager) Create(ctx context.Context, request *bugs.CreateRequest) (string, error) {
	components := request.MonorailComponents
	components, err := m.filterToValidComponents(ctx, components)
	if err != nil {
		return "", errors.Annotate(err, "validate components").Err()
	}

	makeReq := m.generator.PrepareNew(request.Impact, request.Description, components)
	var bugName string
	if m.Simulate {
		logging.Debugf(ctx, "Would create Monorail issue: %s", textPBMultiline.Format(makeReq))
		bugName = fmt.Sprintf("%s/12345678", m.projectCfg.Monorail.Project)
	} else {
		// Save the issue in Monorail.
		issue, err := m.client.MakeIssue(ctx, makeReq)
		if err != nil {
			return "", errors.Annotate(err, "create issue in monorail").Err()
		}
		bugName, err = fromMonorailIssueName(issue.Name)
		if err != nil {
			return "", errors.Annotate(err, "parsing monorail issue name").Err()
		}
	}

	modifyReq, err := m.generator.PrepareLinkComment(bugName)
	if err != nil {
		return "", errors.Annotate(err, "prepare link comment").Err()
	}
	if m.Simulate {
		logging.Debugf(ctx, "Would update Monorail issue: %s", textPBMultiline.Format(modifyReq))
		return "", bugs.ErrCreateSimulated
	}
	if err := m.client.ModifyIssues(ctx, modifyReq); err != nil {
		return "", errors.Annotate(err, "update issue").Err()
	}
	bugs.BugsCreatedCounter.Add(ctx, 1, m.project, "monorail")
	return bugName, nil
}

// filterToValidComponents limits the given list of components to only those
// components which exist in monorail, and are active.
func (m *BugManager) filterToValidComponents(ctx context.Context, components []string) ([]string, error) {
	var result []string
	for _, c := range components {
		if !componentRE.MatchString(c) {
			continue
		}
		existsAndActive, err := m.client.GetComponentExistsAndActive(ctx, m.projectCfg.Monorail.Project, c)
		if err != nil {
			return nil, err
		}
		if !existsAndActive {
			continue
		}
		result = append(result, c)
	}
	return result, nil
}

// Update updates the specified list of bugs.
func (m *BugManager) Update(ctx context.Context, request []bugs.BugUpdateRequest) ([]bugs.BugUpdateResponse, error) {
	// Fetch issues for bugs to update.
	issues, err := m.fetchIssues(ctx, request)
	if err != nil {
		return nil, err
	}

	var responses []bugs.BugUpdateResponse
	for i, req := range request {
		issue := issues[i]
		if issue == nil {
			// The bug does not exist, or is in a different monorail project
			// to the monorail project configured for this project. Take
			// no action.
			responses = append(responses, bugs.BugUpdateResponse{
				IsDuplicate:   false,
				ShouldArchive: false,
			})
			logging.Warningf(ctx, "Monorail issue %s not found, skipping.", req.Bug.ID)
			continue
		}

		isDuplicate := issue.Status.Status == DuplicateStatus
		shouldArchive := shouldArchiveRule(ctx, issue, req.IsManagingBug)
		if !isDuplicate && !shouldArchive && req.IsManagingBug && req.Impact != nil {
			if m.generator.NeedsUpdate(req.Impact, issue) {
				comments, err := m.client.ListComments(ctx, issue.Name)
				if err != nil {
					return nil, err
				}
				updateReq := m.generator.MakeUpdate(req.Impact, issue, comments)
				if m.Simulate {
					logging.Debugf(ctx, "Would update Monorail issue: %s", textPBMultiline.Format(updateReq))
				} else {
					if err := m.client.ModifyIssues(ctx, updateReq); err != nil {
						return nil, errors.Annotate(err, "failed to update monorail issue %s", req.Bug.ID).Err()
					}
					bugs.BugsUpdatedCounter.Add(ctx, 1, m.project, "monorail")
				}
			}
		}
		responses = append(responses, bugs.BugUpdateResponse{
			IsDuplicate:   isDuplicate,
			ShouldArchive: shouldArchive && !isDuplicate,
		})
	}
	return responses, nil
}

// shouldArchiveRule determines if the rule managing the given issue should
// be archived.
func shouldArchiveRule(ctx context.Context, issue *mpb.Issue, isManaging bool) bool {
	// If the bug is set to a status like "Archived", immediately archive
	// the rule as well. We should not re-open such a bug.
	if _, ok := ArchivedStatuses[issue.Status.Status]; ok {
		return true
	}
	now := clock.Now(ctx)
	if isManaging {
		// If LUCI Analysis is managing the bug,
		// more than 30 days since the issue was verified.
		return issue.Status.Status == VerifiedStatus &&
			now.Sub(issue.StatusModifyTime.AsTime()).Hours() >= 30*24
	} else {
		// If the user is managing the bug,
		// more than 30 days since the issue was closed.
		_, ok := ClosedStatuses[issue.Status.Status]
		return ok &&
			now.Sub(issue.StatusModifyTime.AsTime()).Hours() >= 30*24
	}
}

// GetMergedInto reads the bug (if any) the given bug was merged into.
// If the given bug is not merged into another bug, this returns nil.
func (m *BugManager) GetMergedInto(ctx context.Context, bug bugs.BugID) (*bugs.BugID, error) {
	if bug.System != bugs.MonorailSystem {
		// Indicates an implementation error with the caller.
		panic("monorail bug manager can only deal with monorail bugs")
	}
	name, err := toMonorailIssueName(bug.ID)
	if err != nil {
		return nil, err
	}
	issue, err := m.client.GetIssue(ctx, name)
	if err != nil {
		return nil, err
	}
	result, err := mergedIntoBug(issue)
	if err != nil {
		return nil, errors.Annotate(err, "resolving canoncial merged into bug").Err()
	}
	return result, nil
}

// Unduplicate updates the given bug to no longer be marked as duplicating
// another bug, posting the given message on the bug.
func (m *BugManager) UpdateDuplicateSource(ctx context.Context, request bugs.UpdateDuplicateSourceRequest) error {
	if request.Bug.System != bugs.MonorailSystem {
		// Indicates an implementation error with the caller.
		panic("monorail bug manager can only deal with monorail bugs")
	}
	req, err := m.generator.UpdateDuplicateSource(request.Bug.ID, request.ErrorMessage, request.DestinationRuleID)
	if err != nil {
		return errors.Annotate(err, "mark issue as available").Err()
	}
	if m.Simulate {
		logging.Debugf(ctx, "Would update Monorail issue: %s", textPBMultiline.Format(req))
	} else {
		if err := m.client.ModifyIssues(ctx, req); err != nil {
			return errors.Annotate(err, "failed to update duplicate source monorail issue %s", request.Bug.ID).Err()
		}
	}
	return nil
}

func (m *BugManager) UpdateDuplicateDestination(ctx context.Context, destinationBug bugs.BugID) error {
	if destinationBug.System != bugs.MonorailSystem {
		// Indicates an implementation error with the caller.
		panic("monorail bug manager can only deal with monorail bugs")
	}
	req, err := m.generator.UpdateDuplicateDestination(destinationBug.ID)
	if err != nil {
		return errors.Annotate(err, "mark issue as available").Err()
	}
	if m.Simulate {
		logging.Debugf(ctx, "Would update Monorail issue: %s", textPBMultiline.Format(req))
	} else {
		if err := m.client.ModifyIssues(ctx, req); err != nil {
			return errors.Annotate(err, "failed to update duplicate destination monorail issue %s", destinationBug.ID).Err()
		}
	}
	return nil
}

var buganizerExtRefRe = regexp.MustCompile(`^b/([1-9][0-9]{0,16})$`)

// mergedIntoBug determines if the given bug is a duplicate of another
// bug, and if so, what the identity of that bug is.
func mergedIntoBug(issue *mpb.Issue) (*bugs.BugID, error) {
	if issue.Status.Status == DuplicateStatus &&
		issue.MergedIntoIssueRef != nil {
		if issue.MergedIntoIssueRef.Issue != "" {
			name, err := fromMonorailIssueName(issue.MergedIntoIssueRef.Issue)
			if err != nil {
				// This should not happen unless monorail or the
				// implementation here is broken.
				return nil, err
			}
			return &bugs.BugID{
				System: bugs.MonorailSystem,
				ID:     name,
			}, nil
		}
		matches := buganizerExtRefRe.FindStringSubmatch(issue.MergedIntoIssueRef.ExtIdentifier)
		if matches == nil {
			// A non-buganizer external issue tracker was used. This is not
			// supported by us, treat the issue as not duplicate of something
			// else and let auto-updating kick the bug out of duplicate state
			// if there is still impact. The user should manually resolve the
			// situation.
			return nil, fmt.Errorf("unsupported non-monorail non-buganizer bug reference: %s", issue.MergedIntoIssueRef.ExtIdentifier)
		}
		return &bugs.BugID{
			System: bugs.BuganizerSystem,
			ID:     matches[1],
		}, nil
	}
	return nil, nil
}

// fetchIssues fetches monorail issues using the internal bug names like
// {monorail_project}/{issue_id}. Issues in the result will be in 1:1
// correspondence (by index) to the request. If an issue does not exist,
// or is from a monorail project other than the one configured for this
// LUCI project, the corresponding item in the response will be nil.
func (m *BugManager) fetchIssues(ctx context.Context, request []bugs.BugUpdateRequest) ([]*mpb.Issue, error) {
	// Calculate the number of requests required, rounding up
	// to the nearest page.
	pages := (len(request) + (monorailPageSize - 1)) / monorailPageSize

	response := make([]*mpb.Issue, 0, len(request))
	for i := 0; i < pages; i++ {
		// Divide names into pages of monorailPageSize.
		pageEnd := (i + 1) * monorailPageSize
		if pageEnd > len(request) {
			pageEnd = len(request)
		}
		requestPage := request[i*monorailPageSize : pageEnd]

		var ids []string
		for _, requestItem := range requestPage {
			if requestItem.Bug.System != bugs.MonorailSystem {
				// Indicates an implementation error with the caller.
				panic("monorail bug manager can only deal with monorail bugs")
			}
			monorailProject, id, err := toMonorailProjectAndID(requestItem.Bug.ID)
			if err != nil {
				return nil, err
			}
			if monorailProject != m.projectCfg.Monorail.Project {
				// Only query bugs from the same monorail project as what has
				// been configured for the LUCI Project.
				continue
			}
			ids = append(ids, id)
		}

		// Guarantees result array in 1:1 correspondence to requested IDs.
		issues, err := m.client.BatchGetIssues(ctx, m.projectCfg.Monorail.Project, ids)
		if err != nil {
			return nil, err
		}
		response = append(response, issues...)
	}
	return response, nil
}

// toMonorailProjectAndID splits an internal bug name like
// "{monorail_project}/{numeric_id}" to the monorail project and
// numeric ID.
func toMonorailProjectAndID(bug string) (project, id string, err error) {
	parts := bugs.MonorailBugIDRe.FindStringSubmatch(bug)
	if parts == nil {
		return "", "", fmt.Errorf("invalid bug %q", bug)
	}
	return parts[1], parts[2], nil
}

// toMonorailIssueName converts an internal bug name like
// "{monorail_project}/{numeric_id}" to a monorail issue name like
// "projects/{project}/issues/{numeric_id}".
func toMonorailIssueName(bug string) (string, error) {
	parts := bugs.MonorailBugIDRe.FindStringSubmatch(bug)
	if parts == nil {
		return "", fmt.Errorf("invalid bug %q", bug)
	}
	return fmt.Sprintf("projects/%s/issues/%s", parts[1], parts[2]), nil
}

// fromMonorailIssueName converts a monorail issue name like
// "projects/{project}/issues/{numeric_id}" to an internal bug name like
// "{monorail_project}/{numeric_id}".
func fromMonorailIssueName(name string) (string, error) {
	parts := monorailRe.FindStringSubmatch(name)
	if parts == nil {
		return "", fmt.Errorf("invalid monorail issue name %q", name)
	}
	return fmt.Sprintf("%s/%s", parts[1], parts[2]), nil
}
