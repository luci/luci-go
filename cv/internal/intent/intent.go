// Copyright 2021 The LUCI Authors.
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

package intent

import (
	"context"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/cv/internal/common"
)

// Intent describes the intention to perform operation(s).
type Intent struct {
	_kind string `gae:"$kind,Intent"`
	// ID is the id of this Intent.
	ID common.IntentID `gae:"$id"`
	// Operations are all operations that will be perfromed by this Intent.
	//
	// An operation is removed from the slice once it completes successfully.
	Operations []*Operation

	leaseExpirationTime time.Time `gae:",noindex"`
	leaseID             string    `gae:",noindex"`
}

// Create creates a new Intent.
func Create(ctx context.Context, id common.IntentID, ops ...*Operation) (*Intent, error) {
	panic("implement")
}

// ErrAlreadyLeased is returned by `Lease` if the requested Intent is
// currently in lease.
var ErrAlreadyLeased = errors.New("already leased")

// ErrNoSuchIntent is returned by `Lease` if the requested Intent doesn't
// exist (might be fully fulfilled already).
var ErrNoSuchIntent = errors.New("no such intent")

// Lease leases the intent for the given `duration`.
//
// Returns
//  - new context that contains lease info and timeouts after `duration`
//  - leased intent
//  - error occurs during the leasing. Returns ErrAlreadyLeased if intent is
//    is already leased. Returns ErrNoSuchIntent if the requested intent
//    doesn't exist.
func Lease(ctx context.Context, id common.IntentID, duration time.Duration) (context.Context, *Intent, error) {
	panic("implement")
}

// Fulfill fulfills the intent by sequentially performing all operations.
//
// It saves the progress once an operation is done and deletes the intent if all operations completes successfully.
func (*Intent) Fulfill(ctx context.Context) error {
	panic("implement")
}
