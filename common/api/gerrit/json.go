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
		Status:          gerritpb.ChangeInfo_Status(gerritpb.ChangeInfo_Status_value[ci.Status]),
		CurrentRevision: ci.CurrentRevision,
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
}

func (ri *revisionInfo) ToProto() *gerritpb.RevisionInfo {
	ret := &gerritpb.RevisionInfo{Number: int32(ri.Number), Ref: ri.Ref}
	if ri.Files != nil {
		ret.Files = make(map[string]*gerritpb.FileInfo, len(ri.Files))
		for i, fi := range ri.Files {
			ret.Files[i] = fi.ToProto()
		}
	}
	return ret
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
