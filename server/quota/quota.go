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

package quota

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/msgpackpb"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/quota/internal/lua"
	"go.chromium.org/luci/server/quota/internal/quotakeys"
	"go.chromium.org/luci/server/quota/quotapb"
)

// ErrQuotaApply is returned by Apply when the updates were not
// applied.
//
// See the returned ApplyOpsResponse for details.
var ErrQuotaApply = errors.New("quota.Apply had errors")

// UpdateAccountsScript is a reference to the lua script used by the quota
// library. This is only a public symbol in order to patch it with
// the quotatestmonkeypatch library.
var UpdateAccountsScript = lua.UpdateAccountsScript

// For now, set the request timeout to 2 hours; can make this flexible later.
var updateAccountsRequestTTL = durationpb.New(time.Hour * 2)

// Holds a mask used to detect any invalid quotapb.Op_Options bits.
var invalidOptionsMask uint32

// currentHashScheme reflects the current UpdateAccountsInput.HashScheme
// value. Refer to the comment there for additional context.
const currentHashScheme = 1

const conflictOptions = quotapb.Op_IGNORE_POLICY_BOUNDS | quotapb.Op_DO_NOT_CAP_PROPOSED

func init() {
	for val := range quotapb.Op_Options_name {
		invalidOptionsMask = invalidOptionsMask | uint32(val)
	}
	invalidOptionsMask = ^invalidOptionsMask
}

func convertToInput(ctx context.Context, requestID string, requestTTL *durationpb.Duration, ops []*quotapb.Op) (*quotapb.UpdateAccountsInput, []string, error) {
	ret := &quotapb.UpdateAccountsInput{Ops: make([]*quotapb.RawOp, len(ops))}
	// KEYS will be fed directly as KEYS to the redis script.
	KEYS := stringset.New(len(ops))
	for i, op := range ops {
		if err := op.Validate(); err != nil {
			return nil, nil, errors.Annotate(err, "ops[%d]", i).Err()
		}

		raw := &quotapb.RawOp{
			AccountRef: quotakeys.AccountKey(op.AccountId),
			RelativeTo: op.RelativeTo,
			Delta:      op.Delta,
			Options:    op.Options,
		}
		KEYS.Add(raw.AccountRef)

		if (op.Options & invalidOptionsMask) > 0 {
			return nil, nil, errors.Reason("ops[%d]: unknown options", i).Err()
		}
		if op.Options&uint32(conflictOptions) == uint32(conflictOptions) {
			return nil, nil, errors.Reason("ops[%d]: conflicting options", i).Err()
		}

		if op.PolicyId != nil {
			if op.AccountId.AppId != op.PolicyId.Config.AppId {
				return nil, nil, errors.Reason(
					"ops[%d]: account and policy come from different apps: %s vs %s",
					i, op.AccountId.AppId, op.PolicyId.Config.AppId).Err()
			}
			if op.AccountId.ResourceType != op.PolicyId.Key.ResourceType {
				return nil, nil, errors.Reason(
					"ops[%d]: account and policy are for different resource types: %s vs %s",
					i, op.AccountId.ResourceType, op.PolicyId.Key.ResourceType).Err()
			}
			raw.PolicyRef = quotakeys.PolicyRef(op.PolicyId)
			KEYS.Add(raw.PolicyRef.Config)
		}

		ret.Ops[i] = raw
	}

	h := sha256.New()
	if err := msgpackpb.MarshalStream(h, ret, msgpackpb.Deterministic); err != nil {
		return nil, nil, errors.Annotate(err, "calculating hash").Err()
	}

	if requestID != "" {
		ret.RequestKey = quotakeys.RequestDedupKey(&quotapb.RequestDedupKey{
			Ident:     string(auth.CurrentIdentity(ctx)),
			RequestId: requestID,
		})

		ret.RequestKeyTtl = updateAccountsRequestTTL
		if requestTTL != nil {
			ret.RequestKeyTtl = requestTTL
		}

		KEYS.Add(ret.RequestKey)
	}
	ret.HashScheme = currentHashScheme
	ret.Hash = hex.EncodeToString(h.Sum(nil))

	return ret, KEYS.ToSortedSlice(), nil
}

// ApplyOps combines several quota operations into one atomic action with a single
// requestID.
//
// The requestID won't be consumed until this returns success, and once it's
// successful, it will continue to return success without any quota changes for
// requestTTL. If requestTTL is not set, the TTL defaults to 2 hours. The
// requestID is tied to auth.CurrentIdentity. If requestID is empty, this
// operation is not idempotent.
//
// Policies must already be loaded with LoadPolicies.
func ApplyOps(ctx context.Context, requestID string, requestTTL *durationpb.Duration, ops []*quotapb.Op) (*quotapb.ApplyOpsResponse, error) {
	inputMsg, keys, err := convertToInput(ctx, requestID, requestTTL, ops)
	if err != nil {
		return nil, err
	}
	input, err := msgpackpb.Marshal(
		inputMsg, msgpackpb.Deterministic,
		msgpackpb.WithStringInternTable(keys))
	if err != nil {
		return nil, errors.Annotate(err, "failed to marshal UpdateAccountsInput").Err()
	}

	fullArgs := make(redis.Args, 0, len(keys)+2)
	fullArgs = fullArgs.Add(len(keys))
	fullArgs = fullArgs.AddFlat(keys)
	fullArgs = fullArgs.Add(string(input))

	resp := &quotapb.ApplyOpsResponse{}
	err = withRedisConn(ctx, func(conn redis.Conn) error {
		respRaw, err := redis.String(UpdateAccountsScript.DoContext(ctx, conn, fullArgs...))
		if err != nil {
			return errors.Annotate(err, "running UpdateAccountsScript").Err()
		}
		if err := msgpackpb.Unmarshal(msgpack.RawMessage(respRaw), resp); err != nil {
			return err
		}
		for _, result := range resp.Results {
			if result.Status != quotapb.OpResult_SUCCESS {
				err = ErrQuotaApply
				break
			}
		}
		return err
	})
	return resp, err
}

// GetAccounts fetches the list of requested accounts. If the account does not
// exist, GetAccountsResponse.Account[i].Account is left unset.
// TODO(aravindvasudev): Implement logic to compute Account.ProjectedBalance.
func GetAccounts(ctx context.Context, accounts []*quotapb.AccountID) (*quotapb.GetAccountsResponse, error) {
	resp := &quotapb.GetAccountsResponse{}
	if len(accounts) == 0 {
		return resp, nil
	}

	args := make(redis.Args, 0, len(accounts))
	for _, accountID := range accounts {
		args = append(args, quotakeys.AccountKey(accountID))

		// rsp contains an entry for all the requested accounts, in spite of their existence.
		resp.Accounts = append(resp.Accounts, &quotapb.GetAccountsResponse_AccountState{
			Id: accountID,
		})
	}

	err := withRedisConn(ctx, func(conn redis.Conn) error {
		accountsRaw, err := redis.Strings(conn.Do("MGET", args...))
		if err != nil {
			return errors.Annotate(err, "running MGET").Err()
		}

		for i, accountRaw := range accountsRaw {
			if accountRaw == "" {
				continue
			}

			account := &quotapb.Account{}
			if err := msgpackpb.Unmarshal(msgpack.RawMessage(accountRaw), account); err != nil {
				return err
			}

			resp.Accounts[i].Account = account
		}

		return nil
	})

	if err != nil {
		return nil, errors.Annotate(err, "failed to query accounts").Err()
	}

	return resp, nil
}
