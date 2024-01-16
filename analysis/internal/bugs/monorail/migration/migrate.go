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

// Package migration handles migration rules from monorail to buganizer.
package migration

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/bugs/monorail"
	migrationpb "go.chromium.org/luci/analysis/internal/bugs/monorail/migration/proto"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/pbutil"
)

// The maximum amount of parallelism to use.
const workers = 10

type migrationServer struct{}

func NewMonorailMigrationServer() migrationpb.MonorailMigrationServer {
	return &migrationpb.DecoratedMonorailMigration{
		Prelude:  checkAllowedAdminPrelude,
		Service:  &migrationServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// MigrateProject updates failure association rules in a LUCI Project to
// use buganizer bugs instead of monorail bugs, after a buganizer migration.
//
// The mapping of monorail bug to buganizer bug is fetched from
// monorail using the GetIssue RPC (MigratedId field).
//
// The MaxRules request parameter defines the maximum number of rules to
// migrate in one go; this can be used to validate migration is working as
// expected before continuing.
func (*migrationServer) MigrateProject(ctx context.Context, req *migrationpb.MigrateProjectRequest) (*migrationpb.MigrateProjectResponse, error) {
	if err := validateMigrateProjectRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}
	cfg, err := config.Get(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "get config").Err()
	}

	monorailClient, err := monorail.NewClient(ctx, cfg.MonorailHostname)
	if err != nil {
		return nil, errors.Annotate(err, "create monorail client").Err()
	}

	rs, err := rules.ReadWithMonorailForProject(span.Single(ctx), req.Project)
	if err != nil {
		return nil, errors.Annotate(err, "read rules requiring migration").Err()
	}

	// Set a shorter timeout for the body of this RPC so we have
	// a chance to return our results before the hard RPC timeout.
	ctx, cancel := bugs.Shorten(ctx, time.Second*5)
	defer cancel()

	var mtx sync.Mutex
	var results []*migrationpb.MigrateProjectResponse_RuleResult
	var notStarted int

	err = parallel.WorkPool(workers, func(c chan<- func() error) {
		for i, r := range rs {
			if int64(i) >= req.MaxRules {
				break
			}
			// Assign loop value to local variable to capture value
			// in function closure.
			rule := r
			c <- func() error {
				if ctx.Err() != nil {
					// Context timeout/cancelled.
					notStarted++
					return nil
				}

				result := migrateRule(ctx, rule, monorailClient)
				mtx.Lock()
				defer mtx.Unlock()
				results = append(results, result)
				return nil
			}
		}
	})
	if err != nil {
		return nil, errors.Annotate(err, "running workers").Err()
	}

	return &migrationpb.MigrateProjectResponse{
		RulesNotStarted: int64(notStarted),
		RuleResults:     results,
	}, nil
}

func validateMigrateProjectRequest(req *migrationpb.MigrateProjectRequest) error {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if req.MaxRules <= 0 {
		return errors.Reason("max_rules must be a postive integer").Err()
	}
	return nil
}

func migrateRule(ctx context.Context, rule *rules.Entry, monorailClient *monorail.Client) *migrationpb.MigrateProjectResponse_RuleResult {
	result := &migrationpb.MigrateProjectResponse_RuleResult{}
	result.RuleId = rule.RuleID
	result.MonorailBugId = rule.BugID.ID

	// Get the monorail issue to find the buganizer bug it has been migrated to.
	issueName, err := monorail.ToMonorailIssueName(rule.BugID.ID)
	if err != nil {
		result.Error = errors.Annotate(err, "prepare monorail issue name").Err().Error()
		return result
	}

	issue, err := monorailClient.GetIssue(ctx, issueName)
	if err != nil {
		result.Error = errors.Annotate(err, "fetch monorail issue").Err().Error()
		return result
	}

	if issue.MigratedId == "" {
		result.Error = "monorail did not return the Buganizer bug this bug was migrated to"
		return result
	}

	result.BuganizerBugId = issue.MigratedId

	// Update the rule to point to the buganizer bug.
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		r, err := rules.Read(ctx, rule.Project, rule.RuleID)
		if err != nil {
			return err
		}
		if r.LastUpdateTime != rule.LastUpdateTime {
			return errors.Reason("race detected: rule updated since it was last read").Err()
		}

		r.BugID = bugs.BugID{
			System: "buganizer",
			ID:     issue.MigratedId,
		}

		m, err := rules.Update(r, rules.UpdateOptions{
			IsAuditableUpdate: true,
		}, rules.LUCIAnalysisSystem)
		if err != nil {
			return errors.Annotate(err, "preparing rule update").Err()
		}
		span.BufferWrite(ctx, m)
		return nil
	})
	if err != nil {
		result.Error = errors.Annotate(err, "updating rule").Err().Error()
	}
	return result
}

// invalidArgumentError annotates err as having an invalid argument.
// The error message is shared with the requester as is.
//
// Note that this differs from FailedPrecondition. It indicates arguments
// that are problematic regardless of the state of the system
// (e.g., a malformed file name).
func invalidArgumentError(err error) error {
	return appstatus.Attachf(err, codes.InvalidArgument, "%s", err)
}
