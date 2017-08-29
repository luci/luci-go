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

// Change represents a gerrit CL. Information about these field in:
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-info
//
// Note that not all fields will be filled for all CLs, and not all fields
// exposed by Gerrit are captured by this struct. Adding more fields to this
// struct should be okay (if gerrit supports them).
type Change struct {
	ID                     string   `json:"id"`
	Project                string   `json:"project"`
	Branch                 string   `json:"branch"`
	Hashtags               []string `json:"hashtags"`
	ChangeID               string   `json:"change_id"`
	Subject                string   `json:"subject"`
	Status                 string   `json:"status"`
	Created                string   `json:"created"`
	Updated                string   `json:"updated"`
	Mergeable              bool     `json:"mergeable"`
	Submitted              string   `json:"submitted"`
	SubmitType             string   `json:"submit_type"`
	Insertions             int      `json:"insertions"`
	Deletions              int      `json:"deletions"`
	UnresolvedCommentCount int      `json:"unresolved_comment_count"`
	HasReviewStarted       bool     `json:"has_review_started"`
	ChangeNumber           int      `json:"_number"`
	Owner                  Owner    `json:"owner"`
	RevertOf               int      `json:"revert_of"`
	CurrentRevision        string   `json:"current_revision"`
	// MoreChanges is not part of a Change, but gerrit piggy-backs on the
	// last Change in a page to set this flag if there are more changes
	// in the results of a query.
	MoreChanges bool `json:"_more_changes"`
}

// Owner represents the owner of a change.
type Owner struct {
	AccountID int64 `json:"_account_id"`
}

// ValidateGerritURL validates gerrit URL for use in this package.
func ValidateGerritURL(gerritURL string) error {
	_, err := NormalizeGerritURL(gerritURL, false)
	return err
}

// NormalizeGerritURL returns canonical for gerrit URL.
//
// error is returned if validation fails.
func NormalizeGerritURL(gerritURL string, allowInsecure bool) (string, error) {
	u, err := url.Parse(gerritURL)
	if err != nil {
		return "", err
	}
	if allowInsecure {
		if u.Scheme != "https" && u.Scheme != "http" {
			return "", fmt.Errorf("%s should start with http(s)://", gerritURL)
		}
	} else {
		if u.Scheme != "https" {
			return "", fmt.Errorf("%s should start with https://", gerritURL)
		}
		if !strings.HasSuffix(u.Host, "-review.googlesource.com") {
			return "", errors.New("only *-review.googlesource.com gerrits supported")
		}
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

// Client is Gerrit client.
type Client struct {
	Client *http.Client

	// Set by NewClient after validation.
	gerritURL url.URL

	// Used for testing only.
	insecure bool
}

// NewClient creates a new instance of Client and validates and stores the URL
// to reach gerrit.
func NewClient(c *http.Client, gerritURL string) (*Client, error) {
	u, err := NormalizeGerritURL(gerritURL, false)
	if err != nil {
		return nil, err
	}
	pu, err := url.Parse(u)
	if err != nil {
		return nil, err
	}
	return &Client{c, *pu, false}, nil
}

// QueryRequest contains the parameters necesary for querying changes from Gerrit.
type QueryRequest struct {
	// Actual query string, see
	// https://gerrit-review.googlesource.com/Documentation/user-search.html#_search_operators
	Query string
	// How many changes to include in the response.
	N int
	// Skip this many from the list of results (useful for paging).
	S int
}

// qs renders the QueryRequest as a url.Values.
func (qr *QueryRequest) qs() url.Values {
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

// Query returns a list of gerrit changes for a given QueryRequest.
//
// One example use case for this is getting the CL for a given commit hash.
// Only the .Query property of the qr parameter is required.
//
// Returns a slice of Change, whether there are more changes to fetch
// and an error.
func (c *Client) Query(ctx context.Context, qr QueryRequest) ([]*Change, bool, error) {
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

// GetDetails gets details about a single change with optional fields.
//
// This method return a single *Change and an error.
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
func (c *Client) GetDetails(ctx context.Context, changeID string, options []string) (*Change, error) {
	resp := &Change{}
	qs := url.Values{}
	if len(options) > 0 {
		for _, o := range options {
			qs.Add("o", o)
		}
	}

	path := fmt.Sprintf("changes/%s/detail", changeID)
	if err := c.get(ctx, path, qs, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, result interface{}) error {
	u := c.gerritURL
	u.Path = path
	u.RawQuery = query.Encode()
	r, err := ctxhttp.Get(ctx, c.Client, u.String())
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
	const gerritPrefix = ")]}'\n"
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
