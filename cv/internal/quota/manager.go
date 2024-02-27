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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	srvquota "go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/quota/quotapb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"
)

var qinit sync.Once
var qapp SrvQuota

const (
	// appID to register with quota module.
	appID = "cv"

	// Resource types that the quota can use.
	runResource    = "runs"
	tryjobResource = "tryjobs"

	defaultUserLimit = "default"

	// Default lifetime of a quota account.
	accountLifeTime = 3 * 24 * time.Hour // 3 days
)

// Manager manages the quota accounts for CV users.
type Manager struct {
	qapp SrvQuota
}

// SrvQuota manages quota
type SrvQuota interface {
	LoadPoliciesManual(ctx context.Context, realm string, version string, cfg *quotapb.PolicyConfig) (*quotapb.PolicyConfigID, error)
	AccountID(realm, namespace, name, resourceType string) *quotapb.AccountID
}

// DebitRunQuota debits the run quota from a given user's account.
func (qm *Manager) DebitRunQuota(ctx context.Context, r *run.Run) (*quotapb.OpResult, *cfgpb.UserLimit, error) {
	return qm.runQuotaOp(ctx, r, "debit", -1)
}

// CreditRunQuota credits the run quota into a given user's account.
func (qm *Manager) CreditRunQuota(ctx context.Context, r *run.Run) (*quotapb.OpResult, *cfgpb.UserLimit, error) {
	return qm.runQuotaOp(ctx, r, "credit", 1)
}

// DebitTryjobQuota debits the tryjob quota from a given user's account.
func (qm *Manager) DebitTryjobQuota(ctx context.Context) (*quotapb.OpResult, error) {
	return nil, nil
}

// CreditTryjobQuota credits the tryjob quota into a given user's account.
func (qm *Manager) CreditTryjobQuota(ctx context.Context) (*quotapb.OpResult, error) {
	return nil, nil
}

// RunQuotaAccountID returns the account id of the run quota for the given run.
func (qm *Manager) RunQuotaAccountID(r *run.Run) *quotapb.AccountID {
	// The email is hashed using md5 for the account id to avoid PII.
	emailHash := md5.Sum([]byte(r.BilledTo.Email()))
	return qm.qapp.AccountID(r.ID.LUCIProject(), r.ConfigGroupID.Name(), hex.EncodeToString(emailHash[:]), runResource)
}

// runQuotaOp updates the run quota for the given run state by the given delta.
func (qm *Manager) runQuotaOp(ctx context.Context, r *run.Run, opID string, delta int64) (*quotapb.OpResult, *cfgpb.UserLimit, error) {
	userLimit, err := findRunLimit(ctx, r)

	// userLimit == nil when no run user limit is configured for this user.
	if err != nil || userLimit == nil {
		return nil, nil, err
	}

	policyID := &quotapb.PolicyID{
		Config: policyConfigID(r.ID.LUCIProject(), r.ConfigGroupID.Hash()),
		Key:    runPolicyKey(r.ConfigGroupID.Name(), userLimit.GetName()),
	}

	// When policy is set to unlimited, the op is applied with IGNORE_POLICY_BOUNDS.
	options := quotapb.Op_WITH_POLICY_LIMIT_DELTA
	if userLimit.GetRun().GetMaxActive().GetUnlimited() {
		options |= quotapb.Op_IGNORE_POLICY_BOUNDS
	}

	quotaOp := []*quotapb.Op{
		{
			AccountId:  qm.RunQuotaAccountID(r),
			PolicyId:   policyID,
			RelativeTo: quotapb.Op_CURRENT_BALANCE,
			Delta:      delta,
			Options:    uint32(options),
		},
	}

	// When server/quota does not have the policyId already, rewrite the policy and retry the op.
	var opResponse *quotapb.ApplyOpsResponse
	err = retry.Retry(clock.Tag(ctx, common.LaunchRetryClockTag), makeRetryFactory(), func() (err error) {
		opResponse, err = srvquota.ApplyOps(ctx, requestID(r.ID, opID), durationpb.New(accountLifeTime), quotaOp)
		if errors.Unwrap(err) == srvquota.ErrQuotaApply && opResponse.Results[0].Status == quotapb.OpResult_ERR_UNKNOWN_POLICY {
			if _, err := qm.WritePolicy(ctx, r.ID.LUCIProject()); err != nil {
				return err
			}

			return errors.Annotate(err, "ApplyOps: ERR_UNKNOWN_POLICY").Tag(transient.Tag).Err()
		}

		return
	}, nil)

	if err == nil || errors.Unwrap(err) == srvquota.ErrQuotaApply {
		metrics.Internal.QuotaOp.Add(
			ctx,
			1,
			r.ID.LUCIProject(),
			r.ConfigGroupID.Name(),
			policyID.GetKey().GetName(),
			runResource,
			opID,
			opResponse.Results[0].Status.String(),
		)

		// On ErrQuotaApply, OpResult.Status stores the reason for failure.
		return opResponse.Results[0], userLimit, err
	}

	metrics.Internal.QuotaOp.Add(
		ctx,
		1,
		r.ID.LUCIProject(),
		r.ConfigGroupID.Name(),
		policyID.GetKey().GetName(),
		runResource,
		opID,
		"UNKNOWN_ERROR",
	)

	return nil, userLimit, err
}

func makeRetryFactory() retry.Factory {
	return transient.Only(func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   100 * time.Millisecond,
				Retries: 3,
			},
			Multiplier: 2,
		}
	})
}

// requestID constructs the idempotent requestID for the quota operation.
func requestID(runID common.RunID, op string) string {
	return string(runID) + "/" + op
}

// findRunLimit identifies the UserLimit to use for the given run.
func findRunLimit(ctx context.Context, r *run.Run) (*cfgpb.UserLimit, error) {
	project := r.ID.LUCIProject()
	cfgGroup, err := prjcfg.GetConfigGroup(ctx, project, r.ConfigGroupID)
	if err != nil {
		return nil, err
	}

	config := cfgGroup.Content
	if config == nil {
		return nil, fmt.Errorf("cannot find cfgGroup content")
	}

	user := r.BilledTo
	for _, userLimit := range config.GetUserLimits() {
		if userLimit.GetRun() == nil {
			continue
		}

		var groups []string
		for _, principal := range userLimit.GetPrincipals() {
			switch parts := strings.SplitN(principal, ":", 2); {
			case len(parts) != 2:
				// Each entry can be either an identity string "user:<email>" or a LUCI group reference "group:<name>".
				return nil, fmt.Errorf("improper format for principal: %s", principal)
			case parts[0] == "user" && parts[1] == user.Email():
				return userLimit, nil
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
			return userLimit, nil
		}
	}

	// Check default run limit if user is not a part of any defined user limit groups.
	if config.GetUserLimitDefault().GetRun() != nil {
		userLimit := config.GetUserLimitDefault()

		// Override default userLimit name to `defaultUserLimit`. The name field
		// is assumed to be `optional` and hence standardized to
		// `defaultUserLimit`. This field is used by policyId.
		userLimit.Name = defaultUserLimit
		return userLimit, nil
	}

	// No limits configured for this user.
	return nil, nil
}

// policyConfigID is a helper to generate quota policyConfigID.
func policyConfigID(realm, version string) *quotapb.PolicyConfigID {
	return &quotapb.PolicyConfigID{
		AppId:   appID,
		Realm:   realm,
		Version: version,
	}
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
				Seconds: int64(accountLifeTime.Seconds()),
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
		defaultkey := runPolicyKey(config.GetName(), defaultUserLimit)

		policies = append(policies, runPolicyEntry(defaultkey, defaultRunLimitVal))
	}

	return policies
}

// loadPolicies loads the given configGroups into server/quota.
// If the policy already exists within server/quota, this immediately returns the &quotapb.PolicyConfigID.
func (qm *Manager) loadPolicies(ctx context.Context, project string, configGroups []*prjcfg.ConfigGroup, version string) (*quotapb.PolicyConfigID, error) {
	runQuotaPolicies := makeRunQuotaPolicies(project, configGroups)
	if runQuotaPolicies == nil {
		return nil, nil
	}

	// Load policies into server/quota.
	return qm.qapp.LoadPoliciesManual(ctx, project, version, &quotapb.PolicyConfig{
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

	return qm.loadPolicies(ctx, project, configGroups, meta.Hash())
}

// NewManager creates a new quota manager.
func NewManager() *Manager {
	qinit.Do(func() {
		qapp = srvquota.Register(appID, &srvquota.ApplicationOptions{
			ResourceTypes: []string{runResource, tryjobResource},
		})
	})
	return &Manager{qapp: qapp}
}
