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

package model

import (
	"context"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"
)

// Txn runs the callback in a datastore transaction.
//
// Transient-tagged errors trigger a transaction retry. If all retries are
// exhausted, returns a transient-tagged error itself.
func Txn(ctx context.Context, cb func(context.Context) error) error {
	var attempt int
	var innerErr error

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		attempt++
		if attempt != 1 {
			if innerErr != nil {
				logging.Warningf(ctx, "Retrying the transaction after the error: %s", innerErr)
			} else {
				logging.Warningf(ctx, "Retrying the transaction: failed to commit")
			}
		}
		innerErr = cb(ctx)
		if transient.Tag.In(innerErr) {
			return datastore.ErrConcurrentTransaction // causes a retry
		}
		return innerErr
	}, nil)

	if err != nil {
		// If the transaction callback failed, prefer its error.
		if innerErr != nil {
			return innerErr
		}
		// Here it can only be a commit error (i.e. produced by RunInTransaction
		// itself, not by its callback). We treat them as transient.
		return transient.Tag.Apply(err)
	}

	return nil
}

// EqualStrSlice compares two string slices.
func EqualStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, s := range a {
		if b[i] != s {
			return false
		}
	}
	return true
}

// asTime converts a timestamp proto to optional time.Time for Datastore.
func asTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime().UTC()
}

// fetchAssets fetches a bunch of asset entities given their IDs.
//
// If shouldExist is true, fails if some of them do not exist. Otherwise
// constructs new empty Asset structs in place of missing ones.
func fetchAssets(ctx context.Context, assets []string, shouldExist bool) (map[string]*Asset, error) {
	ents := make([]*Asset, len(assets))
	for idx, id := range assets {
		ents[idx] = &Asset{ID: id}
	}

	if err := datastore.Get(ctx, ents); err != nil {
		merr, ok := err.(errors.MultiError)
		if !ok {
			return nil, transient.Tag.Apply(err)
		}

		var missing []string
		for idx, err := range merr {
			switch {
			case err == datastore.ErrNoSuchEntity:
				if shouldExist {
					missing = append(missing, assets[idx])
				} else {
					ents[idx] = &Asset{
						ID:    assets[idx],
						Asset: &modelpb.Asset{Id: assets[idx]},
					}
				}
			case err != nil:
				return nil, transient.Tag.Apply(err)
			}
		}

		if len(missing) != 0 {
			return nil, errors.Fmt("assets entities unexpectedly missing: %s", strings.Join(missing, ", "))
		}
	}

	assetMap := make(map[string]*Asset, len(assets))
	for _, ent := range ents {
		assetMap[ent.ID] = ent
	}

	return assetMap, nil
}

// IsActuateDecision returns true for ACTUATE_* decisions.
func IsActuateDecision(d modelpb.ActuationDecision_Decision) bool {
	return d == modelpb.ActuationDecision_ACTUATE_STALE ||
		d == modelpb.ActuationDecision_ACTUATE_FORCE
}
