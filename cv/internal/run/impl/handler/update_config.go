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

package handler

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"golang.org/x/sync/errgroup"
)

// UpdateConfig implements Handler interface.
func (impl *Impl) UpdateConfig(ctx context.Context, rs *state.RunState, hash string) (*Result, error) {
	// First, check if config update is feasible given Run Status.
	switch status := rs.Run.Status; {
	case status == run.Status_STATUS_UNSPECIFIED:
		err := errors.Reason("CRITICAL: Received UpdateConfig event but Run is in unspecified status").Err()
		common.LogError(ctx, err)
		panic(err)
	case status == run.Status_SUBMITTING:
		// Don't consume the events so that the RM executing the submission will
		// be able to read the UpdateConfig events and take necessary actions after
		// submission completes. For example, a new PS is uploaded for one of
		// the not submitted CLs and cause the run submission to fail. RM should
		// cancel this Run instead of retrying.
		return &Result{State: rs, PreserveEvents: true}, nil
	case run.IsEnded(status):
		// Run is ended, update on CL shouldn't change the Run state.
		return &Result{State: rs}, nil
	}

	// Second, check if config update is necessary.
	var curMeta, newMeta config.Meta
	eg, eCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		var err error
		curMeta, err = config.GetHashMeta(eCtx, rs.Run.ID.LUCIProject(), rs.Run.ConfigGroupID.Hash())
		return err
	})
	eg.Go(func() error {
		var err error
		newMeta, err = config.GetHashMeta(eCtx, rs.Run.ID.LUCIProject(), hash)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	if curMeta.EVersion >= newMeta.EVersion {
		// Current config at least as recent as the "new" one.
		return &Result{State: rs}, nil
	}

	// Third, try upgrading config.
	// A Run remains feasible iff all of:
	//  * each CL is still watched by one ConfigGroup;
	//  * all CLs are watched by the same ConfigGroup,
	//    although ConfigGroup's name may have changed;
	//  * each CL's .Trigger is still the same.
	// TODO(tandrii): implement.
	return &Result{State: rs}, nil
}
