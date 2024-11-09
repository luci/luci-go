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

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"
)

// shouldRecordHistory returns true if the new history entry should be recorded.
//
// It is skipped if it is not sufficiently interesting compared to the last
// committed entry.
//
// Note that skipping a historical record also skips sending any notifications
// related to it (notifications need an AssetHistory record to link to to be
// useful).
func shouldRecordHistory(next, prev *modelpb.AssetHistory) bool {
	nextDecision := next.Decision.Decision
	prevDecision := prev.Decision.Decision
	switch {
	case nextDecision == modelpb.ActuationDecision_SKIP_UPTODATE && IsActuateDecision(prevDecision):
		return false // this particular transition is very common and not interesting
	case nextDecision != prevDecision:
		return true // other changes are always interesting
	case IsActuateDecision(nextDecision):
		return true // active actuations are also always interesting
	case nextDecision == modelpb.ActuationDecision_SKIP_UPTODATE:
		return false // repeating UPTODATE decisions are boring, it is steady state
	case nextDecision == modelpb.ActuationDecision_SKIP_DISABLED:
		return false // repeating DISABLED decisions are also boring
	case nextDecision == modelpb.ActuationDecision_SKIP_LOCKED:
		return !sameLocks(next.Decision.Locks, prev.Decision.Locks)
	case nextDecision == modelpb.ActuationDecision_SKIP_BROKEN:
		return true // errors are always interesting (for retries and alerts)
	default:
		panic("unreachable")
	}
}

func sameLocks(a, b []*modelpb.ActuationLock) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Id != b[i].Id {
			return false
		}
	}
	return true
}

// historyRecorder collects AssetHistory records emitted by an actuation to
// store them and send notifications based on them.
type historyRecorder struct {
	actuation *modelpb.Actuation
	entries   []*modelpb.AssetHistory
}

// recordAndNotify emits a notification and records the historical entry.
func (h *historyRecorder) recordAndNotify(e *modelpb.AssetHistory) {
	h.entries = append(h.entries, e)
	h.notifyOnly(e)
}

// notifyOnly emits the notification without updating the history.
//
// Useful for sending notifications pertaining to ongoing actuations.
func (h *historyRecorder) notifyOnly(e *modelpb.AssetHistory) {
	// TODO
}

// commit prepares the history entries for commit and emits TQ tasks.
//
// Must be called inside a transaction. Returns a list of entities to
// transactionally store.
func (h *historyRecorder) commit(ctx context.Context) ([]any, error) {
	// TODO: Emit TQ tasks.
	toPut := make([]any, len(h.entries))
	for idx, entry := range h.entries {
		toPut[idx] = &AssetHistory{
			ID:      entry.HistoryId,
			Parent:  datastore.NewKey(ctx, "Asset", entry.AssetId, 0, nil),
			Entry:   entry,
			Created: asTime(entry.Actuation.Created),
		}
	}
	return toPut, nil
}
