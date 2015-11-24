// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"crypto/subtle"
	"fmt"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// Execution represents either an ongoing execution on the Quest's specified
// distributor, or is a placeholder for an already-completed Execution.
type Execution struct {
	ID      types.UInt32   `gae:"$id"`
	Attempt *datastore.Key `gae:"$parent"`

	// ExecutionKey is a randomized nonce that's used to identify API calls from
	// the current execution to dungeon master. Once DM decides that the execution
	// should no longer be able to make API calls (e.g. it uploaded a result or
	// added new dependencies), DM will revoke this key, making all further API
	// access invalid.
	ExecutionKey []byte `gae:",noindex"`
}

// Done returns true iff the Execution is done (i.e. it has a blank
// ExecutionKey).
func (e *Execution) Done() bool {
	return len(e.ExecutionKey) == 0
}

// Revoke will null-out the ExecutionKey and Put this Execution to the
// datastore.
func (e *Execution) Revoke(c context.Context) error {
	e.ExecutionKey = nil
	return datastore.Get(c).Put(e)
}

func verifyExecutionInternal(c context.Context, aid *types.AttemptID, evkey []byte) (*Attempt, *Execution, error) {
	if len(evkey) == 0 {
		return nil, nil, fmt.Errorf("Empty ExecutionKey")
	}

	ds := datastore.Get(c)

	a := &Attempt{AttemptID: *aid}
	if err := ds.Get(a); err != nil {
		return nil, nil, fmt.Errorf("couldn't get attempt (%+v): %v", a, err)
	}

	e := &Execution{ID: a.CurExecution, Attempt: ds.KeyForObj(a)}
	if err := ds.Get(e); err != nil {
		return nil, nil, fmt.Errorf("couldn't get execution (%+v): %v", e, err)
	}

	if a.State != types.Executing {
		return nil, nil, fmt.Errorf("Attempt is not executing yet")
	}

	if !e.Done() && subtle.ConstantTimeCompare(e.ExecutionKey, evkey) == 0 {
		return nil, nil, fmt.Errorf("Incorrect ExecutionKey")
	}

	return a, e, nil
}

// VerifyExecution verifies that the Attempt is executing, and that evkey
// matches the execution key of the current Execution for this Attempt.
//
// As a bonus, it will return the loaded Attempt and Execution.
func VerifyExecution(c context.Context, aid *types.AttemptID, evkey []byte) (*Attempt, *Execution, error) {
	a, e, err := verifyExecutionInternal(c, aid, evkey)
	if err != nil {
		logging.Errorf(logging.SetError(c, err), "failed to verify execution")
	}
	return a, e, err
}

// InvalidateExecution verifies that the execution key is valid, and then
// revokes the execution key.
//
// As a bonus, it will return the loaded Attempt and Execution.
func InvalidateExecution(c context.Context, aid *types.AttemptID, evkey []byte) (*Attempt, *Execution, error) {
	a, e, err := verifyExecutionInternal(c, aid, evkey)
	if err != nil {
		return nil, nil, err
	}
	return a, e, e.Revoke(c)
}
