// Copyright 2025 The LUCI Authors.
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

package config

import (
	"time"
)

// CulpritRevertEvent stores the most recent automated revert for a tree.
//
// Design: Only the latest revert is kept per tree (TreeName is the entity ID).
// Each new revert overwrites the previous one. The revert remains useful as long
// as RevertLandTime > newestFailingBuildStartTime, which means it can:
//  1. Help reopen the tree after all builds pass OR the revert lands
//  2. Continue helping after reopening by ignoring failures from builds that
//     started before the revert (preventing premature re-closure)
//
// This stateless design avoids bugs where a revert is marked "processed" and
// discarded before it has exhausted its usefulness.
//
// This entity is created by LUCI Bisection when it reverts a culprit CL
// and is consumed by the tree status update cron job.
type CulpritRevertEvent struct {
	// TreeName is the tree that should be considered for reopening.
	// This is the entity ID.
	TreeName string `gae:"$id"`

	// RevertLandTime is when the automated revert landed.
	// Used to compare against TreeCloser.BuildCreateTime.
	// If all failing builds started before this time, the revert
	// should fix them all and we can reopen the tree.
	RevertLandTime time.Time

	// CulpritReviewURL is the URL of the culprit CL that was reverted.
	// Used for generating the tree status message.
	CulpritReviewURL string

	// RevertReviewURL is the URL of the revert CL that landed.
	// Used for generating the tree status message.
	RevertReviewURL string

	// CreatedAt is when this event was created.
	// Used for cleanup of old events (TTL-like behavior).
	CreatedAt time.Time
}
