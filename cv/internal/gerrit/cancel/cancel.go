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

// Package cancel implements cancelling triggers of Run by removing CQ Votes
// on a CL.
package cancel

import (
	"context"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/lease"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
)

// ErrPreconditionFailedTag is an error tag indicating that Cancel precondition
// failed.
var ErrPreconditionFailedTag = errors.BoolTag{
	Key: errors.NewTagKey("cancel precondition not met"),
}

// ErrPermanentTag is an error tag indicating that error occurs during the
// cancellation is permanent (e.g. lack of vote permission).
var ErrPermanentTag = errors.BoolTag{
	Key: errors.NewTagKey("permanent error while cancelling triggers"),
}

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
	// VOTERS notifies all users that have voted on CQ label when cancelling.
	VOTERS Notify = 4
)

func (notify Notify) validate() error {
	if (notify&VOTERS == VOTERS || notify&REVIEWERS == REVIEWERS) && notify&OWNER != OWNER {
		return errors.New("must notify OWNER when notifying REVIEWERS or VOTERS")
	}
	return nil
}

func (notify Notify) toGerritNotify(voterAccounts []int64) (n gerritpb.Notify, nd *gerritpb.NotifyDetails) {
	n = gerritpb.Notify_NOTIFY_NONE
	if len(voterAccounts) > 0 && notify&VOTERS == VOTERS {
		nd = &gerritpb.NotifyDetails{
			Recipients: []*gerritpb.NotifyDetails_Recipient{
				{
					RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
					Info: &gerritpb.NotifyDetails_Info{
						Accounts: voterAccounts,
					},
				},
			},
		}
	}
	switch {
	case notify&REVIEWERS == REVIEWERS:
		n = gerritpb.Notify_NOTIFY_OWNER_REVIEWERS
	case notify&OWNER == OWNER:
		n = gerritpb.Notify_NOTIFY_OWNER
	}
	return
}

// Input contains info to cancel triggers of Run on a CL.
type Input struct {
	// CL is a Gerrit CL entity.
	//
	// Must have CL.Snapshot set.
	CL *changelist.CL
	// Trigger identifies the triggering vote. Required.
	//
	// Removed only after all other votes on CQ label are removed.
	Trigger *run.Trigger
	// LUCIProject is the project that initiates this cancellation.
	//
	// The project scoped account of this LUCI project SHOULD have the permission
	// to set the CQ label on behalf of other users in Gerrit.
	LUCIProject string
	// Message to be posted along with the triggering vote removal
	Message string
	// Requester describes the caller (e.g. Project Manager, Run Manager).
	Requester string
	// Notify describes whom to notify regarding the cancellation.
	//
	// Example: OWNER|REVIEWERS|VOTERS
	Notify Notify
	// LeaseDuration is how long a lease will be held for this cancellation.
	//
	// If the passed context has a closer deadline, uses that deadline as lease
	// `ExpireTime`.
	LeaseDuration time.Duration
	// ConfigGroups are the ConfigGroups that are watching this CL.
	//
	// They are used to remove votes for additional modes. Normally, there is
	// just 1 ConfigGroup.
	ConfigGroups []*prjcfg.ConfigGroup
	// RunCLExternalIDs are IDs of all CLs involved in the Run.
	//
	// It will be included in `botdata.BotData` and posted to Gerrit as part of
	// the message in "unhappy path". See doc for `Cancel()`
	//
	// TODO(yiwzhang): consider dropping after M1 is launched if it is not adding
	// any value to include those IDs in the bot data.
	RunCLExternalIDs []changelist.ExternalID
}

// Cancel removes or "deactivates" CQ-triggering votes on a CL and posts the
// given message.
//
// Returns error tagged with `ErrPreconditionFailedTag` if one of the
// following conditions is matched.
//  * The patchset of the provided CL is not the latest in Gerrit.
//  * The provided CL gets `changelist.AccessDenied` or
//    `changelist.AccessDeniedProbably` from Gerrit.
//
// Normally, the triggering vote(s) is removed last and all other votes
// are removed in chronological order (latest to earliest).
// After all votes are removed, the message is posted to Gerrit.
//
// Abnormally, e.g. lack of permission to remove votes, falls back to post a
// special message which "deactivates" the triggering votes. This special
// message is a combination of:
//   * the original message in the input
//   * reason for abnormality,
//   * special `botdata.BotData` which ensures CV won't consider previously
//     triggering votes as triggering in the future.
func Cancel(ctx context.Context, gFactory gerrit.ClientFactory, in Input) error {
	switch {
	case in.CL.Snapshot == nil:
		panic("cl.Snapshot must be non-nil")
	case in.Trigger == nil:
		panic("trigger must be non-nil")
	case in.LUCIProject != in.CL.Snapshot.GetLuciProject():
		panic(errors.Reason("mismatched LUCI Project: got %q in input and %q in CL snapshot", in.LUCIProject, in.CL.Snapshot.GetLuciProject()).Err())
	case in.CL.AccessKindFromCodeReviewSite(ctx, in.LUCIProject) != changelist.AccessGranted:
		return errors.New("failed to cancel trigger because CV lost access to this CL", ErrPreconditionFailedTag)
	}
	if err := in.Notify.validate(); err != nil {
		panic(err)
	}

	if err := ensurePSLatestInCV(ctx, in.CL); err != nil {
		return err
	}

	c := change{
		Host:        in.CL.Snapshot.GetGerrit().GetHost(),
		LUCIProject: in.LUCIProject,
		Project:     in.CL.Snapshot.GetGerrit().GetInfo().GetProject(),
		Number:      in.CL.Snapshot.GetGerrit().GetInfo().GetNumber(),
		Revision:    in.CL.Snapshot.GetGerrit().GetInfo().GetCurrentRevision(),
	}

	var err error
	if c.gc, err = gFactory(ctx, c.Host, c.LUCIProject); err != nil {
		return err
	}

	leaseCtx, close, err := applyLeaseForCL(ctx, in.CL.ID, in.LeaseDuration, in.Requester)
	if err != nil {
		return err
	}
	defer close()
	return cancelLeased(leaseCtx, &c, &in)
}

// TODO(tandrii): merge with prjmanager/purger's error messages.
var failMessage = "CV failed to unset the " + trigger.CQLabelName +
	" label on your behalf. Please unvote and revote on the " +
	trigger.CQLabelName + " label to retry."

func cancelLeased(ctx context.Context, c *change, in *Input) error {
	logging.Infof(ctx, "Canceling triggers on %s/%d", c.Host, c.Number)
	ci, err := c.getLatest(ctx)
	switch {
	case err != nil:
		return err
	case ci.GetUpdated().AsTime().Before(in.CL.Snapshot.GetGerrit().GetInfo().GetUpdated().AsTime()):
		return errors.Reason("got stale change info from gerrit for %s/%d", c.Host, c.Number).Tag(transient.Tag).Err()
	case ci.GetCurrentRevision() != c.Revision:
		return errors.Reason("failed to cancel because ps %d is not current for %s/%d", in.CL.Snapshot.GetPatchset(), c.Host, c.Number).Tag(ErrPreconditionFailedTag).Err()
	}

	labelsToRemove := stringset.NewFromSlice(trigger.CQLabelName)
	if l := in.Trigger.GetAdditionalLabel(); l != "" {
		labelsToRemove.Add(l)
	}
	for _, cg := range in.ConfigGroups {
		for _, am := range cg.Content.GetAdditionalModes() {
			if l := am.GetTriggeringLabel(); l != "" {
				labelsToRemove.Add(l)
			}
		}
	}
	labelsToRemove.Iter(func(label string) bool {
		c.recordVotesToRemove(label, ci)
		return true
	})

	removeErr := c.removeVotesAndPostMsg(ctx, ci, in.Trigger, in.Message, in.Notify)
	if removeErr == nil || !ErrPermanentTag.In(removeErr) {
		return removeErr
	}

	// Received permanent error, try posting message.
	logging.Warningf(ctx, "Falling back to canceling via botdata message %s/%d", c.Host, c.Number)
	var msgBuilder strings.Builder
	if in.Message != "" {
		msgBuilder.WriteString(in.Message)
		msgBuilder.WriteString("\n\n")
	}
	msgBuilder.WriteString(failMessage)
	if err := c.postCancelMessage(ctx, ci, msgBuilder.String(), in.Trigger, in.RunCLExternalIDs, in.Notify); err != nil {
		// Return the original error, but add details from just posting a message.
		return errors.Annotate(removeErr, "even just posting message also failed: %s", err).Err()
	}
	return nil
}

func ensurePSLatestInCV(ctx context.Context, cl *changelist.CL) error {
	curCLInCV := &changelist.CL{ID: cl.ID}
	switch err := datastore.Get(ctx, curCLInCV); {
	case err == datastore.ErrNoSuchEntity:
		return errors.Reason("cl(id=%d) doesn't exist in datastore", cl.ID).Err()
	case err != nil:
		return errors.Annotate(err, "failed to load cl: %d", cl.ID).Tag(transient.Tag).Err()
	case curCLInCV.Snapshot.GetPatchset() > cl.Snapshot.GetPatchset():
		return errors.Reason("failed to cancel because ps %d is not current for cl(%d)", cl.Snapshot.GetPatchset(), cl.ID).Tag(ErrPreconditionFailedTag).Err()
	}
	return nil
}

func applyLeaseForCL(ctx context.Context, clid common.CLID, duration time.Duration, requester string) (context.Context, func(), error) {
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
		return nil, nil, errors.Annotate(err, "CL is currently being mutated").Tag(transient.Tag).Err()
	case err != nil:
		return nil, nil, err
	}

	dctx, cancel := context.WithDeadline(ctx, leaseExpireTime)
	close := func() {
		cancel()
		if err := l.Terminate(ctx); err != nil {
			// Best-effort termination since lease will expire naturally.
			errors.Log(ctx, err)
		}
	}
	return dctx, close, nil
}

type change struct {
	Host        string
	LUCIProject string
	Project     string
	Number      int64
	Revision    string

	gc gerrit.Client
	// votesToRemove maps accountID to a set of labels.
	//
	// For ease of passing to SetReview API, each label maps to 0.
	votesToRemove map[int64]map[string]int32
}

func (c *change) getLatest(ctx context.Context) (*gerritpb.ChangeInfo, error) {
	ci, err := c.gc.GetChange(ctx, &gerritpb.GetChangeRequest{
		Number:  c.Number,
		Project: c.Project,
		Options: []gerritpb.QueryOption{
			gerritpb.QueryOption_CURRENT_REVISION,
			gerritpb.QueryOption_DETAILED_LABELS,
			gerritpb.QueryOption_DETAILED_ACCOUNTS,
			gerritpb.QueryOption_MESSAGES,
		},
	})

	return ci, c.annotateGerritErr(ctx, err, "get")
}

func (c *change) recordVotesToRemove(label string, ci *gerritpb.ChangeInfo) {
	for _, vote := range ci.GetLabels()[label].GetAll() {
		if vote.GetValue() == 0 {
			continue
		}
		if c.votesToRemove == nil {
			c.votesToRemove = make(map[int64]map[string]int32, 1)
		}
		accountID := vote.GetUser().GetAccountId()
		if labels, exists := c.votesToRemove[accountID]; exists {
			labels[label] = 0
		} else {
			c.votesToRemove[accountID] = map[string]int32{label: 0}
		}
	}
}

func (c *change) sortedAccountIDs() []int64 {
	ids := make([]int64, 0, len(c.votesToRemove))
	for id := range c.votesToRemove {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (c *change) removeVotesAndPostMsg(ctx context.Context, ci *gerritpb.ChangeInfo, t *run.Trigger, msg string, notify Notify) error {
	accounts := c.sortedAccountIDs()
	var nonTriggeringVotesRemovalErrs errors.MultiError
	needRemoveTriggerVote := false
	for _, accountID := range accounts {
		if accountID == t.GetGerritAccountId() {
			needRemoveTriggerVote = true
			continue
		}
		if err := c.removeVote(ctx, accountID, "", gerritpb.Notify_NOTIFY_NONE, nil); err != nil {
			nonTriggeringVotesRemovalErrs = append(nonTriggeringVotesRemovalErrs, err)
		}
	}
	if err := common.MostSevereError(nonTriggeringVotesRemovalErrs); err != nil {
		// Return if failure occurs during removal of non-triggering votes so that
		// triggering votes will be kept. Otherwise, CV may create a new Run using
		// the non-triggering votes that CV fails to remove.
		return err
	}

	if needRemoveTriggerVote {
		// Remove the triggering vote last.
		if err := c.removeVote(ctx, t.GetGerritAccountId(), "", gerritpb.Notify_NOTIFY_NONE, nil); err != nil {
			return err
		}
	}

	n, nd := notify.toGerritNotify(accounts)
	return c.postGerritMsg(ctx, ci, msg, t, n, nd)
}

func (c *change) removeVote(ctx context.Context, accountID int64, msg string, n gerritpb.Notify, nd *gerritpb.NotifyDetails) error {
	_, err := c.gc.SetReview(ctx, &gerritpb.SetReviewRequest{
		Number:     c.Number,
		Project:    c.Project,
		RevisionId: c.Revision,
		Labels:     c.votesToRemove[accountID],
		Message:    gerrit.TruncateMessage(msg),
		// TODO(yiwzhang): implement subtag like "autogenerated:cv~dry-run" if
		// we want to display more than one message from CV in Gerrit UI.
		// Gerrit will show the latest message for each unique tag.
		Tag:           "autogenerated:cv",
		Notify:        n,
		NotifyDetails: nd,
		OnBehalfOf:    accountID,
	})
	return c.annotateGerritErr(ctx, err, "remove vote")
}

func (c *change) postCancelMessage(ctx context.Context, ci *gerritpb.ChangeInfo, msg string, t *run.Trigger, runCLExternalIDs []changelist.ExternalID, notify Notify) (err error) {
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
	n, nd := notify.toGerritNotify(c.sortedAccountIDs())
	return c.postGerritMsg(ctx, ci, msg, t, n, nd)
}

// postGerritMsg posts the given message to Gerrit.
//
// Skips if duplicate message is found after triggering time.
func (c *change) postGerritMsg(ctx context.Context, ci *gerritpb.ChangeInfo, msg string, t *run.Trigger, n gerritpb.Notify, nd *gerritpb.NotifyDetails) (err error) {
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
		Message:       gerrit.TruncateMessage(msg),
		Tag:           "autogenerated:cv",
		Notify:        n,
		NotifyDetails: nd,
	})
	return c.annotateGerritErr(ctx, err, "post message")
}

func (c *change) annotateGerritErr(ctx context.Context, err error, action string) error {
	switch grpcutil.Code(err) {
	case codes.OK:
		return nil
	case codes.PermissionDenied:
		return errors.Reason("no permission to %s %s/%d", action, c.Host, c.Number).Tag(ErrPermanentTag).Err()
	case codes.NotFound:
		return errors.Reason("change %s/%d not found", c.Host, c.Number).Tag(ErrPermanentTag).Err()
	case codes.FailedPrecondition:
		return errors.Reason("change %s/%d in an unexpected state for action %s: %s", c.Host, c.Number, action, err).Tag(ErrPermanentTag).Err()
	default:
		return gerrit.UnhandledError(ctx, err, "failed to %s %s/%d", action, c.Host, c.Number)
	}
}
