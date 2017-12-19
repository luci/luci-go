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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const contentType = "application/json; charset=UTF-8"

// Change represents a Gerrit CL. Information about these fields in:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-info
//
// Note that not all fields will be filled for all CLs and queries depending on
// query options, and not all fields exposed by Gerrit are captured by this
// struct. Adding more fields to this struct should be okay (but only
// Gerrit-supported keys will be populated).
type Change struct {
	ChangeNumber           int         `json:"_number"`
	ID                     string      `json:"id"`
	ChangeID               string      `json:"change_id"`
	Project                string      `json:"project"`
	Branch                 string      `json:"branch"`
	Topic                  string      `json:"topic"`
	Hashtags               []string    `json:"hashtags"`
	Subject                string      `json:"subject"`
	Status                 string      `json:"status"`
	Created                string      `json:"created"`
	Updated                string      `json:"updated"`
	Mergeable              bool        `json:"mergeable"`
	Submitted              string      `json:"submitted"`
	SubmitType             string      `json:"submit_type"`
	Insertions             int         `json:"insertions"`
	Deletions              int         `json:"deletions"`
	UnresolvedCommentCount int         `json:"unresolved_comment_count"`
	HasReviewStarted       bool        `json:"has_review_started"`
	Owner                  AccountInfo `json:"owner"`
	Submitter              AccountInfo `json:"submitter"`
	Reviewers              Reviewers   `json:"reviewers"`
	RevertOf               int         `json:"revert_of"`
	CurrentRevision        string      `json:"current_revision"`
	// MoreChanges is not part of a Change, but gerrit piggy-backs on the
	// last Change in a page to set this flag if there are more changes
	// in the results of a query.
	MoreChanges bool `json:"_more_changes"`
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
	return &Client{c, *pu}, nil
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
	if len(result) == 0 {
		return nil, false, nil
	}
	moreChanges := result[len(result)-1].MoreChanges
	result[len(result)-1].MoreChanges = false
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

// ChangeInput contains the parameters necesary for creating a change in Gerrit.
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

// ReviewerInput contians information for adding a reviewer to a change.
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
// TODO(mknyszek): Add support for comments, robot_comments, drafts, notify, and
// omit_duplicate_comments.
type ReviewInput struct {
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
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id
//
// The revisionID parameter may be in any of the forms supported by Gerrit:
//   - "current"
//   - a commit ID
//   - "0" or the literal "edit" for a change edit
//   - etc. See the link below.
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#revision-id
func (c *Client) SetReview(ctx context.Context, changeID string, revisionID string, ri *ReviewInput) (*ReviewResult, error) {
	var resp ReviewResult
	path := fmt.Sprintf("a/changes/%s/revisions/%s/review", url.PathEscape(changeID), url.PathEscape(revisionID))
	if _, err := c.post(ctx, path, ri, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, result interface{}) (int, error) {
	u := c.gerritURL
	u.Path = path
	u.RawQuery = query.Encode()
	r, err := ctxhttp.Get(ctx, c.httpClient, u.String())
	if err != nil {
		return 0, transient.Tag.Apply(err)
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		err = errors.Reason("failed to fetch %q, status code %d", u.String(), r.StatusCode).Err()
		if r.StatusCode >= 500 {
			// TODO(tandrii): consider retrying.
			err = transient.Tag.Apply(err)
		}
		return r.StatusCode, err
	}
	return r.StatusCode, parseResponse(r.Body, result)
}

func (c *Client) post(ctx context.Context, path string, data interface{}, result interface{}) (int, error) {
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(data); err != nil {
		return 200, err
	}
	u := c.gerritURL
	u.Path = path
	r, err := ctxhttp.Post(ctx, c.httpClient, u.String(), contentType, &buffer)
	if err != nil {
		return 0, transient.Tag.Apply(err)
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		err = errors.Reason("failed to post to %q with %v, status code %d", u.String(), data, r.StatusCode).Err()
		if r.StatusCode >= 500 {
			err = transient.Tag.Apply(err)
		}
		return r.StatusCode, err
	}
	return r.StatusCode, parseResponse(r.Body, result)
}

func parseResponse(resp io.Reader, result interface{}) error {
	// Strip out the jsonp header, which is ")]}'"
	const gerritPrefix = ")]}'"
	trash := make([]byte, len(gerritPrefix))
	cnt, err := resp.Read(trash)
	if err != nil {
		return errors.Annotate(err, "unexpected response from Gerrit").Err()
	}
	if cnt != len(gerritPrefix) || gerritPrefix != string(trash) {
		return errors.New("unexpected response from Gerrit")
	}
	if err = json.NewDecoder(resp).Decode(result); err != nil {
		return errors.Annotate(err, "failed to decode Gerrit response into %T", result).Err()
	}
	return nil
}

// pureRevertResponse contains the response fields for calls to get-pure-revert
// api.
type pureRevertResponse struct {
	IsPureRevert bool `json:"is_pure_revert"`
}
