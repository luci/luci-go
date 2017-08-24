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

// Change represents a gerrit CL.
type Change struct {
	ID                     string   `json:"id"`
	Project                string   `json:"project"`
	Branch                 string   `json:"branch"`
	Hashtags               []string `json:"hastags"`
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
	MoreChanges            bool     `json:"_more_changes"`
}

// Owner represents the owner of a change.
type Owner struct {
	AccountID int64 `json:"_account_id"`
}

// ValidateGerritURL validates gerrit URL for use in this package.
func ValidateGerritURL(gerritURL string) error {
	_, err := NormalizeGerritURL(gerritURL)
	return err
}

// NormalizeGerritURL returns canonical for gerrit URL.
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
	if !strings.HasSuffix(u.Host, ".googlesource.com") {
		return "", errors.New("only .googlesource.com gerrits supported")
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

	// Used for testing only.
	mockGerritURL string
}

// QueryRequest contains the parameters necesary for querying changes from Gerrit.
type QueryRequest struct {
	// Actual query string, see https://gerrit-review.googlesource.com/Documentation/user-search.html#_search_operators
	Query string
	// How many changes to include in the response.
	N int
	// Skip these many from the list of results (useful for paging).
	S int
}

// QS renders the QueryRequest in query string format.
func (qr *QueryRequest) QS() string {
	qs := &url.Values{}
	qs.Add("q", qr.Query)
	if qr.N > 0 {
		qs.Add("n", strconv.Itoa(qr.N))
	}
	if qr.S > 0 {
		qs.Add("S", strconv.Itoa(qr.S))
	}
	return qs.Encode()
}

// Query returns a list of gerrit changes for a given string.
//
// One example use case for this is getting the CL for a given commit hash.
// Only the .Query property of the qr parameter is required.
//
// Returns a slice of Change, whether there are more changes to fetch
// and an error.
func (c *Client) Query(ctx context.Context, gerritURL string, qr QueryRequest) ([]Change, bool, error) {
	gerritURL, err := NormalizeGerritURL(gerritURL)
	if err != nil {
		return nil, false, err
	}
	subPath := fmt.Sprintf("%s?%s", "changes/", qr.QS())
	resp := &queryResponse{}
	if err := c.get(ctx, gerritURL, subPath, &resp.Collection); err != nil {
		return nil, false, err
	}
	result := resp.Collection
	if len(result) < 1 {
		return result, false, nil
	}
	return result, result[len(result)-1].MoreChanges, nil
}

//////////////
type queryResponse struct {
	Collection []Change
}

func (c *Client) get(ctx context.Context, gerritURL, subPath string, result interface{}) error {
	URL := fmt.Sprintf("%s/%s", gerritURL, subPath)
	if c.mockGerritURL != "" {
		URL = fmt.Sprintf("%s/%s", c.mockGerritURL, subPath)
	}
	r, err := ctxhttp.Get(ctx, c.Client, URL)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		err = fmt.Errorf("failed to fetch %s, status code %d", URL, r.StatusCode)
		if r.StatusCode >= 500 {
			// TODO(tandrii): consider retrying.
			err = transient.Tag.Apply(err)
		}
		return err
	}
	// Strip out the jsonp header, which is ")]}'"
	trash := make([]byte, 4)
	cnt, err := r.Body.Read(trash)
	if err != nil {
		return errors.Annotate(err, "unexpected response from Gerrit").Err()
	}
	if cnt != 4 || ")]}'" != string(trash) {
		return errors.New("unexpected response from Gerrit")
	}
	if err = json.NewDecoder(r.Body).Decode(result); err != nil {
		return errors.Annotate(err, "failed to decode Gerrit response into %T", result).Err()
	}
	return nil
}
