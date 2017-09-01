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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

// Change represents a Gerrit CL. Information about these fields in:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-info
//
// Note that not all fields will be filled for all CLs and queries depending on
// query options, and not all fields exposed by Gerrit are captured by this
// struct. Adding more fields to this struct should be okay (but only
// Gerrit-supported keys will be populated).
type Change struct {
	ChangeNumber           int           `json:"_number"`
	ID                     string        `json:"id"`
	ChangeID               string        `json:"change_id"`
	Project                string        `json:"project"`
	Branch                 string        `json:"branch"`
	Hashtags               []string      `json:"hashtags"`
	Subject                string        `json:"subject"`
	Status                 string        `json:"status"`
	Created                string        `json:"created"`
	Updated                string        `json:"updated"`
	Mergeable              bool          `json:"mergeable"`
	Submitted              string        `json:"submitted"`
	SubmitType             string        `json:"submit_type"`
	Insertions             int           `json:"insertions"`
	Deletions              int           `json:"deletions"`
	UnresolvedCommentCount int           `json:"unresolved_comment_count"`
	HasReviewStarted       bool          `json:"has_review_started"`
	Owner                  AccountInfo   `json:"owner"`
	Submitter              AccountInfo   `json:"submitter"`
	Reviewers              []AccountInfo `json:"reviewers"`
	// MoreChanges is not part of a Change, but Gerrit piggy-backs on the
	// last Change in a page to set this flag if there are more changes
	// in the results of a query.
	MoreChanges bool `json:"_more_changes"`
}

// AccountInfo contains fields associated with a Gerrit account.
type AccountInfo struct {
	AccountID int64 `json:"_account_id"`

	// The following fields will only be populated if the DETAILED_ACCOUNTS
	// option is specified in the query.
	Name     string `json:"name"`
	Email    string `json:"email"`
	UserName string `json:"username"`
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

// ChangeQueryRequest contains the parameters necesary for querying changes from Gerrit.
type ChangeQueryRequest struct {
	// Actual query string, see
	// https://gerrit-review.googlesource.com/Documentation/user-search.html#_search_operators
	Query string
	// How many changes to include in the response.
	N int
	// Skip this many from the list of results (unreliable for paging).
	S int
}

// qs renders the ChangeQueryRequest as a url.Values.
func (qr *ChangeQueryRequest) qs() url.Values {
	qs := url.Values{}
	qs.Add("q", qr.Query)
	if qr.N > 0 {
		qs.Add("n", strconv.Itoa(qr.N))
	}
	if qr.S > 0 {
		qs.Add("S", strconv.Itoa(qr.S))
	}
	return qs
}

// ChangeQuery returns a list of Gerrit changes for a given ChangeQueryRequest.
//
// One example use case for this is getting the CL for a given commit hash.
// Only the .Query property of the qr parameter is required.
//
// Returns a slice of Change, whether there are more changes to fetch
// and an error.
func (c *Client) ChangeQuery(ctx context.Context, qr ChangeQueryRequest) ([]*Change, bool, error) {
	var resp struct {
		Collection []*Change
	}
	if err := c.get(ctx, "changes/", qr.qs(), &resp.Collection); err != nil {
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

func (c *Client) get(ctx context.Context, path string, query url.Values, result interface{}) error {
	u := c.gerritURL
	u.Path = path
	u.RawQuery = query.Encode()
	r, err := ctxhttp.Get(ctx, c.httpClient, u.String())
	if err != nil {
		return transient.Tag.Apply(err)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		err = errors.Reason("failed to fetch %q, status code %d", u, r.StatusCode).Err()
		if r.StatusCode >= 500 {
			// TODO(tandrii): consider retrying.
			err = transient.Tag.Apply(err)
		}
		return err
	}
	// Strip out the jsonp header, which is ")]}'"
	const gerritPrefix = ")]}'"
	trash := make([]byte, len(gerritPrefix))
	cnt, err := r.Body.Read(trash)
	if err != nil {
		return errors.Annotate(err, "unexpected response from Gerrit").Err()
	}
	if cnt != len(gerritPrefix) || gerritPrefix != string(trash) {
		return errors.New("unexpected response from Gerrit")
	}
	if err = json.NewDecoder(r.Body).Decode(result); err != nil {
		return errors.Annotate(err, "failed to decode Gerrit response into %T", result).Err()
	}
	return nil
}
