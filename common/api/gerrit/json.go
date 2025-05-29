// Copyright 2020 The LUCI Authors.
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

package gerrit

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// This file contains code related to JSON representations of messages that are
// used for requests to the Gerrit REST API, and unmarshalling code to convert
// from the JSON representations to protos defined in `gerritpb`.
//
// Each of these structs corresponds to an entity described at
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#json-entities
// and also to a message in `gerritpb`, and each has a unmarshalling method
// called ToProto.

// timestamp implements customized JSON marshal/unmarshal behavior that matches
// the timestamp format used in Gerrit.

type accountInfo struct {
	Name            string   `json:"name,omitempty"`
	Email           string   `json:"email,omitempty"`
	SecondaryEmails []string `json:"secondary_emails,omitempty"`
	Username        string   `json:"username,omitempty"`
	AccountID       int64    `json:"_account_id,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

func (a *accountInfo) ToProto() *gerritpb.AccountInfo {
	if a == nil {
		return nil
	}
	return &gerritpb.AccountInfo{
		Name:            a.Name,
		Email:           a.Email,
		SecondaryEmails: a.SecondaryEmails,
		Username:        a.Username,
		AccountId:       a.AccountID,
		Tags:            a.Tags,
	}
}

type ownerInfo struct {
	Account accountInfo `json:"account,omitempty"`
}

// changeInfo represents JSON for a gerritpb.ChangeInfo on the wire.
type changeInfo struct {
	Number    int64                     `json:"_number"`
	Owner     *accountInfo              `json:"owner"`
	Project   string                    `json:"project"`
	Branch    string                    `json:"branch"`
	ChangeID  string                    `json:"change_id"`
	Reviewers map[string][]*accountInfo `json:"reviewers"`
	Hashtags  []string                  `json:"hashtags"`
	Subject   string                    `json:"subject"`

	// json.Unmarshal cannot convert enum string to value,
	// so this field is handled specially in ToProto.
	Status string `json:"status"`

	CurrentRevision    string                         `json:"current_revision"`
	Revisions          map[string]*revisionInfo       `json:"revisions"`
	Labels             map[string]*labelInfo          `json:"labels"`
	Messages           []changeMessageInfo            `json:"messages"`
	Requirements       []requirement                  `json:"requirements"`
	SubmitRequirements []*submitRequirementResultInfo `json:"submit_requirements"`

	Created     Timestamp `json:"created"`
	Updated     Timestamp `json:"updated"`
	Submitted   Timestamp `json:"submitted"`
	Submittable bool      `json:"submittable,omitempty"`
	IsPrivate   bool      `json:"is_private,omitempty"`
	MetaRevID   string    `json:"meta_rev_id,omitempty"`

	RevertOf           int64 `json:"revert_of,omitempty"`
	CherryPickOfChange int64 `json:"cherry_pick_of_change,omitempty"`

	// MoreChanges may be set on the last change in a response to a query for
	// changes, but this is not a property of the change itself and is not
	// needed in gerritpb.ChangeInfo.
	MoreChanges bool `json:"_more_changes"`
}

func (ci *changeInfo) ToProto() (*gerritpb.ChangeInfo, error) {
	ret := &gerritpb.ChangeInfo{
		Number:             ci.Number,
		Owner:              ci.Owner.ToProto(),
		Project:            ci.Project,
		Ref:                branchToRef(ci.Branch),
		Subject:            ci.Subject,
		Status:             gerritpb.ChangeStatus(gerritpb.ChangeStatus_value[ci.Status]),
		Hashtags:           ci.Hashtags,
		CurrentRevision:    ci.CurrentRevision,
		Submittable:        ci.Submittable,
		IsPrivate:          ci.IsPrivate,
		MetaRevId:          ci.MetaRevID,
		Created:            timestamppb.New(ci.Created.Time),
		Updated:            timestamppb.New(ci.Updated.Time),
		Submitted:          timestamppb.New(ci.Submitted.Time),
		RevertOf:           ci.RevertOf,
		CherryPickOfChange: ci.CherryPickOfChange,
		Branch:             ci.Branch,
	}
	if ci.Revisions != nil {
		ret.Revisions = make(map[string]*gerritpb.RevisionInfo, len(ci.Revisions))
		for rev, info := range ci.Revisions {
			ret.Revisions[rev] = info.ToProto()
		}
	}
	if ci.Labels != nil {
		ret.Labels = make(map[string]*gerritpb.LabelInfo, len(ci.Labels))
		for label, info := range ci.Labels {
			ret.Labels[label] = info.ToProto()
		}
	}
	if ci.Messages != nil {
		ret.Messages = make([]*gerritpb.ChangeMessageInfo, len(ci.Messages))
		for i, msg := range ci.Messages {
			ret.Messages[i] = msg.ToProto()
		}
	}
	var err error
	if ci.Requirements != nil {
		ret.Requirements = make([]*gerritpb.Requirement, len(ci.Requirements))
		for i, r := range ci.Requirements {
			if ret.Requirements[i], err = r.ToProto(); err != nil {
				return nil, err
			}
		}
	}
	if ci.SubmitRequirements != nil {
		ret.SubmitRequirements = make([]*gerritpb.SubmitRequirementResultInfo,
			len(ci.SubmitRequirements))
		for i, r := range ci.SubmitRequirements {
			if ret.SubmitRequirements[i], err = r.ToProto(); err != nil {
				return nil, err
			}
		}
	}
	if ci.Reviewers != nil {
		ret.Reviewers = &gerritpb.ReviewerStatusMap{}
		if accs, exist := ci.Reviewers["REVIEWER"]; exist {
			ret.Reviewers.Reviewers = make([]*gerritpb.AccountInfo, len(accs))
			for i, acc := range accs {
				ret.Reviewers.Reviewers[i] = acc.ToProto()
			}
		}
		if accs, exist := ci.Reviewers["CC"]; exist {
			ret.Reviewers.Ccs = make([]*gerritpb.AccountInfo, len(accs))
			for i, acc := range accs {
				ret.Reviewers.Ccs[i] = acc.ToProto()
			}
		}
		if accs, exist := ci.Reviewers["REMOVED"]; exist {
			ret.Reviewers.Removed = make([]*gerritpb.AccountInfo, len(accs))
			for i, acc := range accs {
				ret.Reviewers.Ccs[i] = acc.ToProto()
			}
		}
	}

	return ret, nil
}

type labelInfo struct {
	Optional     bool              `json:"optional"`
	Approved     *accountInfo      `json:"approved"`
	Rejected     *accountInfo      `json:"rejected"`
	Recommended  *accountInfo      `json:"recommended"`
	Disliked     *accountInfo      `json:"disliked"`
	Blocking     bool              `json:"blocking"`
	Value        int32             `json:"value"`
	DefaultValue int32             `json:"default_value"`
	All          []*approvalInfo   `json:"all"`
	Values       map[string]string `json:"values"`
}

func (li *labelInfo) ToProto() *gerritpb.LabelInfo {
	ret := &gerritpb.LabelInfo{
		Optional:     li.Optional,
		Approved:     li.Approved.ToProto(),
		Rejected:     li.Rejected.ToProto(),
		Recommended:  li.Recommended.ToProto(),
		Disliked:     li.Disliked.ToProto(),
		Blocking:     li.Blocking,
		Value:        li.Value,
		DefaultValue: li.DefaultValue,
	}
	if len(li.All) > 0 {
		ret.All = make([]*gerritpb.ApprovalInfo, len(li.All))
		for i, a := range li.All {
			ret.All[i] = a.ToProto()
		}
	}
	if li.Values != nil {
		ret.Values = make(map[int32]string, len(li.Values))
		for value, description := range li.Values {
			i, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
			// Error is silently ignored for consistency with other parts of code.
			if err == nil {
				ret.Values[int32(i)] = description
			}
		}
	}
	return ret
}

type approvalInfo struct {
	accountInfo
	Value                int32                     `json:"value"`
	PermittedVotingRange *gerritpb.VotingRangeInfo `json:"permitted_voting_range"`
	Date                 Timestamp                 `json:"date"`
	Tag                  string                    `json:"tag"`
	PostSubmit           bool                      `json:"post_submit"`
}

func (ai *approvalInfo) ToProto() *gerritpb.ApprovalInfo {
	ret := &gerritpb.ApprovalInfo{
		User:                 ai.accountInfo.ToProto(),
		Value:                ai.Value,
		PermittedVotingRange: ai.PermittedVotingRange,
		Date:                 timestamppb.New(ai.Date.Time),
		Tag:                  ai.Tag,
		PostSubmit:           ai.PostSubmit,
	}
	return ret
}

type changeMessageInfo struct {
	ID         string       `json:"id"`
	Author     *accountInfo `json:"author"`
	RealAuthor *accountInfo `json:"real_author"`
	Date       Timestamp    `json:"date"`
	Message    string       `json:"message"`
	Tag        string       `json:"tag"`
}

func (cmi *changeMessageInfo) ToProto() *gerritpb.ChangeMessageInfo {
	if cmi == nil {
		return nil
	}
	return &gerritpb.ChangeMessageInfo{
		Id:         cmi.ID,
		Author:     cmi.Author.ToProto(),
		RealAuthor: cmi.RealAuthor.ToProto(),
		Date:       timestamppb.New(cmi.Date.Time),
		Message:    cmi.Message,
		Tag:        cmi.Tag,
	}
}

type requirement struct {
	Status       string `json:"status"`
	FallbackText string `json:"fallback_text"`
	Type         string `json:"type"`
}

func (r *requirement) ToProto() (*gerritpb.Requirement, error) {
	stringVal := "REQUIREMENT_STATUS_" + r.Status
	numVal, found := gerritpb.Requirement_Status_value[stringVal]
	if !found {
		return nil, errors.Fmt("no Status enum value for %q", r.Status)
	}
	return &gerritpb.Requirement{
		Status:       gerritpb.Requirement_Status(numVal),
		FallbackText: r.FallbackText,
		Type:         r.Type,
	}, nil
}

type fileInfo struct {
	LinesInserted int32 `json:"lines_inserted"`
	LinesDeleted  int32 `json:"lines_deleted"`
	SizeDelta     int64 `json:"size_delta"`
	Size          int64 `json:"size"`
}

func (fi *fileInfo) ToProto() *gerritpb.FileInfo {
	return &gerritpb.FileInfo{
		LinesInserted: fi.LinesInserted,
		LinesDeleted:  fi.LinesDeleted,
		SizeDelta:     fi.SizeDelta,
		Size:          fi.Size,
	}
}

type revisionInfo struct {
	Kind        string               `json:"kind"`
	Number      int                  `json:"_number"`
	Uploader    *accountInfo         `json:"uploader"`
	Ref         string               `json:"ref"`
	Created     Timestamp            `json:"created"`
	Description string               `json:"description"`
	Files       map[string]*fileInfo `json:"files"`
	Commit      *commitInfo          `json:"commit"`
}

func (ri *revisionInfo) ToProto() *gerritpb.RevisionInfo {
	ret := &gerritpb.RevisionInfo{
		Number:      int32(ri.Number),
		Uploader:    ri.Uploader.ToProto(),
		Ref:         ri.Ref,
		Created:     timestamppb.New(ri.Created.Time),
		Description: ri.Description,
	}
	if v, ok := gerritpb.RevisionInfo_Kind_value[ri.Kind]; ok {
		ret.Kind = gerritpb.RevisionInfo_Kind(v)
	}
	if ri.Files != nil {
		ret.Files = make(map[string]*gerritpb.FileInfo, len(ri.Files))
		for i, fi := range ri.Files {
			ret.Files[i] = fi.ToProto()
		}
	}
	if ri.Commit != nil {
		ret.Commit = ri.Commit.ToProto()
	}
	return ret
}

// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#git-person-info
type gitPersonInfo struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (g *gitPersonInfo) ToProto() *gerritpb.GitPersonInfo {
	return &gerritpb.GitPersonInfo{
		Name:  g.Name,
		Email: g.Email,
	}
}

// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#commit-info
type commitInfo struct {
	Commit    string         `json:"commit"`
	Parents   []*commitInfo  `json:"parents"`
	Author    *gitPersonInfo `json:"author"`
	Committer *gitPersonInfo `json:"committer"`
	Subject   string         `json:"subject"`
	Message   string         `json:"message"`
}

func (c *commitInfo) ToProto() *gerritpb.CommitInfo {
	parents := make([]*gerritpb.CommitInfo_Parent, len(c.Parents))
	for i, p := range c.Parents {
		parents[i] = &gerritpb.CommitInfo_Parent{Id: p.Commit}
	}
	return &gerritpb.CommitInfo{
		Id:      c.Commit,
		Parents: parents,
		Message: c.Message,
		Author:  c.Author.ToProto(),
		// TODO(tandrii): support other fields once added.
	}
}

// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#related-change-and-commit-info
type relatedChangeAndCommitInfo struct {
	Project         string     `json:"project"`
	ChangeID        string     `json:"change_id"`
	Commit          commitInfo `json:"commit"`
	Number          int64      `json:"_change_number"`
	Patchset        int64      `json:"_revision_number"`
	CurrentPatchset int64      `json:"_current_revision_number"`

	// json.Unmarshal cannot convert enum string to value,
	// so this field is handled specially in ToProto.
	Status string `json:"status"`
}

func (r *relatedChangeAndCommitInfo) ToProto() *gerritpb.GetRelatedChangesResponse_ChangeAndCommit {
	return &gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
		Project:         r.Project,
		Number:          r.Number,
		Patchset:        r.Patchset,
		CurrentPatchset: r.CurrentPatchset,
		Commit:          r.Commit.ToProto(),
		Status:          gerritpb.ChangeStatus(gerritpb.ChangeStatus_value[r.Status]),
	}
}

type mergeableInfo struct {
	SubmitType    string   `json:"submit_type"`
	Strategy      string   `json:"strategy"`
	Mergeable     bool     `json:"mergeable"`
	CommitMerged  bool     `json:"commit_merged"`
	ContentMerged bool     `json:"content_merged"`
	Conflicts     []string `json:"conflicts"`
	MergeableInto []string `json:"mergeable_into"`
}

func (mi *mergeableInfo) ToProto() (*gerritpb.MergeableInfo, error) {
	// Convert something like 'simple-two-way-in-core' to 'SIMPLE_TWO_WAY_IN_CORE'.
	strategyEnumName := strings.Replace(strings.ToUpper(mi.Strategy), "-", "_", -1)
	strategyEnumNum, found := gerritpb.MergeableStrategy_value[strategyEnumName]
	if !found {
		return nil, errors.Fmt("no MergeableStrategy enum value for %q", strategyEnumName)
	}
	submitTypeEnumNum, found := gerritpb.MergeableInfo_SubmitType_value[mi.SubmitType]
	if !found {
		return nil, errors.Fmt("no SubmitType enum value for %q", mi.SubmitType)
	}
	return &gerritpb.MergeableInfo{
		SubmitType:    gerritpb.MergeableInfo_SubmitType(submitTypeEnumNum),
		Strategy:      gerritpb.MergeableStrategy(strategyEnumNum),
		Mergeable:     mi.Mergeable,
		CommitMerged:  mi.CommitMerged,
		ContentMerged: mi.ContentMerged,
		Conflicts:     mi.Conflicts,
		MergeableInto: mi.MergeableInto,
	}, nil
}

type addReviewerRequest struct {
	Reviewer  string `json:"reviewer"`
	State     string `json:"state,omitempty"`
	Confirmed bool   `json:"confirmed,omitempty"`
	Notify    string `json:"notify,omitempty"`
}

type reviewerInfo struct {
	Name            string            `json:"name,omitempty"`
	Email           string            `json:"email,omitempty"`
	SecondaryEmails []string          `json:"secondary_emails,omitempty"`
	Username        string            `json:"username,omitempty"`
	Approvals       map[string]string `json:"approvals,omitempty"`
	AccountID       int64             `json:"_account_id,omitempty"`
}

func (ri *reviewerInfo) ToProtoReviewerInfo() (*gerritpb.ReviewerInfo, error) {
	approvals := make(map[string]int32, 0)
	for label, score := range ri.Approvals {
		score = strings.TrimLeft(score, " ")
		scoreInt, err := strconv.ParseInt(score, 10, 32)
		if err != nil {
			return nil, errors.Fmt("parsing approvals: %w", err)
		}
		approvals[label] = int32(scoreInt)
	}
	return &gerritpb.ReviewerInfo{
		Account: &gerritpb.AccountInfo{
			Name:            ri.Name,
			Email:           ri.Email,
			SecondaryEmails: ri.SecondaryEmails,
			Username:        ri.Username,
			AccountId:       ri.AccountID,
		},
		Approvals: approvals,
	}, nil
}

type addReviewerResult struct {
	Input     string         `json:"input"`
	Reviewers []reviewerInfo `json:"reviewers,omitempty"`
	Ccs       []reviewerInfo `json:"ccs,omitempty"`
	Error     string         `json:"error,omitempty"`
	Confirm   bool           `json:"confirm,omitempty"`
}

func (rr *addReviewerResult) ToProto() (*gerritpb.AddReviewerResult, error) {
	reviewers := make([]*gerritpb.ReviewerInfo, 0)
	for _, r := range rr.Reviewers {
		rInfo, err := r.ToProtoReviewerInfo()
		if err != nil {
			return nil, errors.Fmt("converting reviewerInfo: %w", err)
		}
		reviewers = append(reviewers, rInfo)
	}
	ccs := make([]*gerritpb.ReviewerInfo, 0)
	for _, r := range rr.Ccs {
		rInfo, err := r.ToProtoReviewerInfo()
		if err != nil {
			return nil, errors.Fmt("converting reviewerInfo: %w", err)
		}
		ccs = append(ccs, rInfo)
	}
	return &gerritpb.AddReviewerResult{
		Input:     rr.Input,
		Reviewers: reviewers,
		Ccs:       ccs,
		Error:     rr.Error,
		Confirm:   rr.Confirm,
	}, nil
}

func enumToString(v int32, m map[int32]string) string {
	if v == 0 {
		return ""
	}
	prefixLen := strings.LastIndex(m[0], "UNSPECIFIED")
	return m[v][prefixLen:]
}

type reviewInput struct {
	Message                          string               `json:"message,omitempty"`
	Labels                           map[string]int32     `json:"labels,omitempty"`
	Tag                              string               `json:"tag,omitempty"`
	Notify                           string               `json:"notify,omitempty"`
	NotifyDetails                    notifyDetails        `json:"notify_details,omitempty"`
	OnBehalfOf                       int64                `json:"on_behalf_of,omitempty"`
	Ready                            bool                 `json:"ready,omitempty"`
	WorkInProgress                   bool                 `json:"work_in_progress,omitempty"`
	AddToAttentionSet                []*attentionSetInput `json:"add_to_attention_set,omitempty"`
	RemoveFromAttentionSet           []*attentionSetInput `json:"remove_from_attention_set,omitempty"`
	IgnoreAutomaticAttentionSetRules bool                 `json:"ignore_automatic_attention_set_rules,omitempty"`
	Reviewers                        []*reviewerInput     `json:"reviewers,omitempty"`
}

type notifyInfo struct {
	Accounts []int64 `json:"accounts,omitempty"`
}

type notifyDetails map[string]*notifyInfo

func toNotifyDetails(in *gerritpb.NotifyDetails) notifyDetails {
	recipients := in.GetRecipients()
	if len(recipients) == 0 {
		return nil
	}
	res := make(map[string]*notifyInfo, len(recipients))
	for _, recipient := range recipients {
		if len(recipient.Info.GetAccounts()) == 0 {
			continue
		}
		rt := recipient.RecipientType
		if rt == gerritpb.NotifyDetails_RECIPIENT_TYPE_UNSPECIFIED {
			// Must have been caught in validation.
			panic(fmt.Errorf("must specify recipient type"))
		}
		rts := enumToString(int32(rt.Number()), gerritpb.NotifyDetails_RecipientType_name)
		if ni, ok := res[rts]; !ok {
			ni = &notifyInfo{
				Accounts: make([]int64, len(recipient.Info.GetAccounts())),
			}
			for i, aid := range recipient.Info.GetAccounts() {
				ni.Accounts[i] = aid
			}
			res[rts] = ni
		} else {
			ni.Accounts = append(ni.Accounts, recipient.Info.GetAccounts()...)
		}
	}

	for _, ni := range res {
		// Sort & dedup accounts in each notification bucket.
		sort.Slice(ni.Accounts, func(i, j int) bool { return ni.Accounts[i] < ni.Accounts[j] })
		n := 0
		for i := 1; i < len(ni.Accounts); i++ {
			if ni.Accounts[n] == ni.Accounts[i] {
				continue
			}
			n++
			ni.Accounts[n] = ni.Accounts[i]
		}
		ni.Accounts = ni.Accounts[:n+1]
	}
	return res
}

type attentionSetInput struct {
	User          string        `json:"user"`
	Reason        string        `json:"reason"`
	Notify        string        `json:"string,omitempty"`
	NotifyDetails notifyDetails `json:"notify_details,omitempty"`
}

func toAttentionSetInput(in *gerritpb.AttentionSetInput) *attentionSetInput {
	return &attentionSetInput{
		User:          in.User,
		Reason:        in.Reason,
		Notify:        enumToString(int32(in.Notify.Number()), gerritpb.Notify_name),
		NotifyDetails: toNotifyDetails(in.NotifyDetails),
	}
}

func toAttentionSetInputs(in []*gerritpb.AttentionSetInput) []*attentionSetInput {
	if len(in) == 0 {
		return nil
	}
	out := make([]*attentionSetInput, len(in))
	for i, x := range in {
		out[i] = toAttentionSetInput(x)
	}
	return out
}

type reviewerInput struct {
	Reviewer string `json:"reviewer"`
	State    string `json:"state,omitempty"`
}

func toReviewerInputs(in []*gerritpb.ReviewerInput) []*reviewerInput {
	if len(in) == 0 {
		return nil
	}

	out := make([]*reviewerInput, len(in))
	for i, x := range in {
		out[i] = &reviewerInput{
			Reviewer: x.Reviewer,
			State:    enumToString(int32(x.State.Number()), gerritpb.ReviewerInput_State_name),
		}
	}
	return out
}

type reviewResult struct {
	Labels    map[string]int32              `json:"labels,omitempty"`
	Reviewers map[string]*addReviewerResult `json:"reviewers,omitempty"`
}

func (rr *reviewResult) ToProto() (*gerritpb.ReviewResult, error) {
	result := &gerritpb.ReviewResult{
		Labels: rr.Labels,
	}
	if len(rr.Reviewers) == 0 {
		return result, nil
	}

	reviewers := make(map[string]*gerritpb.AddReviewerResult, len(rr.Reviewers))
	for i, x := range rr.Reviewers {
		reviewerDetails, err := x.ToProto()
		if err != nil {
			return nil, err
		}
		reviewers[i] = reviewerDetails
	}
	result.Reviewers = reviewers

	return result, nil
}

type projectInfo struct {
	ID          string                  `json:"id,omitempty"`
	Parent      string                  `json:"parent,omitempty"`
	Description string                  `json:"description,omitempty"`
	State       string                  `json:"state,omitempty"`
	Branches    map[string]string       `json:"branches,omitempty"`
	WebLinks    []*gerritpb.WebLinkInfo `json:"web_links,omitempty"`
}

func (pi *projectInfo) ToProto() (*gerritpb.ProjectInfo, error) {
	stateEnumVal := "PROJECT_STATE_" + pi.State
	stateEnumNum, found := gerritpb.ProjectInfo_State_value[stateEnumVal]
	if !found {
		return nil, errors.Fmt("no State enum value for %q", pi.State)
	}
	projectName, err := url.QueryUnescape(pi.ID)
	if err != nil {
		return nil, errors.Fmt("decoding name: %w", err)
	}
	absoluteRefs := make(map[string]string, len(pi.Branches))
	for ref, sha1 := range pi.Branches {
		absoluteRefs[branchToRef(ref)] = sha1
	}
	return &gerritpb.ProjectInfo{
		Name:        projectName,
		Parent:      pi.Parent,
		Description: pi.Description,
		State:       gerritpb.ProjectInfo_State(stateEnumNum),
		Refs:        absoluteRefs,
		WebLinks:    pi.WebLinks,
	}, nil
}

// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#submit-info
type submitInfo struct {
	Status string `json:"status"`
}

func (si *submitInfo) ToProto() *gerritpb.SubmitInfo {
	return &gerritpb.SubmitInfo{
		Status: gerritpb.ChangeStatus(gerritpb.ChangeStatus_value[si.Status]),
	}
}

type submitRequirementResultInfo struct {
	Name                           string                           `json:"name"`
	Description                    string                           `json:"description"`
	Status                         string                           `json:"status"`
	IsLegacy                       bool                             `json:"is_legacy"`
	ApplicabilityExpressionResult  *submitRequirementExpressionInfo `json:"applicability_expression_result"`
	SubmittabilityExpressionResult *submitRequirementExpressionInfo `json:"submittability_expression_result"`
	OverrideExpressionResult       *submitRequirementExpressionInfo `json:"override_expression_result"`
}

func (ri *submitRequirementResultInfo) ToProto() (*gerritpb.SubmitRequirementResultInfo, error) {
	numVal, found := gerritpb.SubmitRequirementResultInfo_Status_value[ri.Status]
	if !found {
		return nil, errors.Fmt("no Status enum value for %q", ri.Status)
	}
	return &gerritpb.SubmitRequirementResultInfo{
		Name:                           ri.Name,
		Description:                    ri.Description,
		Status:                         gerritpb.SubmitRequirementResultInfo_Status(numVal),
		IsLegacy:                       ri.IsLegacy,
		ApplicabilityExpressionResult:  ri.ApplicabilityExpressionResult.ToProto(),
		SubmittabilityExpressionResult: ri.SubmittabilityExpressionResult.ToProto(),
		OverrideExpressionResult:       ri.OverrideExpressionResult.ToProto(),
	}, nil
}

type submitRequirementExpressionInfo struct {
	Expression   string   `json:"expression"`
	Fulfilled    bool     `json:"fulfilled"`
	PassingAtoms []string `json:"passing_atoms"`
	FailingAtoms []string `json:"failing_atoms"`
	ErrorMessage string   `json:"error_message"`
}

func (ei *submitRequirementExpressionInfo) ToProto() *gerritpb.SubmitRequirementExpressionInfo {
	if ei == nil {
		return nil
	}
	return &gerritpb.SubmitRequirementExpressionInfo{
		Expression:   ei.Expression,
		Fulfilled:    ei.Fulfilled,
		PassingAtoms: ei.PassingAtoms,
		FailingAtoms: ei.FailingAtoms,
		ErrorMessage: ei.ErrorMessage,
	}
}

type metaDiff struct {
	Added         *changeInfo `json:"added"`
	Removed       *changeInfo `json:"removed"`
	OldChangeInfo *changeInfo `json:"old_change_info"`
	NewChangeInfo *changeInfo `json:"new_change_info"`
}

func (md *metaDiff) ToProto() (*gerritpb.MetaDiff, error) {
	var resp gerritpb.MetaDiff
	var ci *gerritpb.ChangeInfo
	var err error

	if ci, err = md.Added.ToProto(); err != nil {
		return nil, err
	}
	resp.Added = ci
	if ci, err = md.Removed.ToProto(); err != nil {
		return nil, err
	}
	resp.Removed = ci
	if ci, err = md.OldChangeInfo.ToProto(); err != nil {
		return nil, err
	}
	resp.OldChangeInfo = ci
	if ci, err = md.NewChangeInfo.ToProto(); err != nil {
		return nil, err
	}
	resp.NewChangeInfo = ci
	return &resp, nil
}
