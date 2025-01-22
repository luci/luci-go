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

package model

import (
	"context"
	"time"

	"go.chromium.org/luci/gae/service/datastore"
)

// BotScannerState stores the state of the bot scanner.
//
// It is used to throttle scan frequency of individual scan.BotVisitor visitors.
type BotScannerState struct {
	_ datastore.PropertyMap `gae:"-,extra"`

	// Key is BotScannerStateKey.
	Key *datastore.Key `gae:"$key"`
	// VisitorState stores per-visitor state.
	VisitorState []BotVisitorState `gae:"bot_visitor_state,noindex"`
}

// BotVisitorState stores the state of a single scan.BotVisitor.
type BotVisitorState struct {
	// VisitorID is ID of the corresponding visitor.
	VisitorID string `gae:"visitor_id"`
	// LastScan is the time when the previous scan has started.
	LastScan time.Time `gae:"last_scan"`
}

// BotScannerStateKey is BotScannerState entity key.
func BotScannerStateKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "BotScannerState", "", 1, nil)
}
