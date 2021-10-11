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

package longops

import (
	"context"
	"fmt"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
)

// PostStartMessageOp posts a start message on each of the Run CLs.
//
// PostStartMessageOp is a single-use object.
type PostStartMessageOp struct {
	// All public fields must be set.

	*Base
	GFactory gerrit.Factory

	// These private fields are set internally as implementation details.

	rcls       []*run.RunCL
	cfg        *prjcfg.ConfigGroup
	botdataCLs []botdata.ChangeID
}

// Do actually posts the start messages.
func (op *PostStartMessageOp) Do(ctx context.Context) (*eventpb.LongOpCompleted, error) {
	op.assertCalledOnce()

	if op.IsCancelRequested() {
		return &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_CANCELLED}, nil
	}
	if err := op.prepare(ctx); err != nil {
		return nil, err
	}

	cancelledCount := int32(0)
	err := parallel.WorkPool(min(len(op.rcls), 8), func(work chan<- func() error) {
		for _, rcl := range op.rcls {
			rcl := rcl
			work <- func() error {
				switch err := op.doCL(ctx, rcl); {
				case errors.Unwrap(err) == errCancelHonored:
					atomic.AddInt32(&cancelledCount, 1)
					return nil
				case err != nil:
					return errors.Annotate(err, "failed to post start message on CL %d %q", rcl.ID, rcl.ExternalID).Err()
				default:
					return nil
				}
			}
		}
	})
	switch {
	case err != nil:
		// TODO(tandrii): when permanent, set appropriate fields in LongOpCompleted
		// event.
		return nil, common.MostSevereError(err)
	case cancelledCount > 0:
		return &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_CANCELLED}, nil
	default:
		return &eventpb.LongOpCompleted{}, nil
	}
}

func (op *PostStartMessageOp) prepare(ctx context.Context) error {
	eg, eCtx := errgroup.WithContext(ctx)
	eg.Go(func() (err error) {
		op.rcls, err = run.LoadRunCLs(eCtx, op.Run.ID, op.Run.CLs)
		return
	})
	eg.Go(func() (err error) {
		op.cfg, err = prjcfg.GetConfigGroup(eCtx, op.Run.ID.LUCIProject(), op.Run.ConfigGroupID)
		return
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	op.botdataCLs = make([]botdata.ChangeID, len(op.rcls))
	for i, rcl := range op.rcls {
		op.botdataCLs[i].Host = rcl.Detail.GetGerrit().GetHost()
		op.botdataCLs[i].Number = rcl.Detail.GetGerrit().GetInfo().GetNumber()
	}
	return nil
}

func (op *PostStartMessageOp) doCL(ctx context.Context, rcl *run.RunCL) error {
	if rcl.Detail.GetGerrit() == nil {
		panic(fmt.Errorf("CL %d is not a Gerrit CL", rcl.ID))
	}

	if op.IsCancelRequested() {
		return errCancelHonored
	}
	// TODO(tandrii): implement.
	return errors.New("not implemented")
}
