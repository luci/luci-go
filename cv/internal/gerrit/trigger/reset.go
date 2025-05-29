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

package trigger

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
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
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/usertext"
)

// ErrResetPreconditionFailedTag is an error tag indicating that the
// precondition of resetting a trigger has not been met,
var ErrResetPreconditionFailedTag = errtag.Make("reset precondition not met", true)

// ErrResetPermanentTag is an error tag indicating that error occurs during the
// reset is permanent (e.g. lack of vote permission).
var ErrResetPermanentTag = errtag.Make("permanent error while resetting triggers", true)

var errGerritTag = errtag.Make("this is a Gerrit error: holds grpcCode", codes.Unknown)

// IsResetErrFromGerrit returns gerrit grpc error code if the `trigger.Reset`
// fails because of Gerrit.
func IsResetErrFromGerrit(err error) (codes.Code, bool) {
	switch v, ok := errGerritTag.Value(err); {
	case err == nil:
		return codes.OK, false
	case !ok:
		return codes.Unknown, false
	default:
		return v, true
	}
}

// ResetInput contains info to reset triggers of Run on a CL.
type ResetInput struct {
	// CL is a Gerrit CL entity.
	//
	// Must have CL.Snapshot set.
	CL *changelist.CL
	// Trigger identifies the triggering vote. Required.
	//
	// Removed only after all other votes on CQ label are removed.
	Triggers *run.Triggers
	// LUCIProject is the project that initiates this reset.
	//
	// The project scoped account of this LUCI project SHOULD have the permission
	// to set the CQ label on behalf of other users in Gerrit.
	LUCIProject string
	// Message to be posted along with the triggering vote removal
	Message string
	// Requester describes the caller (e.g. Project Manager, Run Manager).
	Requester string
	// Notify describes whom to notify regarding the reset.
	//
	// If empty, notifies no one.
	Notify gerrit.Whoms
	// AddToAttentionSet describes whom to add in the attention set.
	//
	// If empty, no change will be made to attention set.
	AddToAttentionSet gerrit.Whoms
	// AttentionReason describes the reason of the attention change.
	//
	// It is attached to the attention set change, and rendered in UI to explain
	// the reason of the attention to users.
	//
	// This is noop, if AddAttentionSet is empty.
	AttentionReason string
	// LeaseDuration is how long a lease will be held for this reset.
	//
	// If the passed context has a closer deadline, uses that deadline as lease
	// `ExpireTime`.
	LeaseDuration time.Duration
	// ConfigGroups are the ConfigGroups that are watching this CL.
	//
	// They are used to remove votes for additional modes. Normally, there is
	// just 1 ConfigGroup.
	ConfigGroups []*prjcfg.ConfigGroup
	// GFactory is used to create the gerrit client needed to perform the reset.
	GFactory gerrit.Factory
	// CLMutator performs mutations to the CL entity and notifies relevant parts
	// of CV when appropriate.
	CLMutator *changelist.Mutator
}

func (in *ResetInput) validateInput() error {
	// These are the conditions that shouldn't be met, and likely require code
	// changes for fixes.
	switch {
	case in.CL.Snapshot == nil:
		return fmt.Errorf("cl.Snapshot must be non-nil")
	case in.Triggers == nil:
		return fmt.Errorf("trigger must be non-nil")
	case in.Triggers.CqVoteTrigger == nil && in.Triggers.NewPatchsetRunTrigger == nil:
		return fmt.Errorf("at least one of {CqVoteTrigger, NewPatchsetRunTrigger} must be non-nil")
	case in.LUCIProject != in.CL.Snapshot.GetLuciProject():
		return fmt.Errorf("mismatched LUCI Project: got %q in input and %q in CL snapshot", in.LUCIProject, in.CL.Snapshot.GetLuciProject())
	case len(in.ConfigGroups) == 0:
		return fmt.Errorf("config_groups must be given")
	case in.GFactory == nil:
		return fmt.Errorf("gerrit factory must be non-nil")
	case in.CLMutator == nil:
		return fmt.Errorf("mutator must be non-nil")
	case len(in.Notify) > 0:
		for _, enum := range in.Notify {
			if _, ok := gerrit.Whom_name[int32(enum)]; !ok {
				return fmt.Errorf("notify: unknown Whom value %d", enum)
			}
		}
	case len(in.AddToAttentionSet) > 0:
		for _, enum := range in.AddToAttentionSet {
			if _, ok := gerrit.Whom_name[int32(enum)]; !ok {
				return fmt.Errorf("add_to_attention: unknown Whom value %d", enum)
			}
		}
	}
	return nil
}

// Reset removes or "deactivates" the trigger that made CV start processing the
// current run, whether by removing votes on a CL and posting the given message,
// or by updating the datastore entity associated with the CL; this, depending
// on the RunMode of the Run.
//
// For vote-removal-based reset:
//
// Returns error tagged with `ErrPreconditionFailedTag` if one of the
// following conditions is matched.
//   - The patchset of the provided CL is not the latest in Gerrit.
//   - The provided CL gets `changelist.AccessDenied` or
//     `changelist.AccessDeniedProbably` from Gerrit.
//
// Normally, the triggering vote(s) is removed last and all other votes
// are removed in chronological order (latest to earliest).
// After all votes are removed, the message is posted to Gerrit.
//
// Abnormally, e.g. lack of permission to remove votes, falls back to post a
// special message which "deactivates" the triggering votes. This special
// message is a combination of:
//   - the original message in the input
//   - reason for abnormality,
//   - special `botdata.BotData` which ensures CV won't consider previously
//     triggering votes as triggering in the future.
//
// Alternatively, in the case of a new patchset run:
//
// Updates the CLEntity to record that CV is not to create new patchset runs
// with the current patchset or lower. This prevents trigger.Find() from
// continuing to return a trigger for this patchset, analog to the effect of
// removing a cq vote on gerrit.
func Reset(ctx context.Context, in ResetInput) error {
	if err := in.validateInput(); err != nil {
		return err
	}
	if in.CL.AccessKindFromCodeReviewSite(ctx, in.LUCIProject) != changelist.AccessGranted {
		return ErrResetPreconditionFailedTag.Apply(errors.New("failed to reset trigger because CV lost access to this CL"))
	}
	if len(in.AddToAttentionSet) > 0 && in.AttentionReason == "" {
		logging.Warningf(ctx, "FIXME reset was given empty in AttentionReason.")
		in.AttentionReason = usertext.StoppedRun
	}

	client, err := in.GFactory.MakeClient(ctx, in.CL.Snapshot.GetGerrit().GetHost(), in.LUCIProject)
	if err != nil {
		return err
	}
	cl := in.CL
	if in.Triggers.GetNewPatchsetRunTrigger() != nil {
		cl, err = resetByUpdatingCLEntity(ctx, &in)
		if err != nil {
			return err
		}
	}
	if in.Message == "" && in.Triggers.GetCqVoteTrigger() == nil {
		return nil
	}

	leaseCtx, close, lErr := lease.ApplyOnCL(ctx, cl.ID, in.LeaseDuration, in.Requester)
	if lErr != nil {
		return lErr
	}
	defer close()

	switch {
	case in.Triggers.GetCqVoteTrigger() != nil:
		if err := ensurePSLatestInCV(ctx, cl); err != nil {
			return err
		}
		return resetLeased(leaseCtx, client, &in, cl)
	case in.Message != "":
		// If there is a CQ Vote trigger to purge, resetLeased() will have
		// taken care of posting any appropriate message.
		// If the Reset() call _only_ applies to an NPR trigger, _and_
		// a message has been specified, then this function needs to post a
		// message.
		return makeChange(client, &in, cl).postGerritMsg(
			leaseCtx, cl.Snapshot.GetGerrit().GetInfo(), in.Message, in.Triggers.NewPatchsetRunTrigger, in.Notify, in.AddToAttentionSet, in.AttentionReason)
	default:
		panic("unreachable")
	}
}

func makeChange(client gerrit.Client, in *ResetInput, cl *changelist.CL) *change {
	return &change{
		Host:        cl.Snapshot.GetGerrit().GetHost(),
		LUCIProject: in.LUCIProject,
		Project:     cl.Snapshot.GetGerrit().GetInfo().GetProject(),
		Number:      cl.Snapshot.GetGerrit().GetInfo().GetNumber(),
		Revision:    cl.Snapshot.GetGerrit().GetInfo().GetCurrentRevision(),
		gf:          in.GFactory,
		gc:          client,
	}
}

func resetByUpdatingCLEntity(ctx context.Context, in *ResetInput) (*changelist.CL, error) {
	return in.CLMutator.Update(ctx, in.LUCIProject, in.CL.ID, func(cl *changelist.CL) error {
		switch {
		case cl.TriggerNewPatchsetRunAfterPS < in.CL.Snapshot.GetPatchset():
			cl.TriggerNewPatchsetRunAfterPS = in.CL.Snapshot.GetPatchset()
			return nil
		default:
			logging.Warningf(ctx, "cl.TriggerNewPatchsetRunAfterPS has already been updated, race?")
			return changelist.ErrStopMutation
		}
	})
}

// TODO(tandrii): merge with prjmanager/purger's error messages.
var failMessage = "CV failed to unset the " + CQLabelName +
	" label on your behalf. Please unvote and revote on the " +
	CQLabelName + " label to retry."

func resetLeased(ctx context.Context, client gerrit.Client, in *ResetInput, cl *changelist.CL) error {
	c := makeChange(client, in, cl)
	logging.Infof(ctx, "Resetting triggers on %s/%d", c.Host, c.Number)
	ci, err := c.getLatest(ctx, cl.Snapshot.GetGerrit().GetInfo().GetUpdated().AsTime())
	switch {
	case err != nil:
		return err
	case ci.GetCurrentRevision() != c.Revision:
		return ErrResetPreconditionFailedTag.Apply(errors.Fmt("failed to reset because ps %d is not current for %s/%d", cl.Snapshot.GetPatchset(), c.Host, c.Number))
	}

	labelsToRemove := stringset.NewFromSlice(CQLabelName)
	if modeDef := in.Triggers.GetCqVoteTrigger().GetModeDefinition(); modeDef != nil {
		labelsToRemove.Add(modeDef.GetTriggeringLabel())
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

	removeErr := c.removeVotesAndPostMsg(ctx, ci, in.Triggers.GetCqVoteTrigger(), in.Message, in.Notify, in.AddToAttentionSet, in.AttentionReason)
	switch {
	case removeErr == nil:
		_, outErr := in.CLMutator.Update(ctx, in.LUCIProject, in.CL.ID, func(cl *changelist.CL) error {
			if cl.Snapshot == nil || cl.Snapshot.GetOutdated() != nil {
				return changelist.ErrStopMutation // noop
			}
			cl.Snapshot.Outdated = &changelist.Snapshot_Outdated{}
			return nil
		})
		if outErr != nil {
			// Let's log and ignore the error.
			logging.Errorf(ctx, "CLMutator.Update: ignoring %s", outErr)
		}
		return nil
	case !ErrResetPermanentTag.In(removeErr):
		return removeErr
	}

	// Received permanent error, try posting message.
	logging.Warningf(ctx, "Falling back to resetting via botdata message %s/%d", c.Host, c.Number)
	var msgBuilder strings.Builder
	if in.Message != "" {
		msgBuilder.WriteString(in.Message)
		msgBuilder.WriteString("\n\n")
	}
	msgBuilder.WriteString(failMessage)
	if err := c.postResetMessage(ctx, ci, msgBuilder.String(), in.Triggers.GetCqVoteTrigger(), in.Notify, in.AddToAttentionSet, in.AttentionReason); err != nil {
		// Return the original error, but add details from just posting a message.
		return errors.Fmt("even just posting message also failed: %w: %w", err, removeErr)
	}
	return nil
}

func ensurePSLatestInCV(ctx context.Context, cl *changelist.CL) error {
	curCLInCV := &changelist.CL{ID: cl.ID}
	switch err := datastore.Get(ctx, curCLInCV); {
	case err == datastore.ErrNoSuchEntity:
		return errors.Fmt("cl(id=%d) doesn't exist in datastore", cl.ID)
	case err != nil:
		return transient.Tag.Apply(errors.Fmt("failed to load cl: %d: %w", cl.ID, err))
	case curCLInCV.Snapshot.GetPatchset() > cl.Snapshot.GetPatchset():
		return ErrResetPreconditionFailedTag.Apply(errors.Fmt("failed to reset because ps %d is not current for cl(%d)", cl.Snapshot.GetPatchset(), cl.ID))
	}
	return nil
}

type change struct {
	Host        string
	LUCIProject string
	Project     string
	Number      int64
	Revision    string

	gf gerrit.Factory
	gc gerrit.Client
	// votesToRemove maps accountID to a set of labels.
	//
	// For ease of passing to SetReview API, each label maps to 0.
	votesToRemove map[int64]map[string]int32
}

func (c *change) getLatest(ctx context.Context, knownUpdatedTime time.Time) (*gerritpb.ChangeInfo, error) {
	var ci *gerritpb.ChangeInfo
	var gerritErr error
	outerErr := c.gf.MakeMirrorIterator(ctx).RetryIfStale(func(opt grpc.CallOption) error {
		ci, gerritErr = c.gc.GetChange(ctx, &gerritpb.GetChangeRequest{
			Number:  c.Number,
			Project: c.Project,
			Options: []gerritpb.QueryOption{
				gerritpb.QueryOption_CURRENT_REVISION,
				gerritpb.QueryOption_DETAILED_LABELS,
				gerritpb.QueryOption_DETAILED_ACCOUNTS,
				gerritpb.QueryOption_MESSAGES,
			},
		}, opt)
		switch {
		case grpcutil.Code(gerritErr) == codes.NotFound:
			// If a Run fails right after this CL is uploaded, it is possible that
			// CV receives NotFound when fetching this CL due to eventual consistency
			// of Gerrit. Therefore, consider this error as stale data. It is also
			// possible that user actually deleted this CL. In that case, it is also
			// okay to retry here because theoretically, Gerrit should consistently
			// return 404 and fail the task. When the task retries, CV should figure
			// that it has lost its access to this CL at the beginning and give up
			// resetting the trigger. But, even if Gerrit accidentally return
			// the deleted CL, the subsequent SetReview call will also fail the task.
			return gerrit.ErrStaleData
		case gerritErr != nil:
			return gerritErr
		case ci.GetUpdated().AsTime().Before(knownUpdatedTime):
			return gerrit.ErrStaleData
		}
		return nil
	})

	switch {
	case gerritErr != nil:
		return nil, c.annotateGerritErr(ctx, gerritErr, "get")
	case outerErr != nil:
		// Should never happen unless MirrorIterator itself errors out for some
		// reason.
		return nil, outerErr
	default:
		return ci, nil
	}
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

func (c *change) sortedVoterAccountIDs() []int64 {
	ids := make([]int64, 0, len(c.votesToRemove))
	for id := range c.votesToRemove {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (c *change) removeVotesAndPostMsg(ctx context.Context, ci *gerritpb.ChangeInfo, t *run.Trigger, msg string, notify, addAttn gerrit.Whoms, reason string) error {
	var nonTriggeringVotesRemovalErrs errors.MultiError
	needRemoveTriggerVote := false
	for _, voter := range c.sortedVoterAccountIDs() {
		if voter == t.GetGerritAccountId() {
			needRemoveTriggerVote = true
			continue
		}
		if err := c.removeVote(ctx, voter, run.Mode(t.GetMode())); err != nil {
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
		if err := c.removeVote(ctx, t.GetGerritAccountId(), run.Mode(t.GetMode())); err != nil {
			return err
		}
	}
	return c.postGerritMsg(ctx, ci, msg, t, notify, addAttn, reason)
}

func (c *change) removeVote(ctx context.Context, accountID int64, mode run.Mode) error {
	var gerritErr error

	outerErr := c.gf.MakeMirrorIterator(ctx).RetryIfStale(func(opt grpc.CallOption) error {
		_, gerritErr = c.gc.SetReview(ctx, &gerritpb.SetReviewRequest{
			Number:                           c.Number,
			Project:                          c.Project,
			RevisionId:                       c.Revision,
			Labels:                           c.votesToRemove[accountID],
			Tag:                              mode.GerritMessageTag(),
			Notify:                           gerritpb.Notify_NOTIFY_NONE,
			OnBehalfOf:                       accountID,
			IgnoreAutomaticAttentionSetRules: true,
		}, opt)
		switch grpcutil.Code(gerritErr) {
		case codes.NotFound:
			// This is known to happen on new CLs or on recently created revisions.
			return gerrit.ErrStaleData
		default:
			return gerritErr
		}
	})

	switch {
	case gerritErr != nil:
		return c.annotateGerritErr(ctx, gerritErr, "remove vote")
	case outerErr != nil:
		// Should never happen unless MirrorIterator itself errors out for some
		// reason.
		return outerErr
	default:
		return nil
	}
}

func (c *change) postResetMessage(ctx context.Context, ci *gerritpb.ChangeInfo, msg string, t *run.Trigger, notify, addAttn gerrit.Whoms, reason string) (err error) {
	bd := botdata.BotData{
		Action:      botdata.Cancel,
		TriggeredAt: t.GetTime().AsTime(),
		Revision:    c.Revision,
	}
	// TODO(crbug.com/1414898) - deprecate botdata
	if msg, err = botdata.Append(msg, bd); err != nil {
		return err
	}
	return c.postGerritMsg(ctx, ci, msg, t, notify, addAttn, reason)
}

// postGerritMsg posts the given message to Gerrit.
//
// Skips if duplicate message is found after triggering time.
func (c *change) postGerritMsg(ctx context.Context, ci *gerritpb.ChangeInfo, msg string, t *run.Trigger, notify, addAttn gerrit.Whoms, reason string) (err error) {
	for _, m := range ci.GetMessages() {
		switch {
		case m.GetDate().AsTime().Before(t.GetTime().AsTime()):
		case strings.Contains(m.Message, strings.TrimSpace(msg)):
			return nil
		}
	}
	nd := makeGerritNotifyDetails(notify, ci)
	reason = fmt.Sprintf("ps#%d: %s", ci.GetRevisions()[ci.GetCurrentRevision()].GetNumber(), reason)
	attention := makeGerritAttentionSetInputs(addAttn, ci, reason)
	msg = gerrit.TruncateMessage(msg)
	// Post message with unique tag per Run so that Gerrit will always display
	// these messages. The uniqueness is achieved by appending the Run triggering
	// time. Otherwise, users may falsely believe LUCI CV is not doing anything
	// to handle their CLs because Gerrit will hide old messages with the same
	// tag (See: crbug.com/1359521). The message in trigger reset normally
	// contains the result for the Run (e.g. passing or why the Run fails) so it
	// is a good indication of LUCI CV is working fine without introducing too
	// much noise.
	tag := fmt.Sprintf("%s:%d", run.Mode(t.Mode).GerritMessageTag(), t.GetTime().AsTime().Unix())
	var gerritErr error
	outerErr := c.gf.MakeMirrorIterator(ctx).RetryIfStale(func(opt grpc.CallOption) error {
		_, gerritErr = c.gc.SetReview(ctx, &gerritpb.SetReviewRequest{
			Number:     c.Number,
			Project:    c.Project,
			RevisionId: ci.GetCurrentRevision(),
			Message:    msg,
			Tag:        tag,
			// Set `Notify` to NONE because LUCI CV has the knowledge on all the
			// accounts to notify. All of them are included through `NotifyDetails`.
			// Therefore, there is no point using the special enum provided via
			// `Notify`.
			Notify:            gerritpb.Notify_NOTIFY_NONE,
			NotifyDetails:     nd,
			AddToAttentionSet: attention,
		}, opt)
		switch grpcutil.Code(gerritErr) {
		case codes.NotFound:
			// This is known to happen on new CLs or on recently created revisions.
			return gerrit.ErrStaleData
		default:
			return gerritErr
		}
	})

	switch {
	case gerritErr != nil:
		return c.annotateGerritErr(ctx, gerritErr, "post message")
	case outerErr != nil:
		// Should never happen unless MirrorIterator itself errors out for some
		// reason.
		return outerErr
	default:
		return nil
	}
}

func makeGerritNotifyDetails(notify gerrit.Whoms, ci *gerritpb.ChangeInfo) *gerritpb.NotifyDetails {
	accounts := notify.ToAccountIDsSorted(ci)
	if len(accounts) == 0 {
		return nil
	}

	return &gerritpb.NotifyDetails{
		Recipients: []*gerritpb.NotifyDetails_Recipient{
			{
				RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
				Info: &gerritpb.NotifyDetails_Info{
					Accounts: accounts,
				},
			},
		},
	}
}

func makeGerritAttentionSetInputs(addAttn gerrit.Whoms, ci *gerritpb.ChangeInfo, reason string) []*gerritpb.AttentionSetInput {
	accounts := addAttn.ToAccountIDsSorted(ci)
	if len(accounts) == 0 {
		return nil
	}
	ret := make([]*gerritpb.AttentionSetInput, len(accounts))
	for i, acct := range accounts {
		ret[i] = &gerritpb.AttentionSetInput{
			// The accountID supports various formats, including a bare account ID,
			// email, full-name, and others.
			User:   strconv.Itoa(int(acct)),
			Reason: reason,
		}
	}
	return ret
}

func (c *change) annotateGerritErr(ctx context.Context, err error, action string) error {
	if err == nil {
		return nil
	}
	code := grpcutil.Code(err)
	var retErr error
	switch code {
	case codes.OK:
		return nil
	case codes.PermissionDenied:
		retErr = ErrResetPermanentTag.Apply(errors.Fmt("no permission to %s %s/%d", action, c.Host, c.Number))
	case codes.NotFound:
		retErr = ErrResetPermanentTag.Apply(errors.Fmt("change %s/%d not found", c.Host, c.Number))
	case codes.FailedPrecondition:
		retErr = ErrResetPermanentTag.Apply(errors.Fmt("change %s/%d in an unexpected state for action %s: %s", c.Host, c.Number, action, err))
	case codes.DeadlineExceeded:
		retErr = transient.Tag.Apply(errors.Fmt("timeout when calling Gerrit to %s %s/%d", action, c.Host, c.Number))
	default:
		retErr = gerrit.UnhandledError(ctx, err, "failed to %s %s/%d", action, c.Host, c.Number)
	}
	return errGerritTag.ApplyValue(retErr, code)
}
