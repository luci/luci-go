// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package quota

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/server/auth"
	srvquota "go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/quota/quotapb"
	"google.golang.org/protobuf/types/known/durationpb"
)

var qinit sync.Once
var qapp SrvQuota

const (
	// Resource types that the quota can use.
	runResource    = "runs"
	tryjobResource = "tryjobs"

	defaultPolicy = "default"

	// Default lifetime of a quota account.
	accountLifeTimeSeconds = 60 * 60 * 24 * 3 // 3 days
)

// Manager manages the quota accounts for CV users.
type Manager struct {
	qapp SrvQuota
}

// SrvQuota manages quota
type SrvQuota interface {
	LoadPoliciesAuto(ctx context.Context, realm string, cfg *quotapb.PolicyConfig) (*quotapb.PolicyConfigID, error)
}

// DebitRunQuota debits the run quota from a given user's account.
func (qm *Manager) DebitRunQuota(ctx context.Context) (*quotapb.OpResult, error) {
	return nil, nil
}

// CreditRunQuota credits the run quota into a given user's account.
func (qm *Manager) CreditRunQuota(ctx context.Context) (*quotapb.OpResult, error) {
	return nil, nil
}

// DebitTryjobQuota debits the tryjob quota from a given user's account.
func (qm *Manager) DebitTryjobQuota(ctx context.Context) (*quotapb.OpResult, error) {
	return nil, nil
}

// CreditTryjobQuota credits the tryjob quota into a given user's account.
func (qm *Manager) CreditTryjobQuota(ctx context.Context) (*quotapb.OpResult, error) {
	return nil, nil
}

// findRunPolicy returns the PolicyID for the given run state.
func (qm *Manager) findRunPolicy(ctx context.Context, rs *state.RunState) (*quotapb.PolicyID, error) {
	project := rs.Run.ID.LUCIProject()
	cfgGroup, err := prjcfg.GetConfigGroup(ctx, project, rs.Run.ConfigGroupID)
	if err != nil {
		return nil, err
	}

	config := cfgGroup.Content
	if config == nil {
		return nil, fmt.Errorf("cannot find cfgGroup content")
	}

	// loadPolicies loads cfgGroup into server/quota. This serves two purposes:
	// 1. If policies for this cfgGroup is not loaded already, this loads it.
	// 2. If they are loaded already, server/quota immediately returns &quotapb.PolicyConfigID which we use for our Ops.
	policyConfigID, err := qm.loadPolicies(ctx, project, []*prjcfg.ConfigGroup{cfgGroup})
	if err != nil {
		return nil, errors.Annotate(err, "loadPolicies").Err()
	}

	user := rs.Run.Owner
	for _, userLimit := range config.GetUserLimits() {
		runLimit := userLimit.GetRun()
		if runLimit == nil {
			continue
		}

		var groups []string
		for _, principal := range userLimit.GetPrincipals() {
			switch parts := strings.SplitN(principal, ":", 2); {
			case len(parts) != 2:
				// Each entry can be either an identity string "user:<email>" or a LUCI group reference "group:<name>".
				return nil, fmt.Errorf("improper format for principal: %s", principal)
			case parts[0] == "user" && parts[1] == user.Email():
				return &quotapb.PolicyID{
					Config: policyConfigID,
					Key:    runPolicyKey(config.GetName(), userLimit.GetName()),
				}, nil
			case parts[0] == "group":
				groups = append(groups, parts[1])
			}
		}

		if len(groups) == 0 {
			continue
		}

		switch result, err := auth.GetState(ctx).DB().IsMember(ctx, user, groups); {
		case err != nil:
			return nil, err
		case result:
			return &quotapb.PolicyID{
				Config: policyConfigID,
				Key:    runPolicyKey(config.GetName(), userLimit.GetName()),
			}, nil
		}
	}

	// Check default run limit if user is not a part of any defined user limit groups.
	if config.GetUserLimitDefault().GetRun() != nil {
		return &quotapb.PolicyID{
			Config: policyConfigID,
			Key:    runPolicyKey(config.GetName(), defaultPolicy),
		}, nil
	}

	// No limits configured for this user.
	return nil, nil
}

// runPolicyKey is a helper to generate run quota policy key.
func runPolicyKey(configName, name string) *quotapb.PolicyKey {
	return &quotapb.PolicyKey{
		Namespace:    configName,
		Name:         name,
		ResourceType: runResource,
	}
}

// runPolicyEntry is a helper to generate a run quota policy entry.
func runPolicyEntry(polkey *quotapb.PolicyKey, limit uint64) *quotapb.PolicyConfig_Entry {
	return &quotapb.PolicyConfig_Entry{
		Key: polkey,
		Policy: &quotapb.Policy{
			Default: limit,
			Limit:   limit,
			Lifetime: &durationpb.Duration{
				Seconds: accountLifeTimeSeconds,
			},
		},
	}
}

// makeRunQuotaPolicies is a a helper to format run quota policies for the given config groups.
func makeRunQuotaPolicies(project string, configGroups []*prjcfg.ConfigGroup) []*quotapb.PolicyConfig_Entry {
	var policies []*quotapb.PolicyConfig_Entry

	for _, configGroup := range configGroups {
		config := configGroup.Content
		if config == nil {
			continue
		}

		for _, userLimit := range config.GetUserLimits() {
			runLimit := userLimit.GetRun()
			if runLimit == nil {
				continue
			}

			// limit is set to 0 when unlimited = True. The unlimited attribute
			// will be handled by setting `IGNORE_POLICY_BOUNDS` for each
			// quota op.
			runLimitVal := uint64(runLimit.GetMaxActive().GetValue())
			polkey := runPolicyKey(config.GetName(), userLimit.GetName())

			policies = append(policies, runPolicyEntry(polkey, runLimitVal))
		}

		// Add default run quota policy.
		defaultRunLimit := config.GetUserLimitDefault().GetRun()
		if defaultRunLimit == nil {
			continue
		}

		// default limit is set to 0 when unlimited = True || not configured.
		// Accounts using this policy will set `IGNORE_POLICY_BOUNDS` for each
		// quota op.
		defaultRunLimitVal := uint64(defaultRunLimit.GetMaxActive().GetValue())
		defaultkey := runPolicyKey(config.GetName(), defaultPolicy)

		policies = append(policies, runPolicyEntry(defaultkey, defaultRunLimitVal))
	}

	return policies
}

// loadPolicies loads the given configGroups into server/quota.
// If the policy already exists within server/quota, this immediately returns the &quotapb.PolicyConfigID.
func (qm *Manager) loadPolicies(ctx context.Context, project string, configGroups []*prjcfg.ConfigGroup) (*quotapb.PolicyConfigID, error) {
	runQuotaPolicies := makeRunQuotaPolicies(project, configGroups)
	if runQuotaPolicies == nil {
		return nil, nil
	}

	// Load policies into server/quota.
	return qm.qapp.LoadPoliciesAuto(ctx, project, &quotapb.PolicyConfig{
		Policies: runQuotaPolicies,
	})
}

// WritePolicy writes lucicfg updates to the srvquota policies.
func (qm *Manager) WritePolicy(ctx context.Context, project string) (*quotapb.PolicyConfigID, error) {
	// Get all config groups.
	meta, err := prjcfg.GetLatestMeta(ctx, project)
	if err != nil {
		return nil, err
	}

	configGroups, err := meta.GetConfigGroups(ctx)
	if err != nil {
		return nil, err
	}

	return qm.loadPolicies(ctx, project, configGroups)
}

// NewManager creates a new quota manager.
func NewManager() *Manager {
	qinit.Do(func() {
		qapp = srvquota.Register("cv", &srvquota.ApplicationOptions{
			ResourceTypes: []string{runResource, tryjobResource},
		})
	})
	return &Manager{qapp: qapp}
}
