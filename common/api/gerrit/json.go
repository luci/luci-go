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
	"strconv"
	"strings"
	"time"

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

// TODO (throughout) use this to correctly parse AccountInfo response
// instead of gerritpb.AccountInfo
type accountInfo struct {
	Name            string   `json:"name,omitempty"`
	Email           string   `json:"email,omitempty"`
	SecondaryEmails []string `json:"secondary_emails,omitempty"`
	Username        string   `json:"username,omitempty"`
	AccountID       int64    `json:"_account_id,omitempty"`
}

func (a *accountInfo) ToProto() *gerritpb.AccountInfo {
	return &gerritpb.AccountInfo{
		Name:            a.Name,
		Email:           a.Email,
		SecondaryEmails: a.SecondaryEmails,
		Username:        a.Username,
		AccountId:       a.AccountID,
	}
}

type ownerInfo struct {
	Account accountInfo `json:"account,omitempty"`
}

type timestamp struct {
	time.Time
}

func (t *timestamp) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", t.Time.UTC().Format(gerritTimestampLayout))), nil
}

func (t *timestamp) UnmarshalJSON(b []byte) error {
	parsedTime, err := time.Parse(gerritTimestampLayout, strings.Trim(string(b), "\""))
	if err != nil {
		return errors.Annotate(err, "parse gerrit timestamp").Err()
	}
	t.Time = parsedTime
	return nil
}

// changeInfo represents JSON for a gerritpb.ChangeInfo on the wire.
type changeInfo struct {
	Number   int64                 `json:"_number"`
	Owner    *gerritpb.AccountInfo `json:"owner"`
	Project  string                `json:"project"`
	Branch   string                `json:"branch"`
	ChangeID string                `json:"change_id"`

	// json.Unmarshal cannot convert enum string to value,
	// so this field is handled specially in ToProto.
	Status string `json:"status"`

	CurrentRevision string                         `json:"current_revision"`
	Revisions       map[string]*revisionInfo       `json:"revisions"`
	Labels          map[string]*gerritpb.LabelInfo `json:"labels"`
	Messages        []changeMessageInfo            `json:"messages"`
	Created         timestamp                      `json:"created"`
	Updated         timestamp                      `json:"updated"`
	Submittable     bool                           `json:"submittable,omitempty"`

	// MoreChanges may be set on the last change in a response to a query for
	// changes, but this is not a property of the change itself and is not
	// needed in gerritpb.ChangeInfo.
	MoreChanges bool `json:"_more_changes"`
}

func (ci *changeInfo) ToProto() *gerritpb.ChangeInfo {
	ret := &gerritpb.ChangeInfo{
		Number:          ci.Number,
		Owner:           ci.Owner,
		Project:         ci.Project,
		Ref:             branchToRef(ci.Branch),
		Status:          gerritpb.ChangeStatus(gerritpb.ChangeStatus_value[ci.Status]),
		CurrentRevision: ci.CurrentRevision,
		Submittable:     ci.Submittable,
		Created:         timestamppb.New(ci.Created.Time),
		Updated:         timestamppb.New(ci.Updated.Time),
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
			ret.Labels[label] = info
		}
	}
	if ci.Messages != nil {
		ret.Messages = make([]*gerritpb.ChangeMessageInfo, len(ci.Messages))
		for i, msg := range ci.Messages {
			ret.Messages[i] = msg.ToProto()
		}
	}
	return ret
}

type changeMessageInfo struct {
	ID         string                `json:"id"`
	Author     *gerritpb.AccountInfo `json:"author"`
	RealAuthor *gerritpb.AccountInfo `json:"real_author"`
	Date       timestamp             `json:"date"`
	Message    string                `json:"message"`
}

func (cmi *changeMessageInfo) ToProto() *gerritpb.ChangeMessageInfo {
	if cmi == nil {
		return nil
	}
	return &gerritpb.ChangeMessageInfo{
		Id:         cmi.ID,
		Author:     cmi.Author,
		RealAuthor: cmi.RealAuthor,
		Date:       timestamppb.New(cmi.Date.Time),
		Message:    cmi.Message,
	}
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
	Number int                  `json:"_number"`
	Ref    string               `json:"ref"`
	Files  map[string]*fileInfo `json:"files"`
	Commit *commitInfo          `json:"commit"`
}

func (ri *revisionInfo) ToProto() *gerritpb.RevisionInfo {
	ret := &gerritpb.RevisionInfo{Number: int32(ri.Number), Ref: ri.Ref}
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

// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#commit-info
type commitInfo struct {
	Commit    string        `json:"commit"`
	Parents   []*commitInfo `json:"parents"`
	Author    AccountInfo   `json:"author"`
	Committer AccountInfo   `json:"committer"`
	Subject   string        `json:"subject"`
	Message   string        `json:"message"`
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
	Status          string     `json:"status"`
}

func (r *relatedChangeAndCommitInfo) ToProto() *gerritpb.GetRelatedChangesResponse_ChangeAndCommit {
	return &gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
		Project:         r.Project,
		Number:          r.Number,
		Patchset:        r.Patchset,
		CurrentPatchset: r.CurrentPatchset,
		Commit:          r.Commit.ToProto(),
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
		return nil, fmt.Errorf("no MergeableStrategy enum value for %q", strategyEnumName)
	}
	submitTypeEnumNum, found := gerritpb.MergeableInfo_SubmitType_value[mi.SubmitType]
	if !found {
		return nil, fmt.Errorf("no SubmitType enum value for %q", mi.SubmitType)
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
			return nil, errors.Annotate(err, "parsing approvals").Err()
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
			return nil, errors.Annotate(err, "converting reviewerInfo").Err()
		}
		reviewers = append(reviewers, rInfo)
	}
	ccs := make([]*gerritpb.ReviewerInfo, 0)
	for _, r := range rr.Ccs {
		rInfo, err := r.ToProtoReviewerInfo()
		if err != nil {
			return nil, errors.Annotate(err, "converting reviewerInfo").Err()
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

type attentionSetRequest struct {
	User   string `json:"user"`
	Reason string `json:"reason"`
	Notify string `json:"string,omitempty"`
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
		return nil, fmt.Errorf("no State enum value for %q", pi.State)
	}
	projectName, err := url.QueryUnescape(pi.ID)
	if err != nil {
		return nil, errors.Annotate(err, "decoding name").Err()
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
