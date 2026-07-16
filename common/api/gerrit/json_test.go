// Copyright 2026 The LUCI Authors.
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
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

func TestAccountInfoToProto(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		var a *accountInfo
		if got := a.ToProto(); got != nil {
			t.Errorf("ToProto() = %v, want nil", got)
		}
	})

	t.Run("populated", func(t *testing.T) {
		a := &accountInfo{
			Name:            "Test User",
			Email:           "user@example.com",
			SecondaryEmails: []string{"user2@example.com"},
			Username:        "testuser",
			AccountID:       1001,
			Tags:            []string{"SERVICE_USER"},
		}
		want := &gerritpb.AccountInfo{
			Name:            "Test User",
			Email:           "user@example.com",
			SecondaryEmails: []string{"user2@example.com"},
			Username:        "testuser",
			AccountId:       1001,
			Tags:            []string{"SERVICE_USER"},
		}
		if got := a.ToProto(); !reflect.DeepEqual(got, want) {
			t.Errorf("ToProto() = %v, want %v", got, want)
		}
	})
}

func TestChangeInfoToProto(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	tsNow := timestamppb.New(now)

	t.Run("full change info", func(t *testing.T) {
		ci := &changeInfo{
			Number:   12345,
			Owner:    &accountInfo{AccountID: 101, Name: "Owner"},
			Project:  "my/project",
			Branch:   "main",
			Subject:  "Subject test",
			Status:   "NEW",
			Hashtags: []string{"tag1"},
			Reviewers: map[string][]*accountInfo{
				"REVIEWER": {{AccountID: 201, Name: "Rev"}},
				"CC":       {{AccountID: 202, Name: "CC"}},
				"REMOVED":  {{AccountID: 203, Name: "Rem"}},
			},
			Revisions: map[string]*revisionInfo{
				"rev1": {Number: 1, Ref: "refs/changes/45/12345/1"},
			},
			Labels: map[string]*labelInfo{
				"Code-Review": {Value: 1},
			},
			Messages: []changeMessageInfo{
				{ID: "msg1", Message: "hello"},
			},
			Requirements: []requirement{
				{Status: "OK", FallbackText: "fallback", Type: "type"},
			},
			SubmitRequirements: []*submitRequirementResultInfo{
				{Name: "Code-Review", Status: "SATISFIED"},
			},
			Created:            Timestamp{Time: now},
			Updated:            Timestamp{Time: now},
			Submitted:          Timestamp{Time: now},
			Submittable:        true,
			IsPrivate:          false,
			MetaRevID:          "meta1",
			RevertOf:           999,
			CherryPickOfChange: 888,
		}

		got, err := ci.ToProto()
		if err != nil {
			t.Fatalf("ToProto() failed: %v", err)
		}

		if got.Number != 12345 || got.Project != "my/project" || got.Branch != "main" || got.Ref != "refs/heads/main" {
			t.Errorf("ToProto basic fields mismatch: %v", got)
		}
		if got.Status != gerritpb.ChangeStatus_NEW {
			t.Errorf("Status = %v, want NEW", got.Status)
		}
		if got.Reviewers == nil || len(got.Reviewers.Reviewers) != 1 || len(got.Reviewers.Ccs) != 1 || len(got.Reviewers.Removed) != 1 {
			t.Errorf("Reviewers status map mismatch: %v", got.Reviewers)
		}
		if len(got.Revisions) != 1 || len(got.Labels) != 1 || len(got.Messages) != 1 || len(got.Requirements) != 1 || len(got.SubmitRequirements) != 1 {
			t.Errorf("Maps and slices length mismatch in ChangeInfo: %v", got)
		}
		if !got.Created.AsTime().Equal(tsNow.AsTime()) {
			t.Errorf("Created timestamp mismatch: got %v, want %v", got.Created, tsNow)
		}
	})

	t.Run("invalid requirement error", func(t *testing.T) {
		ci := &changeInfo{
			Requirements: []requirement{{Status: "INVALID_STATUS"}},
		}
		if _, err := ci.ToProto(); err == nil {
			t.Error("expected error for invalid requirement status, got nil")
		}
	})

	t.Run("invalid submit requirement error", func(t *testing.T) {
		ci := &changeInfo{
			SubmitRequirements: []*submitRequirementResultInfo{{Status: "INVALID_STATUS"}},
		}
		if _, err := ci.ToProto(); err == nil {
			t.Error("expected error for invalid submit requirement status, got nil")
		}
	})
}

func TestLabelInfoToProto(t *testing.T) {
	t.Parallel()

	li := &labelInfo{
		Optional:     true,
		Approved:     &accountInfo{AccountID: 1},
		Rejected:     &accountInfo{AccountID: 2},
		Recommended:  &accountInfo{AccountID: 3},
		Disliked:     &accountInfo{AccountID: 4},
		Blocking:     true,
		Value:        1,
		DefaultValue: 0,
		All: []*approvalInfo{
			{Value: 1, Tag: "tag1"},
		},
		Values: map[string]string{
			"-1":      "Do not submit",
			" 0":      "No score",
			"+1":      "Looks good",
			"invalid": "Should be ignored",
		},
	}

	got := li.ToProto()
	if !got.Optional || !got.Blocking || got.Value != 1 {
		t.Errorf("LabelInfo basic fields mismatch: %v", got)
	}
	if got.Approved.AccountId != 1 || got.Rejected.AccountId != 2 {
		t.Errorf("LabelInfo approved/rejected mismatch: %v", got)
	}
	if len(got.All) != 1 || got.All[0].Value != 1 {
		t.Errorf("LabelInfo All approvals mismatch: %v", got.All)
	}
	wantValues := map[int32]string{
		-1: "Do not submit",
		0:  "No score",
		1:  "Looks good",
	}
	if !reflect.DeepEqual(got.Values, wantValues) {
		t.Errorf("LabelInfo Values = %v, want %v", got.Values, wantValues)
	}
}

func TestApprovalInfoToProto(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	ai := &approvalInfo{
		accountInfo: accountInfo{AccountID: 100},
		Value:       2,
		Date:        Timestamp{Time: now},
		Tag:         "autogenerated:gerrit:cq",
		PostSubmit:  true,
	}

	got := ai.ToProto()
	if got.User.AccountId != 100 || got.Value != 2 || got.Tag != "autogenerated:gerrit:cq" || !got.PostSubmit {
		t.Errorf("ApprovalInfo mismatch: %v", got)
	}
}

func TestChangeMessageInfoToProto(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		var cmi *changeMessageInfo
		if got := cmi.ToProto(); got != nil {
			t.Errorf("ToProto() = %v, want nil", got)
		}
	})

	t.Run("populated", func(t *testing.T) {
		now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		cmi := &changeMessageInfo{
			ID:         "msg123",
			Author:     &accountInfo{AccountID: 1},
			RealAuthor: &accountInfo{AccountID: 2},
			Date:       Timestamp{Time: now},
			Message:    "Uploaded patch set 1.",
			Tag:        "autogenerated:gerrit:newPatchSet",
		}
		got := cmi.ToProto()
		if got.Id != "msg123" || got.Author.AccountId != 1 || got.RealAuthor.AccountId != 2 || got.Message != "Uploaded patch set 1." {
			t.Errorf("ChangeMessageInfo mismatch: %v", got)
		}
	})
}

func TestRequirementToProto(t *testing.T) {
	t.Parallel()

	t.Run("valid status", func(t *testing.T) {
		r := &requirement{
			Status:       "OK",
			FallbackText: "fallback text",
			Type:         "wip",
		}
		got, err := r.ToProto()
		if err != nil {
			t.Fatalf("ToProto() failed: %v", err)
		}
		if got.Status != gerritpb.Requirement_REQUIREMENT_STATUS_OK || got.FallbackText != "fallback text" || got.Type != "wip" {
			t.Errorf("Requirement mismatch: %v", got)
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		r := &requirement{Status: "NON_EXISTENT"}
		if _, err := r.ToProto(); err == nil {
			t.Error("expected error for non existent status, got nil")
		}
	})
}

func TestFileInfoToProto(t *testing.T) {
	t.Parallel()

	fi := &fileInfo{
		LinesInserted: 10,
		LinesDeleted:  5,
		SizeDelta:     120,
		Size:          1000,
	}
	got := fi.ToProto()
	want := &gerritpb.FileInfo{
		LinesInserted: 10,
		LinesDeleted:  5,
		SizeDelta:     120,
		Size:          1000,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FileInfo.ToProto() = %v, want %v", got, want)
	}
}

func TestRevisionInfoToProto(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	ri := &revisionInfo{
		Kind:        "REWORK",
		Number:      2,
		Uploader:    &accountInfo{AccountID: 100},
		Ref:         "refs/changes/63/8094863/2",
		Created:     Timestamp{Time: now},
		Description: "Patch set 2",
		Files: map[string]*fileInfo{
			"foo.go": {LinesInserted: 1},
		},
		Commit: &commitInfo{
			Commit:  "sha123",
			Message: "commit msg",
			Author:  &gitPersonInfo{Name: "Author", Email: "author@example.com"},
		},
	}
	got := ri.ToProto()
	if got.Number != 2 || got.Kind != gerritpb.RevisionInfo_REWORK || got.Description != "Patch set 2" {
		t.Errorf("RevisionInfo basic fields mismatch: %v", got)
	}
	if len(got.Files) != 1 || got.Commit == nil || got.Commit.Id != "sha123" {
		t.Errorf("RevisionInfo files/commit mismatch: %v", got)
	}
}

func TestCommitInfoAndGitPersonInfoToProto(t *testing.T) {
	t.Parallel()

	ci := &commitInfo{
		Commit: "hash123",
		Parents: []*commitInfo{
			{Commit: "parent1"},
		},
		Author: &gitPersonInfo{
			Name:  "Author Name",
			Email: "author@example.com",
		},
		Message: "Commit summary",
	}

	got := ci.ToProto()
	if got.Id != "hash123" || len(got.Parents) != 1 || got.Parents[0].Id != "parent1" {
		t.Errorf("CommitInfo mismatch: %v", got)
	}
	if got.Author.Name != "Author Name" || got.Author.Email != "author@example.com" {
		t.Errorf("GitPersonInfo mismatch: %v", got.Author)
	}
}

func TestRelatedChangeAndCommitInfoToProto(t *testing.T) {
	t.Parallel()

	r := &relatedChangeAndCommitInfo{
		Project:         "my/project",
		ChangeID:        "I1234",
		Number:          100,
		Patchset:        2,
		CurrentPatchset: 3,
		Status:          "MERGED",
		Commit: commitInfo{
			Commit: "sha456",
			Author: &gitPersonInfo{Name: "Author"},
		},
	}

	got := r.ToProto()
	if got.Project != "my/project" || got.Number != 100 || got.Patchset != 2 || got.CurrentPatchset != 3 {
		t.Errorf("RelatedChangeAndCommitInfo basic fields mismatch: %v", got)
	}
	if got.Status != gerritpb.ChangeStatus_MERGED || got.Commit.Id != "sha456" {
		t.Errorf("RelatedChangeAndCommitInfo status/commit mismatch: %v", got)
	}
}

func TestMergeableInfoToProto(t *testing.T) {
	t.Parallel()

	t.Run("valid strategy and submit type", func(t *testing.T) {
		mi := &mergeableInfo{
			SubmitType:    "MERGE_IF_NECESSARY",
			Strategy:      "simple-two-way-in-core",
			Mergeable:     true,
			CommitMerged:  false,
			ContentMerged: true,
			Conflicts:     []string{"conf1"},
			MergeableInto: []string{"main"},
		}
		got, err := mi.ToProto()
		if err != nil {
			t.Fatalf("ToProto() failed: %v", err)
		}
		if got.SubmitType != gerritpb.MergeableInfo_MERGE_IF_NECESSARY {
			t.Errorf("SubmitType = %v, want MERGE_IF_NECESSARY", got.SubmitType)
		}
		if got.Strategy != gerritpb.MergeableStrategy_SIMPLE_TWO_WAY_IN_CORE {
			t.Errorf("Strategy = %v, want SIMPLE_TWO_WAY_IN_CORE", got.Strategy)
		}
		if !got.Mergeable || !got.ContentMerged || len(got.Conflicts) != 1 {
			t.Errorf("MergeableInfo mismatch: %v", got)
		}
	})

	t.Run("invalid strategy", func(t *testing.T) {
		mi := &mergeableInfo{
			SubmitType: "MERGE_IF_NECESSARY",
			Strategy:   "invalid-strategy",
		}
		if _, err := mi.ToProto(); err == nil {
			t.Error("expected error for invalid strategy, got nil")
		}
	})

	t.Run("invalid submit type", func(t *testing.T) {
		mi := &mergeableInfo{
			SubmitType: "INVALID_SUBMIT_TYPE",
			Strategy:   "simple-two-way-in-core",
		}
		if _, err := mi.ToProto(); err == nil {
			t.Error("expected error for invalid submit type, got nil")
		}
	})
}

func TestReviewerInfoAndAddReviewerResultToProto(t *testing.T) {
	t.Parallel()

	t.Run("valid approvals", func(t *testing.T) {
		ri := &reviewerInfo{
			Name:      "Reviewer 1",
			Email:     "rev1@example.com",
			AccountID: 101,
			Approvals: map[string]string{
				"Code-Review": " +1",
				"Verified":    "-1",
			},
		}
		got, err := ri.ToProtoReviewerInfo()
		if err != nil {
			t.Fatalf("ToProtoReviewerInfo() failed: %v", err)
		}
		if got.Account.Name != "Reviewer 1" || got.Account.AccountId != 101 {
			t.Errorf("ReviewerInfo account mismatch: %v", got.Account)
		}
		wantApprovals := map[string]int32{
			"Code-Review": 1,
			"Verified":    -1,
		}
		if !reflect.DeepEqual(got.Approvals, wantApprovals) {
			t.Errorf("Approvals = %v, want %v", got.Approvals, wantApprovals)
		}
	})

	t.Run("invalid approval score", func(t *testing.T) {
		ri := &reviewerInfo{
			Approvals: map[string]string{
				"Code-Review": "not-a-number",
			},
		}
		if _, err := ri.ToProtoReviewerInfo(); err == nil {
			t.Error("expected error for invalid approval score, got nil")
		}
	})

	t.Run("addReviewerResult ToProto", func(t *testing.T) {
		arr := &addReviewerResult{
			Input: "rev1@example.com",
			Reviewers: []reviewerInfo{
				{Name: "Rev 1", AccountID: 1},
			},
			Ccs: []reviewerInfo{
				{Name: "CC 1", AccountID: 2},
			},
			Error:   "",
			Confirm: true,
		}
		got, err := arr.ToProto()
		if err != nil {
			t.Fatalf("addReviewerResult.ToProto() failed: %v", err)
		}
		if got.Input != "rev1@example.com" || len(got.Reviewers) != 1 || len(got.Ccs) != 1 || !got.Confirm {
			t.Errorf("AddReviewerResult mismatch: %v", got)
		}
	})
}

func TestReviewResultToProto(t *testing.T) {
	t.Parallel()

	rr := &reviewResult{
		Labels: map[string]int32{
			"Code-Review": 1,
		},
		Reviewers: map[string]*addReviewerResult{
			"rev1@example.com": {
				Input: "rev1@example.com",
			},
		},
	}

	got, err := rr.ToProto()
	if err != nil {
		t.Fatalf("ReviewResult.ToProto() failed: %v", err)
	}
	if got.Labels["Code-Review"] != 1 || len(got.Reviewers) != 1 {
		t.Errorf("ReviewResult mismatch: %v", got)
	}
}

func TestProjectInfoToProto(t *testing.T) {
	t.Parallel()

	t.Run("valid project info", func(t *testing.T) {
		pi := &projectInfo{
			ID:          "my%2Fproject",
			Parent:      "parent/project",
			Description: "Test project",
			State:       "ACTIVE",
			Branches: map[string]string{
				"main":             "sha1",
				"refs/meta/config": "sha2",
			},
			WebLinks: []*gerritpb.WebLinkInfo{
				{Name: "gitiles", Url: "https://gitiles.example.com"},
			},
		}

		got, err := pi.ToProto()
		if err != nil {
			t.Fatalf("ProjectInfo.ToProto() failed: %v", err)
		}
		if got.Name != "my/project" || got.Parent != "parent/project" || got.State != gerritpb.ProjectInfo_PROJECT_STATE_ACTIVE {
			t.Errorf("ProjectInfo basic fields mismatch: %v", got)
		}
		if got.Refs["refs/heads/main"] != "sha1" || got.Refs["refs/meta/config"] != "sha2" {
			t.Errorf("ProjectInfo absolute refs mismatch: %v", got.Refs)
		}
	})

	t.Run("invalid state enum", func(t *testing.T) {
		pi := &projectInfo{
			ID:    "proj",
			State: "NON_EXISTENT_STATE",
		}
		if _, err := pi.ToProto(); err == nil {
			t.Error("expected error for invalid project state, got nil")
		}
	})
}

func TestSubmitInfoToProto(t *testing.T) {
	t.Parallel()

	si := &submitInfo{Status: "MERGED"}
	got := si.ToProto()
	if got.Status != gerritpb.ChangeStatus_MERGED {
		t.Errorf("SubmitInfo.ToProto() = %v, want MERGED", got.Status)
	}
}

func TestSubmitRequirementResultInfoToProto(t *testing.T) {
	t.Parallel()

	t.Run("valid requirement result", func(t *testing.T) {
		ri := &submitRequirementResultInfo{
			Name:        "Code-Review",
			Description: "Requires Code-Review +1",
			Status:      "SATISFIED",
			IsLegacy:    false,
			ApplicabilityExpressionResult: &submitRequirementExpressionInfo{
				Expression: "is:true",
				Fulfilled:  true,
			},
			SubmittabilityExpressionResult: &submitRequirementExpressionInfo{
				Expression:   "label:Code-Review=+1",
				Fulfilled:    true,
				PassingAtoms: []string{"label:Code-Review=+1"},
			},
		}

		got, err := ri.ToProto()
		if err != nil {
			t.Fatalf("SubmitRequirementResultInfo.ToProto() failed: %v", err)
		}
		if got.Name != "Code-Review" || got.Status != gerritpb.SubmitRequirementResultInfo_SATISFIED {
			t.Errorf("SubmitRequirementResultInfo basic fields mismatch: %v", got)
		}
		if got.ApplicabilityExpressionResult == nil || !got.ApplicabilityExpressionResult.Fulfilled {
			t.Errorf("SubmitRequirementExpressionInfo mismatch: %v", got)
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		ri := &submitRequirementResultInfo{Status: "INVALID_STATUS"}
		if _, err := ri.ToProto(); err == nil {
			t.Error("expected error for invalid status, got nil")
		}
	})
}

func TestMetaDiffToProto(t *testing.T) {
	t.Parallel()

	md := &metaDiff{
		Added:         &changeInfo{Number: 1, Status: "NEW"},
		Removed:       &changeInfo{Number: 2, Status: "MERGED"},
		OldChangeInfo: &changeInfo{Number: 3, Status: "NEW"},
		NewChangeInfo: &changeInfo{Number: 4, Status: "MERGED"},
	}

	got, err := md.ToProto()
	if err != nil {
		t.Fatalf("MetaDiff.ToProto() failed: %v", err)
	}
	if got.Added.Number != 1 || got.Removed.Number != 2 || got.OldChangeInfo.Number != 3 || got.NewChangeInfo.Number != 4 {
		t.Errorf("MetaDiff mismatch: %v", got)
	}
}

func TestAttentionSetAndNotifyDetailsConversion(t *testing.T) {
	t.Parallel()

	inDetails := &gerritpb.NotifyDetails{
		Recipients: []*gerritpb.NotifyDetails_Recipient{
			{
				RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
				Info: &gerritpb.NotifyDetails_Info{
					Accounts: []int64{200, 100, 100}, // tests sorting & deduplication
				},
			},
		},
	}

	details := toNotifyDetails(inDetails)
	if len(details) != 1 {
		t.Fatalf("toNotifyDetails length = %d, want 1", len(details))
	}

	toBucket, ok := details["TO"]
	if !ok {
		t.Fatalf("missing TO key in %v", details)
	}
	wantAccounts := []int64{100, 200}
	if !reflect.DeepEqual(toBucket.Accounts, wantAccounts) {
		t.Errorf("NotifyDetails sorted & deduped accounts = %v, want %v", toBucket.Accounts, wantAccounts)
	}

	inInput := []*gerritpb.AttentionSetInput{
		{
			User:          "user1",
			Reason:        "Need review",
			Notify:        gerritpb.Notify_NOTIFY_NONE,
			NotifyDetails: inDetails,
		},
	}

	outInputs := toAttentionSetInputs(inInput)
	if len(outInputs) != 1 || outInputs[0].User != "user1" || outInputs[0].Reason != "Need review" {
		t.Errorf("toAttentionSetInputs mismatch: %v", outInputs)
	}

	inReviewers := []*gerritpb.ReviewerInput{
		{
			Reviewer: "reviewer1",
			State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_REVIEWER,
		},
	}
	outReviewers := toReviewerInputs(inReviewers)
	if len(outReviewers) != 1 || outReviewers[0].Reviewer != "reviewer1" || outReviewers[0].State != "REVIEWER" {
		t.Errorf("toReviewerInputs mismatch: %v", outReviewers)
	}
}
