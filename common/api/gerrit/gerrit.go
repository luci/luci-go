// Copyright 2017 The LUCI Authors.
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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/context/ctxhttp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

const contentType = "application/json; charset=UTF-8"

// Change represents a Gerrit CL. Information about these fields in:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-info
//
// Note that not all fields will be filled for all CLs and queries depending on
// query options, and not all fields exposed by Gerrit are captured by this
// struct. Adding more fields to this struct should be okay (but only
// Gerrit-supported keys will be populated).
//
// TODO(nodir): replace this type with
// https://godoc.org/go.chromium.org/luci/common/proto/gerrit#ChangeInfo.
type Change struct {
	ChangeNumber           int                           `json:"_number"`
	ID                     string                        `json:"id"`
	ChangeID               string                        `json:"change_id"`
	Project                string                        `json:"project"`
	Branch                 string                        `json:"branch"`
	Topic                  string                        `json:"topic"`
	Hashtags               []string                      `json:"hashtags"`
	Subject                string                        `json:"subject"`
	Status                 string                        `json:"status"`
	Created                string                        `json:"created"`
	Updated                string                        `json:"updated"`
	Mergeable              bool                          `json:"mergeable,omitempty"`
	Messages               []ChangeMessageInfo           `json:"messages"`
	Submittable            bool                          `json:"submittable,omitempty"`
	Submitted              string                        `json:"submitted"`
	SubmitType             string                        `json:"submit_type"`
	SubmitRequirements     []SubmitRequirementResultInfo `json:"submit_requirements,omitempty"`
	Insertions             int                           `json:"insertions"`
	Deletions              int                           `json:"deletions"`
	UnresolvedCommentCount int                           `json:"unresolved_comment_count"`
	HasReviewStarted       bool                          `json:"has_review_started"`
	Owner                  AccountInfo                   `json:"owner"`
	Labels                 map[string]LabelInfo          `json:"labels"`
	Submitter              AccountInfo                   `json:"submitter"`
	Reviewers              Reviewers                     `json:"reviewers"`
	RevertOf               int                           `json:"revert_of"`
	CurrentRevision        string                        `json:"current_revision"`
	CurrentRevisionNumber  int                           `json:"current_revision_number"`
	WorkInProgress         bool                          `json:"work_in_progress,omitempty"`
	CherryPickOfChange     int                           `json:"cherry_pick_of_change"`
	CherryPickOfPatchset   int                           `json:"cherry_pick_of_patch_set"`
	Revisions              map[string]RevisionInfo       `json:"revisions"`
	// MoreChanges is not part of a Change, but gerrit piggy-backs on the
	// last Change in a page to set this flag if there are more changes
	// in the results of a query.
	MoreChanges bool `json:"_more_changes"`
}

// ChangeMessageInfo contains information about a message attached to a change:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-message-info
type ChangeMessageInfo struct {
	ID             string      `json:"id"`
	Author         AccountInfo `json:"author,omitempty"`
	RealAuthor     AccountInfo `json:"real_author,omitempty"`
	Date           Timestamp   `json:"date"`
	Message        string      `json:"message"`
	Tag            string      `json:"tag,omitempty"`
	RevisionNumber int         `json:"_revision_number"`
}

// SubmitRequirementResultInfo contains the result of evaluating a submit requirement on a change:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#submit-requirement-result-info
type SubmitRequirementResultInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
}

// LabelInfo contains information about a label on a change, always
// corresponding to the current patch set.
//
// Only one of Approved, Rejected, Recommended, and Disliked will be set, with
// priority Rejected > Approved > Recommended > Disliked if multiple users have
// given the label different values.
//
// TODO(mknyszek): Support 'value' and 'default_value' fields, as well as the
// fields given with the query parameter DETAILED_LABELS.
type LabelInfo struct {
	// Optional reflects whether the label is optional (neither necessary
	// for nor blocking submission).
	Optional bool `json:"optional,omitempty"`

	// Approved refers to one user who approved this label (max value).
	Approved *AccountInfo `json:"approved,omitempty"`

	// Rejected refers to one user who rejected this label (min value).
	Rejected *AccountInfo `json:"rejected,omitempty"`

	// Recommended refers to one user who recommended this label (positive
	// value, but not max).
	Recommended *AccountInfo `json:"recommended,omitempty"`

	// Disliked refers to one user who disliked this label (negative value
	// but not min).
	Disliked *AccountInfo `json:"disliked,omitempty"`

	// Blocking reflects whether this label block the submit operation.
	Blocking bool `json:"blocking,omitempty"`

	All    []VoteInfo        `json:"all,omitempty"`
	Values map[string]string `json:"values,omitempty"`
}

// RevisionKind represents the "kind" field for a patch set.
type RevisionKind string

// Known revision kinds.
var (
	RevisionRework                 RevisionKind = "REWORK"
	RevisionTrivialRebase                       = "TRIVIAL_REBASE"
	RevisionMergeFirstParentUpdate              = "MERGE_FIRST_PARENT_UPDATE"
	RevisionNoCodeChange                        = "NO_CODE_CHANGE"
	RevisionNoChange                            = "NO_CHANGE"
)

// RevisionInfo contains information about a specific patch set.
//
// Corresponds with
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#revision-info.
// TODO(mknyszek): Support the rest of that structure.
type RevisionInfo struct {
	// PatchSetNumber is the number associated with the given patch set.
	PatchSetNumber int `json:"_number"`

	// Kind is the kind of revision this is.
	//
	// Allowed values are specified in the RevisionKind type.
	Kind RevisionKind `json:"kind"`

	// Ref is the Git reference for the patchset.
	Ref string `json:"ref"`

	// Uploader represents the account which uploaded this patch set.
	Uploader AccountInfo `json:"uploader"`

	// The description of this patchset, as displayed in the patchset selector menu.
	Description string `json:"description,omitempty"`

	// The commit associated with this revision.
	Commit CommitInfo `json:"commit,omitempty"`

	// A map of modified file paths to FileInfos.
	Files map[string]FileInfo `json:"files,omitempty"`
}

// CommitInfo contains information about a commit.
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#commit-info
//
// TODO(kjharland): Support remaining fields.
type CommitInfo struct {
	Commit    string       `json:"commit,omitempty"`
	Parents   []CommitInfo `json:"parents,omitempty"`
	Author    AccountInfo  `json:"author,omitempty"`
	Committer AccountInfo  `json:"committer,omitempty"`
	Subject   string       `json:"subject,omitempty"`
	Message   string       `json:"message,omitempty"`
}

// FileInfo contains information about a file in a patch set.
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#file-info
type FileInfo struct {
	Status        string `json:"status,omitempty"`
	Binary        bool   `json:"binary,omitempty"`
	OldPath       string `json:"old_path,omitempty"`
	LinesInserted int    `json:"lines_inserted,omitempty"`
	LinesDeleted  int    `json:"lines_deleted,omitempty"`
	SizeDelta     int64  `json:"size_delta"`
	Size          int64  `json:"size"`
}

// Reviewers is a map that maps a type of reviewer to its account info.
type Reviewers struct {
	CC       []AccountInfo `json:"CC"`
	Reviewer []AccountInfo `json:"REVIEWER"`
	Removed  []AccountInfo `json:"REMOVED"`
}

// AccountInfo contains information about an account.
type AccountInfo struct {
	// AccountID is the numeric ID of the account managed by Gerrit, and is guaranteed to
	// be unique.
	AccountID int `json:"_account_id,omitempty"`

	// Name is the full name of the user.
	Name string `json:"name,omitempty"`

	// Email is the email address the user prefers to be contacted through.
	Email string `json:"email,omitempty"`

	// SecondaryEmails is a list of secondary email addresses of the user.
	SecondaryEmails []string `json:"secondary_emails,omitempty"`

	// Username is the username of the user.
	Username string `json:"username,omitempty"`

	// MoreAccounts represents whether the query would deliver more results if not limited.
	// Only set on the last account that is returned.
	MoreAccounts bool `json:"_more_accounts,omitempty"`
}

// VoteInfo is AccountInfo plus a value field, used to represent votes on a label.
type VoteInfo struct {
	AccountInfo       // Embedded type
	Value       int64 `json:"value"`
}

// SubmittedTogetherInfo contains information about a collection of changes that would be submitted together.
type SubmittedTogetherInfo struct {

	// A list of ChangeInfo entities representing the changes to be submitted together.
	Changes []Change `json:"changes"`

	// The number of changes to be submitted together that the current user cannot see.
	NonVisibleChanges int `json:"non_visible_changes,omitempty"`
}

// BranchInfo contains information about a branch.
type BranchInfo struct {
	// Ref is the ref of the branch.
	Ref string `json:"ref"`

	// Revision is the revision to which the branch points.
	Revision string `json:"revision"`
}

// ValidateGerritURL validates Gerrit URL for use in this package.
func ValidateGerritURL(gerritURL string) error {
	_, err := NormalizeGerritURL(gerritURL)
	return err
}

// NormalizeGerritURL returns canonical for Gerrit URL.
//
// error is returned if validation fails.
func NormalizeGerritURL(gerritURL string) (string, error) {
	u, err := url.Parse(gerritURL)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("%s should start with https://", gerritURL)
	}
	if !strings.HasSuffix(u.Host, "-review.googlesource.com") {
		return "", errors.New("only *-review.googlesource.com Gerrits supported")
	}
	if u.Fragment != "" {
		return "", errors.New("no fragments allowed in gerritURL")
	}
	if u.Path != "" && u.Path != "/" {
		return "", errors.New("Unexpected path in URL")
	}
	if u.Path != "/" {
		u.Path = "/"
	}
	return u.String(), nil
}

// Client is a production Gerrit client, instantiated via NewClient(),
// and uses an http.Client along with a validated url to communicate with
// Gerrit.
type Client struct {
	httpClient *http.Client

	// Set by NewClient after validation.
	gerritURL url.URL

	// Strategy for retrying GET requests. Other requests are not retried as
	// they may not be idempotent.
	retryStrategy retry.Factory
}

// NewClient creates a new instance of Client and validates and stores
// the URL to reach Gerrit. The result is wrapped inside a Client so that
// the apis can be called directly on the value returned by this function.
func NewClient(c *http.Client, gerritURL string) (*Client, error) {
	u, err := NormalizeGerritURL(gerritURL)
	if err != nil {
		return nil, err
	}
	pu, err := url.Parse(u)
	if err != nil {
		return nil, err
	}
	return &Client{c, *pu, retry.Default}, nil
}

// EmailInfo contains the response fields from ListAccountEmails.
type EmailInfo struct {
	// Email address linked to the user account.
	Email string `json:"email"`

	// Set true if the email address is set as preferred.
	Preferred bool `json:"preferred"`

	// Set true if the user must confirm control of the email address by following
	// a verification link before Gerrit will permit use of this address.
	PendingConfirmation bool `json:"pending_confirmation"`
}

// ListAccountEmails returns the email addresses linked in the given Gerrit account.
func (c *Client) ListAccountEmails(email string) ([]*EmailInfo, error) {
	panic("not implemented")
}

// ChangeQueryParams contains the parameters necessary for querying changes from Gerrit.
type ChangeQueryParams struct {
	// Actual query string, see
	// https://gerrit-review.googlesource.com/Documentation/user-search.html#_search_operators
	Query string `json:"q"`
	// How many changes to include in the response.
	N int `json:"n"`
	// Skip this many from the list of results (unreliable for paging).
	S int `json:"S"`
	// Include these options in the queries. Certain options will make
	// Gerrit fill in additional fields of the response. These require
	// additional database searches and may delay the response.
	//
	// The supported strings for options are listed in Gerrit's api
	// documentation at the link below:
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
	Options []string `json:"o"`
}

// queryString renders the ChangeQueryParams as a url.Values.
func (qr *ChangeQueryParams) queryString() url.Values {
	qs := make(url.Values, len(qr.Options)+3)
	qs.Add("q", qr.Query)
	if qr.N > 0 {
		qs.Add("n", strconv.Itoa(qr.N))
	}
	if qr.S > 0 {
		qs.Add("S", strconv.Itoa(qr.S))
	}
	for _, o := range qr.Options {
		qs.Add("o", o)
	}
	return qs
}

// ChangeQuery returns a list of Gerrit changes for a given ChangeQueryParams.
//
// One example use case for this is getting the CL for a given commit hash.
// Only the .Query property of the qr parameter is required.
//
// Returns a slice of Change, whether there are more changes to fetch
// and an error.
func (c *Client) ChangeQuery(ctx context.Context, qr ChangeQueryParams) ([]*Change, bool, error) {
	var resp struct {
		Collection []*Change
	}
	if _, err := c.get(ctx, "a/changes/", qr.queryString(), &resp.Collection); err != nil {
		return nil, false, err
	}
	result := resp.Collection
	moreChanges := false
	if len(result) > 0 {
		moreChanges = result[len(result)-1].MoreChanges
		result[len(result)-1].MoreChanges = false
	}
	return result, moreChanges, nil
}

// ChangeDetailsParams contains optional parameters for getting change details from Gerrit.
type ChangeDetailsParams struct {
	// Options is a set of optional details to add as additional fieldsof the response.
	// These require additional database searches and may delay the response.
	//
	// The supported strings for options are listed in Gerrit's api
	// documentation at the link below:
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
	Options []string `json:"o"`
}

// queryString renders the ChangeDetailsParams as a url.Values.
func (qr *ChangeDetailsParams) queryString() url.Values {
	qs := make(url.Values, len(qr.Options))
	for _, o := range qr.Options {
		qs.Add("o", o)
	}
	return qs
}

// ChangeDetails gets details about a single change with optional fields.
//
// This method returns a single *Change and an error.
//
// The changeID parameter may be in any of the forms supported by Gerrit:
//   - "4247"
//   - "I8473b95934b5732ac55d26311a706c9c2bde9940"
//   - etc. See the link below.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id
//
// options is a list of strings like {"CURRENT_REVISION"} which tells Gerrit
// to return non-default properties for Change. The supported strings for
// options are listed in Gerrit's api documentation at the link below:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
func (c *Client) ChangeDetails(ctx context.Context, changeID string, options ChangeDetailsParams) (*Change, error) {
	var resp Change
	path := fmt.Sprintf("a/changes/%s/detail", url.PathEscape(changeID))
	if _, err := c.get(ctx, path, options.queryString(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// The CommentInput entity contains information for creating an inline comment.
type CommentInput struct {
	// The URL encoded UUID of the comment if an existing draft comment should be updated.
	// Optional
	ID string `json:"id,omitempty"`

	// The file path for which the inline comment should be added.
	// Doesn’t need to be set if contained in a map where the key is the file path.
	// Optional
	Path string `json:"path,omitempty"`

	// The side on which the comment should be added.
	// Allowed values are REVISION and PARENT.
	// If not set, the default is REVISION.
	// Optional
	Side string `json:"side,omitempty"`

	// The number of the line for which the comment should be added.
	// 0 if it is a file comment.
	// If neither line nor range is set, a file comment is added.
	// If range is set, this value is ignored in favor of the end_line of the range.
	// Optional
	Line int `json:"line,omitempty"`

	// The range of the comment as a CommentRange entity.
	// Optional
	Range CommentRange `json:"range,omitempty"`

	// The URL encoded UUID of the comment to which this comment is a reply.
	// Optional
	InReplyTo string `json:"in_reply_to,omitempty"`

	// The comment message.
	// If not set and an existing draft comment is updated, the existing draft comment is deleted.
	// Optional
	Message string `json:"message,omitempty"`

	// Value of the tag field. Only allowed on draft comment inputs;
	// for published comments, use the tag field in ReviewInput.
	// Votes/comments that contain tag with 'autogenerated:' prefix can be filtered out in the web UI.
	// Optional
	Tag string `json:"tag,omitempty"`

	// Whether or not the comment must be addressed by the user.
	// This value will default to false if the comment is an orphan, or the
	// value of the in_reply_to comment if it is supplied.
	// Optional
	Unresolved bool `json:"unresolved,omitempty"`
}

// The RobotCommentInput entity contains information for creating an inline
// robot comment.
type RobotCommentInput struct {
	// The name of the robot that generated this comment.
	// Required
	RobotID string `json:"robot_id"`

	// A unique ID of the run of the robot.
	// Required
	RobotRunID string `json:"robot_run_id"`

	// The file path for which the inline comment should be added.
	Path string `json:"path,omitempty"`

	// The side on which the comment should be added.
	// Allowed values are REVISION and PARENT.
	// If not set, the default is REVISION.
	// Optional
	Side string `json:"side,omitempty"`

	// The number of the line for which the comment should be added.
	// 0 if it is a file comment.
	// If neither line nor range is set, a file comment is added.
	// If range is set, this value is ignored in favor of the end_line of the range.
	// Optional
	Line int `json:"line,omitempty"`

	// The range of the comment as a CommentRange entity.
	// Optional
	Range *CommentRange `json:"range,omitempty"`

	// The URL encoded UUID of the comment to which this comment is a reply.
	// Optional
	InReplyTo string `json:"in_reply_to,omitempty"`

	// The comment message.
	// If not set and an existing draft comment is updated, the existing draft comment is deleted.
	// Optional
	Message string `json:"message,omitempty"`

	// Suggested fixes.
	FixSuggestions []FixSuggestion `json:"fix_suggestions,omitempty"`
}

// FixSuggestion represents a suggested fix for a robot comment.
type FixSuggestion struct {
	Description  string        `json:"description"`
	Replacements []Replacement `json:"replacements"`
}

// Replacement represents a potential source replacement for applying a
// suggested fix.
type Replacement struct {
	Path        string        `json:"path"`
	Replacement string        `json:"replacement"`
	Range       *CommentRange `json:"range,omitempty"`
}

// CommentRange is included within Comment. See Comment for more details.
type CommentRange struct {
	StartLine      int `json:"start_line,omitempty"`
	StartCharacter int `json:"start_character,omitempty"`
	EndLine        int `json:"end_line,omitempty"`
	EndCharacter   int `json:"end_character,omitempty"`
}

// Comment represents a comment on a Gerrit CL. Information about these fields
// is in:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#comment-info
//
// Note that not all fields will be filled for all comments depending on the
// way the comment was added to Gerrit, and not all fields exposed by Gerrit
// are captured by this struct. Adding more fields to this struct should be
// okay (but only Gerrit-supported keys will be populated).
type Comment struct {
	ID              string       `json:"id"`
	Owner           AccountInfo  `json:"author"`
	ChangeMessageID string       `json:"change_message_id"`
	PatchSet        int          `json:"patch_set"`
	Line            int          `json:"line"`
	Range           CommentRange `json:"range"`
	Updated         string       `json:"updated"`
	Message         string       `json:"message"`
	Unresolved      bool         `json:"unresolved"`
	InReplyTo       string       `json:"in_reply_to"`
	CommitID        string       `json:"commit_id"`
}

// RobotComment represents a robot comment on a Gerrit CL. Information about
// these fields is in:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#robot-comment-info
//
// RobotComment shares most of the same fields with Comment. Note that robot
// comments do not have the `unresolved` field so it will always be false.
type RobotComment struct {
	Comment

	RobotID    string            `json:"robot_id"`
	RobotRunID string            `json:"robot_run_id"`
	URL        string            `json:"url"`
	Properties map[string]string `json:"properties"`
}

// ListChangeComments gets all comments on a single change.
//
// This method returns a list of comments for each file path (including
// pseudo-files like '/PATCHSET_LEVEL' and '/COMMIT_MSG') and an error.
//
// The changeID parameter may be in any of the forms supported by Gerrit:
//   - "4247"
//   - "I8473b95934b5732ac55d26311a706c9c2bde9940"
//   - etc. See the link below.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id
func (c *Client) ListChangeComments(ctx context.Context, changeID string, revisionID string) (map[string][]Comment, error) {
	var resp map[string][]Comment
	var path string
	if revisionID != "" {
		path = fmt.Sprintf("a/changes/%s/revisions/%s/comments", url.PathEscape(changeID), url.PathEscape(revisionID))
	} else {
		path = fmt.Sprintf("a/changes/%s/comments", url.PathEscape(changeID))
	}
	if _, err := c.get(ctx, path, url.Values{}, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListRobotComments gets all robot comments on a single change.
//
// This method returns a list of robot comments for each file path (including
// pseudo-files like '/PATCHSET_LEVEL' and '/COMMIT_MSG') and an error.
//
// The changeID parameter may be in any of the forms supported by Gerrit:
//   - "4247"
//   - "I8473b95934b5732ac55d26311a706c9c2bde9940"
//   - etc. See the link below.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id
func (c *Client) ListRobotComments(ctx context.Context, changeID string, revisionID string) (map[string][]RobotComment, error) {
	var resp map[string][]RobotComment
	var path string
	if revisionID != "" {
		path = fmt.Sprintf("a/changes/%s/revisions/%s/robotcomments", url.PathEscape(changeID), url.PathEscape(revisionID))
	} else {
		path = fmt.Sprintf("a/changes/%s/robotcomments", url.PathEscape(changeID))
	}
	if _, err := c.get(ctx, path, url.Values{}, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ChangesSubmittedTogether returns a list of Gerrit changes which are submitted
// when Submit is called for the given change, including the current change itself.
// As a special case, the list is empty if this change would be submitted by itself
// (without other changes).
//
// Returns a slice of Change and an error.
//
// The changeID parameter may be in any of the forms supported by Gerrit:
//   - "4247"
//   - "I8473b95934b5732ac55d26311a706c9c2bde9940"
//   - etc. See the link below.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#submitted-together
//
// options is a list of strings like {"CURRENT_REVISION"} which tells Gerrit
// to return non-default properties for each Change. The supported strings for
// options are listed in Gerrit's api documentation at the link below:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
func (c *Client) ChangesSubmittedTogether(ctx context.Context, changeID string, options ChangeDetailsParams) (*SubmittedTogetherInfo, error) {
	var resp SubmittedTogetherInfo
	path := fmt.Sprintf("a/changes/%s/submitted_together", url.PathEscape(changeID))
	if _, err := c.get(ctx, path, options.queryString(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ChangeInput contains the parameters necessary for creating a change in
// Gerrit.
//
// This struct is intended to be one-to-one with the ChangeInput structure
// described in Gerrit's documentation:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-input
//
// The exception is only status, which is always "NEW" and is therefore unavailable in
// this struct.
//
// TODO(mknyszek): Support merge and notify_details.
type ChangeInput struct {
	// Project is the name of the project for this change.
	Project string `json:"project"`

	// Branch is the name of the target branch for this change.
	// refs/heads/ prefix is omitted.
	Branch string `json:"branch"`

	// Subject is the header line of the commit message.
	Subject string `json:"subject"`

	// Topic is the topic to which this change belongs. Optional.
	Topic string `json:"topic,omitempty"`

	// IsPrivate sets whether the change is marked private.
	IsPrivate bool `json:"is_private,omitempty"`

	// WorkInProgress sets the work-in-progress bit for the change.
	WorkInProgress bool `json:"work_in_progress,omitempty"`

	// BaseChange is the change this new change is based on. Optional.
	//
	// This can be anything Gerrit expects as a ChangeID. We recommend <numericId>,
	// but the full list of supported formats can be found here:
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id
	BaseChange string `json:"base_change,omitempty"`

	// NewBranch allows for creating a new branch when set to true.
	NewBranch bool `json:"new_branch,omitempty"`

	// Notify is an enum specifying whom to send notifications to.
	//
	// Valid values are NONE, OWNER, OWNER_REVIEWERS, and ALL.
	//
	// Optional. The default is ALL.
	Notify string `json:"notify,omitempty"`
}

// CreateChange creates a new change in Gerrit.
//
// Returns a Change describing the newly created change or an error.
func (c *Client) CreateChange(ctx context.Context, ci *ChangeInput) (*Change, error) {
	var resp Change
	if _, err := c.post(ctx, "a/changes/", ci, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// AbandonInput contains information for abandoning a change.
//
// This struct is intended to be one-to-one with the AbandonInput structure
// described in Gerrit's documentation:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#abandon-input
//
// TODO(mknyszek): Support notify_details.
type AbandonInput struct {
	// Message to be added as review comment to the change when abandoning it.
	Message string `json:"message,omitempty"`

	// Notify is an enum specifying whom to send notifications to.
	//
	// Valid values are NONE, OWNER, OWNER_REVIEWERS, and ALL.
	//
	// Optional. The default is ALL.
	Notify string `json:"notify,omitempty"`
}

// AbandonChange abandons an existing change in Gerrit.
//
// Returns a Change referenced by changeID, with status ABANDONED, if
// abandoned.
//
// AbandonInput is optional (that is, may be nil).
//
// The changeID parameter may be in any of the forms supported by Gerrit:
//   - "4247"
//   - "I8473b95934b5732ac55d26311a706c9c2bde9940"
//   - etc. See the link below.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id
func (c *Client) AbandonChange(ctx context.Context, changeID string, ai *AbandonInput) (*Change, error) {
	var resp Change
	path := fmt.Sprintf("a/changes/%s/abandon", url.PathEscape(changeID))
	if _, err := c.post(ctx, path, ai, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// IsChangePureRevert determines if a change is a pure revert of another commit.
//
// This method returns a bool and an error.
//
// To determine which commit the change is purportedly reverting, use the
// revert_of property of the change.
//
// Gerrit's doc for the api:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#get-pure-revert
func (c *Client) IsChangePureRevert(ctx context.Context, changeID string) (bool, error) {
	resp := &pureRevertResponse{}
	path := fmt.Sprintf("changes/%s/pure_revert", url.PathEscape(changeID))
	sc, err := c.get(ctx, path, url.Values{}, resp)
	if err != nil {
		switch sc {
		case 400:
			return false, nil
		default:
			return false, err
		}
	}
	return resp.IsPureRevert, nil
}

// ReviewerInput contains information for adding a reviewer to a change.
//
// TODO(mknyszek): Add support for notify_details.
type ReviewerInput struct {
	// Reviewer is an account-id that should be added as a reviewer, or a group-id for which
	// all members should be added as reviewers. If Reviewer identifies both an account and a
	// group, only the account is added as a reviewer to the change.
	//
	// More information on account-id may be found here:
	// https://gerrit-review.googlesource.com/Documentation/rest-api-accounts.html#account-id
	//
	// More information on group-id may be found here:
	// https://gerrit-review.googlesource.com/Documentation/rest-api-groups.html#group-id
	Reviewer string `json:"reviewer"`

	// State is the state in which to add the reviewer. Possible reviewer states are REVIEWER
	// and CC. If not given, defaults to REVIEWER.
	State string `json:"state,omitempty"`

	// Confirmed represents whether adding the reviewer is confirmed.
	//
	// The Gerrit server may be configured to require a confirmation when adding a group as
	// a reviewer that has many members.
	Confirmed bool `json:"confirmed"`

	// Notify is an enum specifying whom to send notifications to.
	//
	// Valid values are NONE, OWNER, OWNER_REVIEWERS, and ALL.
	//
	// Optional. The default is ALL.
	Notify string `json:"notify,omitempty"`
}

// ReviewInput contains information for adding a review to a revision.
//
// TODO(mknyszek): Add support for drafts, notify, and omit_duplicate_comments.
type ReviewInput struct {
	// Inline comments to be added to the revision.
	Comments map[string][]CommentInput `json:"comments,omitempty"`

	// Robot comments to be added to the revision.
	RobotComments map[string][]RobotCommentInput `json:"robot_comments,omitempty"`

	// Message is the message to be added as a review comment.
	Message string `json:"message,omitempty"`

	// Tag represents a tag to apply to review comment messages, votes, and inline comments.
	//
	// Tags may be used by CI or other automated systems to distinguish them from human
	// reviews.
	Tag string `json:"tag,omitempty"`

	// Labels represents the votes that should be added to the revision as a map of label names
	// to voting values.
	Labels map[string]int `json:"labels,omitempty"`

	// Notify is an enum specifying whom to send notifications to.
	//
	// Valid values are NONE, OWNER, OWNER_REVIEWERS, and ALL.
	//
	// Optional. The default is ALL.
	Notify string `json:"notify,omitempty"`

	// OnBehalfOf is an account-id which the review should be posted on behalf of.
	//
	// To use this option the caller must have granted labelAs-NAME permission for all keys
	// of labels.
	//
	// More information on account-id may be found here:
	// https://gerrit-review.googlesource.com/Documentation/rest-api-accounts.html#account-id
	OnBehalfOf string `json:"on_behalf_of,omitempty"`

	// Reviewers is a list of ReviewerInput representing reviewers that should be added to the change. Optional.
	Reviewers []ReviewerInput `json:"reviewers,omitempty"`

	// Ready, if set to true, will move the change from WIP to ready to review if possible.
	//
	// It is an error for both Ready and WorkInProgress to be true.
	Ready bool `json:"ready,omitempty"`

	// WorkInProgress sets the work-in-progress bit for the change.
	//
	// It is an error for both Ready and WorkInProgress to be true.
	WorkInProgress bool `json:"work_in_progress,omitempty"`
}

// SubmitInput contains information for submitting a change.
type SubmitInput struct {
	// Notify is an enum specifying whom to send notifications to.
	//
	// Valid values are NONE, OWNER, OWNER_REVIEWERS, and ALL.
	//
	// Optional. The default is ALL.
	Notify string `json:"notify,omitempty"`

	// OnBehalfOf is an account-id which the review should be posted on behalf of.
	//
	// To use this option the caller must have granted labelAs-NAME permission for all keys
	// of labels.
	//
	// More information on account-id may be found here:
	// https://gerrit-review.googlesource.com/Documentation/rest-api-accounts.html#account-id
	OnBehalfOf string `json:"on_behalf_of,omitempty"`
}

// ReviewerInfo contains information about a reviewer and their votes on a change.
//
// It has the same fields as AccountInfo, except the AccountID is optional if an unregistered reviewer
// was added.
type ReviewerInfo struct {
	AccountInfo

	// Approvals maps label names to the approval values given by this reviewer.
	Approvals map[string]string `json:"approvals,omitempty"`
}

// AddReviewerResult describes the result of adding a reviewer to a change.
type AddReviewerResult struct {
	// Input is the value of the `reviewer` field from ReviewerInput set while adding the reviewer.
	Input string `json:"input"`

	// Reviewers is a list of newly added reviewers.
	Reviewers []ReviewerInfo `json:"reviewers,omitempty"`

	// CCs is a list of newly CCed accounts. This field will only appear if the requested `state`
	// for the reviewer was CC and NoteDb is enabled on the server.
	CCs []ReviewerInfo `json:"ccs,omitempty"`

	// Error is a message explaining why the reviewer could not be added.
	//
	// If a group was specified in the input and an error is returned, that means none of the
	// members were added as reviewer.
	Error string `json:"error,omitempty"`

	// Confirm represents whether adding the reviewer requires confirmation.
	Confirm bool `json:"confirm,omitempty"`
}

// ReviewResult contains information regarding the updates that were made to a review.
type ReviewResult struct {
	// Labels is a map of labels to values after the review was posted.
	//
	// nil if any reviewer additions were rejected.
	Labels map[string]int `json:"labels,omitempty"`

	// Reviewers is a map of accounts or group identifiers to an AddReviewerResult representing
	// the outcome of adding the reviewer.
	//
	// Absent if no reviewer additions were requested.
	Reviewers map[string]AddReviewerResult `json:"reviewers,omitempty"`

	// Ready marks if the change was moved from WIP to ready for review as a result of this action.
	Ready bool `json:"ready,omitempty"`
}

// SetReview sets a review on a revision, optionally also publishing
// draft comments, setting labels, adding reviewers or CCs, and modifying
// the work in progress property.
//
// Returns a ReviewResult which describes the applied labels and any added reviewers.
//
// The changeID parameter may be in any of the forms supported by Gerrit:
//   - "4247"
//   - "I8473b95934b5732ac55d26311a706c9c2bde9940"
//   - etc. See the link below.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id
//
// The revisionID parameter may be in any of the forms supported by Gerrit:
//   - "current"
//   - a commit ID
//   - "0" or the literal "edit" for a change edit
//   - etc. See the link below.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#revision-id
func (c *Client) SetReview(ctx context.Context, changeID string, revisionID string, ri *ReviewInput) (*ReviewResult, error) {
	var resp ReviewResult
	path := fmt.Sprintf("a/changes/%s/revisions/%s/review", url.PathEscape(changeID), url.PathEscape(revisionID))
	if _, err := c.post(ctx, path, ri, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// MergeableResult contains information if the change for the revision can be merged or not.
type MergeableResult struct {

	// Mergeable marks if the change is ready to be submitted cleanly, false otherwise
	Mergeable bool `json:"mergeable"`
}

// GetMergeable API checks if a change is ready to be submitted cleanly.
//
// Returns a MergeableResult that has details if the change be merged or not.
func (c *Client) GetMergeable(ctx context.Context, changeID string, revisionID string) (*MergeableResult, error) {
	var resp MergeableResult
	path := fmt.Sprintf("a/changes/%s/revisions/%s/mergeable", url.PathEscape(changeID), url.PathEscape(revisionID))
	if _, err := c.get(ctx, path, url.Values{}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Submit submits a change to the repository. It bypasses the Commit Queue.
//
// Returns a Change.
//
// The changeID parameter may be in any of the forms supported by Gerrit:
//   - "4247"
//   - "I8473b95934b5732ac55d26311a706c9c2bde9940"
//   - etc. See the link below.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id
func (c *Client) Submit(ctx context.Context, changeID string, si *SubmitInput) (*Change, error) {
	var resp Change
	path := fmt.Sprintf("a/changes/%s/submit", url.PathEscape(changeID))
	if _, err := c.post(ctx, path, si, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RebaseInput contains information for rebasing a change.
type RebaseInput struct {
	// Base is the revision to rebase on top of.
	Base string `json:"base,omitempty"`

	// AllowConflicts allows the rebase to succeed even if there are conflicts.
	AllowConflicts bool `json:"allow_conflicts,omitempty"`

	// OnBehalfOfUploader causes the rebase to be done on behalf of the
	// uploader. This means the uploader of the current patch set will also be
	// the uploader of the rebased patch set.
	//
	// Rebasing on behalf of the uploader is only supported for trivial rebases.
	// This means this option cannot be combined with the `allow_conflicts`
	// option.
	OnBehalfOfUploader bool `json:"on_behalf_of_uploader,omitempty"`
}

// RebaseChange rebases an open change on top of a specified revision (or its
// parent change, if no revision is specified).
//
// Returns a Change referenced by changeID, if rebased.
//
// RebaseInput is optional (that is, may be nil).
//
// The changeID parameter may be in any of the forms supported by Gerrit:
//   - "4247"
//   - "I8473b95934b5732ac55d26311a706c9c2bde9940"
//   - etc. See the link below.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id
func (c *Client) RebaseChange(ctx context.Context, changeID string, ri *RebaseInput) (*Change, error) {
	var resp Change
	path := fmt.Sprintf("a/changes/%s/rebase", url.PathEscape(changeID))
	if _, err := c.post(ctx, path, ri, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RestoreInput contains information for restoring a change.
type RestoreInput struct {
	// Message is the message to be added as review comment to the change when restoring the change.
	Message string `json:"message,omitempty"`
}

// RestoreChange restores an existing abandoned change in Gerrit.
//
// Returns a Change referenced by changeID, if restored.
//
// RestoreInput is optional (that is, may be nil).
//
// The changeID parameter may be in any of the forms supported by Gerrit:
//   - "4247"
//   - "I8473b95934b5732ac55d26311a706c9c2bde9940"
//   - etc. See the link below.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id
func (c *Client) RestoreChange(ctx context.Context, changeID string, ri *RestoreInput) (*Change, error) {
	var resp Change
	path := fmt.Sprintf("a/changes/%s/restore", url.PathEscape(changeID))
	if _, err := c.post(ctx, path, ri, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BranchInput contains information for creating a branch.
type BranchInput struct {
	// Ref is the name of the branch.
	Ref string `json:"ref,omitempty"`

	// Revision is the base revision of the new branch.
	Revision string `json:"revision,omitempty"`
}

// CreateBranch creates a branch.
//
// Returns BranchInfo, or an error if the branch could not be created.
//
// See the link below for API documentation.
// https://gerrit-review.googlesource.com/Documentation/rest-api-projects.html#create-branch
func (c *Client) CreateBranch(ctx context.Context, projectID string, bi *BranchInput) (*BranchInfo, error) {
	var resp BranchInfo
	path := fmt.Sprintf("a/projects/%s/branches/%s", url.PathEscape(projectID), url.PathEscape(bi.Ref))
	if _, err := c.post(ctx, path, bi, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// AccountQueryParams contains the parameters necessary for querying accounts from Gerrit.
type AccountQueryParams struct {
	// Actual query string, see
	// https://gerrit-review.googlesource.com/Documentation/user-search-accounts.html#_search_operators
	Query string `json:"q"`
	// How many changes to include in the response.
	N int `json:"n"`
	// Skip this many from the list of results (unreliable for paging).
	S int `json:"S"`
	// Include these options in the queries. Certain options will make
	// Gerrit fill in additional fields of the response. These require
	// additional database searches and may delay the response.
	//
	// The supported strings for options are listed in Gerrit's api
	// documentation at the link below:
	// https://gerrit-review.googlesource.com/Documentation/rest-api-accounts.html
	Options []string `json:"o"`
}

// queryString renders the ChangeQueryParams as a url.Values.
func (qr *AccountQueryParams) queryString() url.Values {
	qs := make(url.Values, len(qr.Options)+3)
	qs.Add("q", qr.Query)
	if qr.N > 0 {
		qs.Add("n", strconv.Itoa(qr.N))
	}
	if qr.S > 0 {
		qs.Add("S", strconv.Itoa(qr.S))
	}
	for _, o := range qr.Options {
		qs.Add("o", o)
	}
	return qs
}

// AccountQuery gets all matching accounts.
//
// Only the .Query property of the qr parameter is required.
//
// Returns a slice of AccountInfo, whether there are more accounts to fetch,
// and an error.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-groups.html
func (c *Client) AccountQuery(ctx context.Context, qr AccountQueryParams) ([]*AccountInfo, bool, error) {
	var resp struct{ Collection []*AccountInfo }
	if _, err := c.get(ctx, "a/accounts/", qr.queryString(), &resp.Collection); err != nil {
		return nil, false, err
	}
	result := resp.Collection
	moreAccounts := false
	if len(result) > 0 {
		moreAccounts = result[len(result)-1].MoreAccounts
		result[len(result)-1].MoreAccounts = false
	}
	return result, moreAccounts, nil
}

// WipInput contains information for marking a change as work-in-progress or
// ready-for-review.
type WipInput struct {
	// Message is the message to be added as review comment to the change when
	// changing the WIP state.
	Message string `json:"message,omitempty"`
}

// Wip marks a change as work-in-progress in Gerrit.
//
// Returns a Change referenced by changeID, if successful.
//
// WipInput is optional (that is, may be nil).
//
// The changeID parameter may be in any of the forms supported by Gerrit:
//   - "4247"
//   - "I8473b95934b5732ac55d26311a706c9c2bde9940"
//   - etc. See the link below.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id
func (c *Client) Wip(ctx context.Context, changeID string, ri *WipInput) (any, error) {
	var resp string
	path := fmt.Sprintf("a/changes/%s/wip", url.PathEscape(changeID))
	if _, err := c.post(ctx, path, ri, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// Ready marks a change as ready-for-review in Gerrit.
//
// Returns a Change referenced by changeID, if successful.
//
// WipInput is optional (that is, may be nil).
//
// The changeID parameter may be in any of the forms supported by Gerrit:
//   - "4247"
//   - "I8473b95934b5732ac55d26311a706c9c2bde9940"
//   - etc. See the link below.
//
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id
func (c *Client) Ready(ctx context.Context, changeID string, ri *WipInput) (any, error) {
	var resp string
	path := fmt.Sprintf("a/changes/%s/ready", url.PathEscape(changeID))
	if _, err := c.post(ctx, path, ri, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, result any) (int, error) {
	u := c.gerritURL
	u.Opaque = "//" + u.Host + "/" + path
	u.RawQuery = query.Encode()

	var statusCode int
	err := retry.Retry(ctx, transient.Only(c.retryStrategy), func() error {
		var err error
		statusCode, err = c.getOnce(ctx, u, result)
		return err
	}, nil)
	return statusCode, err
}

func (c *Client) getOnce(ctx context.Context, u url.URL, result any) (int, error) {
	r, err := ctxhttp.Get(ctx, c.httpClient, u.String())
	if err != nil {
		return 0, transient.Tag.Apply(err)
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		err = errors.Fmt("failed to fetch %q, status code %d", u.String(), r.StatusCode)
		if r.StatusCode >= 500 {
			err = transient.Tag.Apply(err)
		}
		return r.StatusCode, err
	}
	return r.StatusCode, parseResponse(r.Body, result)
}

func (c *Client) post(ctx context.Context, path string, data any, result any) (int, error) {
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(data); err != nil {
		return 200, err
	}
	u := c.gerritURL
	u.Opaque = "//" + u.Host + "/" + path
	r, err := ctxhttp.Post(ctx, c.httpClient, u.String(), contentType, &buffer)
	if err != nil {
		return 0, transient.Tag.Apply(err)
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return 0, transient.Tag.Apply(err)
		}
		err = errors.Fmt("failed to post to %q, status code %d: %s", u.String(), r.StatusCode, strings.TrimSpace(string(b)))
		if r.StatusCode >= 500 {
			err = transient.Tag.Apply(err)
		}
		return r.StatusCode, err
	}
	return r.StatusCode, parseResponse(r.Body, result)
}

func parseResponse(resp io.Reader, result any) error {
	// Strip out the jsonp header, which is ")]}'"
	const gerritPrefix = ")]}'"
	trash := make([]byte, len(gerritPrefix))
	cnt, err := resp.Read(trash)
	if err != nil {
		return errors.Fmt("unexpected response from Gerrit: %w", err)
	}
	if cnt != len(gerritPrefix) || gerritPrefix != string(trash) {
		return errors.New("unexpected response from Gerrit")
	}
	if err = json.NewDecoder(resp).Decode(result); err != nil {
		return errors.Fmt("failed to decode Gerrit response into %T: %w", result, err)
	}
	return nil
}

// pureRevertResponse contains the response fields for calls to get-pure-revert
// api.
type pureRevertResponse struct {
	IsPureRevert bool `json:"is_pure_revert"`
}
