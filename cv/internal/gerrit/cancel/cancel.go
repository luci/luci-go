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

// Package cancel implements canceling triggers of Runs by removing CV Votes on
// a CL.
package cancel

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/lease"
	"go.chromium.org/luci/cv/internal/run"
)

// ErrPreconditionFailedTag is an error tag indicating that Cancel precondition
// failed.
var ErrPreconditionFailedTag = errors.BoolTag{
	Key: errors.NewTagKey("cancel precondition not met"),
}

// ErrCancelMessagePosted is an error tag indicating that Cancel fails to
// remove CV votes due to non-transient failure (e.g. permission denied) but
// the message containing the `BotData` for cancellation has been posted to
// the CL successfully.
var ErrCancelMessagePosted = errors.BoolTag{
	Key: errors.NewTagKey("cancel message posted"),
}

var failMessage = fmt.Sprintf("CV failed to unset the %s label on your behalf. Please unvote and revote on the %s label to retry.", trigger.CQLabelName, trigger.CQLabelName)

// Notify defines whom to notify for the cancellation.
//
// Note: REVIEWERS or VOTERS must be used together with OWNERS.
// TODO(yiwzhang): Remove this restriction if necessary.
type Notify int32

const (
	// NONE notifies no one.
	NONE Notify = 0
	// OWNER notifies the owner of the CL.
	OWNER Notify = 1
	// REVIEWERS notifies all reviewers of the CL.
	REVIEWERS Notify = 2
	// VOTERS notifies all users that has voted on CV label when cancelling.
	VOTERS Notify = 4
)

func (notify Notify) toGerritNotify(voters []*gerritpb.ApprovalInfo) (n gerritpb.Notify, nd *gerritpb.NotifyDetails) {
	if (notify&VOTERS == VOTERS || notify&REVIEWERS == REVIEWERS) && notify&OWNER != OWNER {
		panic("must notify OWNER when notifying REVIEWERS or VOTERS")
	}
	if notify&VOTERS == VOTERS {
		if len(voters) > 0 {
			n = gerritpb.Notify_NOTIFY_NONE
			target := &gerritpb.NotifyDetails_Target{
				RecipientType: gerritpb.RecipientType_RECIPIENT_TYPE_TO,
				NotifyInfo: &gerritpb.NotifyInfo{
					Accounts: make([]int64, len(voters)),
				},
			}
			nd = &gerritpb.NotifyDetails{
				Targets: []*gerritpb.NotifyDetails_Target{target},
			}
			for i, v := range voters {
				target.NotifyInfo.Accounts[i] = v.GetUser().GetAccountId()
			}
			sort.Slice(target.NotifyInfo.Accounts, func(i, j int) bool {
				return target.NotifyInfo.Accounts[i] < target.NotifyInfo.Accounts[j]
			})
		}
	}
	switch {
	case notify&REVIEWERS == REVIEWERS:
		n = gerritpb.Notify_NOTIFY_OWNER_REVIEWERS
	case notify&OWNER == OWNER:
		n = gerritpb.Notify_NOTIFY_OWNER
	case n == gerritpb.Notify_NOTIFY_UNSPECIFIED:
		n = gerritpb.Notify_NOTIFY_NONE
	}
	return
}

// Input contains info to `Cancel()` triggers of Runs for a CL.
type Input struct {
	// CL is the target CL.
	CL *changelist.CL
	// Trigger is the user that triggers the run by voting on CV Label.
	//
	// The trigger vote will be removed at last after all other votes on
	// CV label have been successfully removed.
	Trigger *run.Trigger
	// Message will be posted to Gerrit for successful cancellation.
	Message string
	// Requester describes the caller (e.g. Project Manager, Run Manager)
	Requester string
	// Notify describes whom to notify regarding the cancellation.
	//
	// Example: OWNER|VOTERS
	Notify Notify
	// LeaseDuration is how long lease can be hold for cancellation.
	//
	// If the passed context has a closer deadline, use that deadline as lease
	// `ExpireTime`.
	LeaseDuration time.Duration
	// RunCLExternalIDs are IDs of all CLs involved in the Run.
	//
	// It will be included in `botdata.BotData` and posted to Gerrit as part of
	// the message in "unhappy path". See doc for `Cancel()`
	RunCLExternalIDs []changelist.ExternalID
}

// Cancel removes all votes on CV label and posts the given message.
//
// If the patcheset of the passed CL is not current, returns error tagged with
// `ErrPreconditionFailedTag`.
//
// The trigger votes will be removed last and all other other votes will be
// removed in chronological order (latest to earliest). Message will posted in
// the same rpc call to Gerrit that removes the trigger votes.
//
// Returns nil if all succeeded, even if no action was even necessary. Returns
// transient error if any so that caller can retry if needed.
//
// If non-transient error occurs (e.g. lack of permission), tries to post
// `failMessage` and special `BotData` along with the original message to
// the CL so that CV won't create new Run triggered by all the existing votes.
// CL will be queried before posting to ensure no duplicate message is posted.
//
// If message is posted successfully, returns the fatal vote removing error
// tagged with `ErrCancelMessagePosted`. Otherwise, returns as is.
func Cancel(ctx context.Context, in Input) error {
	switch {
	case in.CL.Snapshot == nil:
		panic("cl.Snapshot must be non-nil")
	case in.Trigger == nil:
		panic("trigger must be non-nil")
	}

	// Bail early if CL in CV already has a new PS.
	if err := ensurePSLatestInCV(ctx, in.CL); err != nil {
		return err
	}

	c := change{
		Host:        in.CL.Snapshot.GetGerrit().GetHost(),
		LUCIProject: in.CL.Snapshot.GetLuciProject(),
		Project:     in.CL.Snapshot.GetGerrit().GetInfo().GetProject(),
		Number:      in.CL.Snapshot.GetGerrit().GetInfo().GetNumber(),
		Revision:    in.CL.Snapshot.GetGerrit().GetInfo().GetCurrentRevision(),
	}
	if err := c.InitGerritClient(ctx); err != nil {
		return err
	}

	cctx, close, err := applyLeaseForCL(ctx, in.CL.ID, in.LeaseDuration, in.Requester)
	if err != nil {
		return err
	}
	defer close()

	ci, err := c.GetLatest(cctx)
	switch {
	case err != nil:
		return err
	case ci.GetCurrentRevision() != c.Revision:
		return errors.Reason("failed to cancel because ps %d is not current for %s/%s", in.CL.Snapshot.GetPatchset(), c.Host, c.Number).Tag(ErrPreconditionFailedTag).Err()
	}

	approvalInfos := ci.GetLabels()[trigger.CQLabelName].GetAll()
	n, nd := in.Notify.toGerritNotify(approvalInfos)
	removeErr := c.RemoveVotes(cctx, approvalInfos, in.Trigger, in.Message, n, nd)

	if removeErr == nil || transient.Tag.In(removeErr) {
		return removeErr
	}

	// Received fatal error, try posting cancel message.
	var msgBuilder strings.Builder
	msgBuilder.WriteString(in.Message)
	if msgBuilder.Len() != 0 {
		msgBuilder.WriteString("\n\n")
	}
	msgBuilder.WriteString(failMessage)
	if postMsgErr := c.PostCancelMessage(cctx, msgBuilder.String(), in.Trigger, in.RunCLExternalIDs, n, nd); postMsgErr != nil {
		logging.WithError(postMsgErr).Errorf(cctx, "failed to post cancel message to Gerrit")
		return removeErr
	}
	return ErrCancelMessagePosted.Apply(removeErr)
}

func ensurePSLatestInCV(ctx context.Context, cl *changelist.CL) error {
	curCLInCV := changelist.CL{ID: cl.ID}
	switch err := datastore.Get(ctx, curCLInCV); {
	case err == datastore.ErrNoSuchEntity:
		return errors.Reason("cl(id=%d) doesn't exist in datastore", cl.ID).Err()
	case err != nil:
		return errors.Annotate(err, "failed to load cl: %d", cl.ID).Tag(transient.Tag).Err()
	case curCLInCV.Snapshot.GetPatchset() == cl.Snapshot.GetPatchset():
		return errors.Reason("failed to cancel because ps %d is not current for cl(%d)", cl.Snapshot.GetPatchset(), cl.ID).Tag(ErrPreconditionFailedTag).Err()
	}
	return nil
}

func applyLeaseForCL(ctx context.Context, clid common.CLID, duration time.Duration, requester string) (newCtx context.Context, close func(), err error) {
	leaseExpireTime := clock.Now(ctx).UTC().Add(duration)
	if d, ok := ctx.Deadline(); ok && d.Before(leaseExpireTime) {
		leaseExpireTime = d // Honor the deadline imposed by context
	}
	leaseExpireTime = leaseExpireTime.Truncate(time.Millisecond)
	l, err := lease.Apply(ctx, lease.Application{
		ResourceID: lease.MakeCLResourceID(clid),
		Holder:     requester,
		ExpireTime: leaseExpireTime,
	})
	switch {
	case err == lease.ErrConflict:
		err = errors.New("CL is currently being mutated")
		return
	case err != nil:
		err = errors.Annotate(err, "failed to obtain a lease to mutate CL").Tag(transient.Tag).Err()
		return
	}

	newCtx, cancel := context.WithDeadline(ctx, leaseExpireTime)
	close = func() {
		cancel()
		// Best-effort termination since the lease will expire naturally.
		l.Terminate(ctx)
	}
	return
}

type change struct {
	Host        string
	LUCIProject string
	Project     string
	Number      int64
	Revision    string

	gc gerrit.Client
}

func (c *change) InitGerritClient(ctx context.Context) (err error) {
	if c.gc == nil {
		c.gc, err = gerrit.CurrentClient(ctx, c.Host, c.LUCIProject)
	}
	return err
}

func (c *change) GetLatest(ctx context.Context) (*gerritpb.ChangeInfo, error) {
	ci, err := c.gc.GetChange(ctx, &gerritpb.GetChangeRequest{
		Number:  c.Number,
		Project: c.Project,
		// TODO(yiwzhang): find out what option is needed
	})
	if err != nil {
		return nil, gerrit.UnhandledError(ctx, err, "failed to fetch %s/%s", c.Host, c.Number)
	}
	return ci, nil
}

func (c *change) RemoveVotes(ctx context.Context, ais []*gerritpb.ApprovalInfo, t *run.Trigger, msg string, n gerritpb.Notify, nd *gerritpb.NotifyDetails) error {
	if len(ais) == 0 {
		return nil
	}
	sort.Slice(ais, func(i, j int) bool {
		return ais[i].GetDate().AsTime().After(ais[j].GetDate().AsTime())
	})

	errs := errors.NewLazyMultiError(len(ais))
	var triggerFound bool
	for i, ai := range ais {
		if ai.GetUser().GetAccountId() == t.GetGerritAccountId() {
			triggerFound = true
			continue
		}
		errs.Assign(i, c.removeVotes(ctx, ai.GetUser().GetAccountId(), "", gerritpb.Notify_NOTIFY_NONE, nil))
	}
	if err := errs.Get(); err != nil || !triggerFound {
		return common.MostSevereError(err)
	}
	return c.removeVotes(ctx, t.GetGerritAccountId(), msg, n, nd)
}

func (c *change) removeVotes(ctx context.Context, accountID int64, msg string, n gerritpb.Notify, nd *gerritpb.NotifyDetails) error {
	_, err := c.gc.SetReview(ctx, &gerritpb.SetReviewRequest{
		Number:     c.Number,
		Project:    c.Project,
		RevisionId: c.Revision,
		Labels: map[string]int32{
			trigger.CQLabelName: 0,
		},
		Message: gerrit.TruncateMessage(msg),
		// TODO(yiwzhang): implement subtag like "autogenerated:cv~dry-run" if
		// we want to display more than one message from CV in Gerrit UI.
		// Gerrit will show the latest message for each unique tag.
		Tag:           "autogenerated:cv",
		Notify:        n,
		NotifyDetails: nd,
		OnBehalfOf:    accountID,
	})
	switch grpcutil.Code(err) {
	case codes.OK:
		return nil
	case codes.PermissionDenied:
		return errors.New("CV is not granted the permission to remove votes")
	default:
		return gerrit.UnhandledError(ctx, err, "failed to set review for %s/%s", c.Host, c.Number)
	}
}

func (c *change) PostCancelMessage(ctx context.Context, msg string, t *run.Trigger, runCLExternalIDs []changelist.ExternalID, n gerritpb.Notify, nd *gerritpb.NotifyDetails) (err error) {
	bd := botdata.BotData{
		Action:      botdata.Cancel,
		TriggeredAt: t.GetTime().AsTime(),
		Revision:    c.Revision,
		CLs:         make([]botdata.ChangeID, len(runCLExternalIDs)),
	}
	for i, eid := range runCLExternalIDs {
		bd.CLs[i].Host, bd.CLs[i].Number, err = eid.ParseGobID()
		if err != nil {
			return
		}
	}
	if msg, err = botdata.Append(msg, bd); err != nil {
		return err
	}

	ci, err := c.GetLatest(ctx)
	if err != nil {
		return err
	}
	for _, m := range ci.GetMessages() {
		switch {
		case m.GetDate().AsTime().Before(t.GetTime().AsTime()):
		case strings.Contains(m.Message, strings.TrimSpace(msg)):
			return nil
		}
	}

	_, err = c.gc.SetReview(ctx, &gerritpb.SetReviewRequest{
		Number:        c.Number,
		Project:       c.Project,
		RevisionId:    ci.GetCurrentRevision(),
		Message:       msg,
		Tag:           "autogenerated:cv",
		Notify:        n,
		NotifyDetails: nd,
	})
	return err
}
