// Copyright 2016 The LUCI Authors.
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

package gitiles

// TODO(tandrii): add tests.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/retry/transient"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

// User is the author or the committer returned from gitiles.
type User struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Time  string `json:"time"`
}

// GetTime returns the Time field as real data!
func (u *User) GetTime() (time.Time, error) {
	return time.Parse(time.ANSIC, u.Time)
}

// Commit is the information of a commit returned from gitiles.
type Commit struct {
	Commit    string   `json:"commit"`
	Tree      string   `json:"tree"`
	Parents   []string `json:"parents"`
	Author    User     `json:"author"`
	Committer User     `json:"committer"`
	Message   string   `json:"message"`
}

// ValidateRepoURL validates gitiles repository URL for use in this package.
func ValidateRepoURL(repoURL string) error {
	_, err := NormalizeRepoURL(repoURL)
	return err
}

// NormalizeRepoURL returns canonical for gitiles URL of the repo including "a/" path prefix.
// error is returned if validation fails.
func NormalizeRepoURL(repoURL string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("%s should start with https://", repoURL)
	}
	if !strings.HasSuffix(u.Host, ".googlesource.com") {
		return "", errors.New("only .googlesource.com repos supported")
	}
	if u.Fragment != "" {
		return "", errors.New("no fragments allowed in repoURL")
	}
	if u.Path == "" || u.Path == "/" {
		return "", errors.New("path to repo is empty")
	}
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	if !strings.HasPrefix(u.Path, "/a/") {
		// Use the authenticated URL
		u.Path = "/a" + u.Path
	}

	u.Path = strings.TrimRight(u.Path, "/")
	u.Path = strings.TrimSuffix(u.Path, ".git")
	return u.String(), nil
}

// Client is Gitiles client.
type Client struct {
	Client *http.Client

	// Used for testing only.
	mockRepoURL string
}

// Log returns a list of commits based on a repo and treeish.
// This should be equivalent of a "git log <treeish>" call in that repository.
//
// treeish can be either:
//   (1) a git revision as 40-char string or its prefix so long as its unique in repo.
//   (2) a ref such as "refs/heads/branch" or just "branch"
//   (3) a ref defined as n-th parent of R in the form "R~n".
//       For example, "master~2" or "deadbeef~1".
//   (4) a range between two revisions in the form "CHILD..PREDECESSOR", where
//       CHILD and PREDECESSOR are each specified in either (1), (2) or (3)
//       formats listed above.
//       For example, "foo..ba1", "master..refs/branch-heads/1.2.3",
//         or even "master~5..master~9".
//
//
// If the returned log has a commit with 2+ parents, the order of commits after
// that is whatever Gitiles returns, which currently means ordered
// by topological sort first, and then by commit timestamps.
//
// This means that if Log(C) contains commit A, Log(A) will not necessarily return
// a subsequence of Log(C) (though definitely a subset). For example,
//
//     common... -> base ------> A ----> C
//                       \            /
//                        --> B ------
//
//     ----commit timestamp increases--->
//
//  Log(A) = [A, base, common...]
//  Log(B) = [B, base, common...]
//  Log(C) = [C, A, B, base, common...]
//
func (c *Client) Log(ctx context.Context, repoURL, treeish string, limit int) ([]Commit, error) {
	repoURL, err := NormalizeRepoURL(repoURL)
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be at least 1, but %d provided", limit)
	}
	subPath := fmt.Sprintf("+log/%s?format=JSON", url.PathEscape(treeish))
	resp := &logResponse{}
	if err := c.get(ctx, repoURL, subPath, resp); err != nil {
		return nil, err
	}
	result := resp.Log
	for {
		if resp.Next == "" || len(result) >= limit {
			if len(result) > limit {
				result = result[:limit]
			}
			return result, nil
		}
		nextPath := subPath + "&s=" + resp.Next
		resp = &logResponse{}
		if err := c.get(ctx, repoURL, nextPath, resp); err != nil {
			return nil, err
		}
		result = append(result, resp.Log...)
	}
}

////////////////////////////////////////////////////////////////////////////////

// logResponse is the JSON response from querying gitiles for a log request.
type logResponse struct {
	Log  []Commit `json:"log"`
	Next string   `json:"next"`
}

func (c *Client) get(ctx context.Context, repoURL, subPath string, result interface{}) error {
	URL := fmt.Sprintf("%s/%s", repoURL, subPath)
	if c.mockRepoURL != "" {
		URL = fmt.Sprintf("%s/%s", c.mockRepoURL, subPath)
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
		return errors.Annotate(err, "unexpected response from Gitiles").Err()
	}
	if cnt != 4 || ")]}'" != string(trash) {
		return errors.New("unexpected response from Gitiles")
	}
	if err = json.NewDecoder(r.Body).Decode(result); err != nil {
		return errors.Annotate(err, "failed to decode Gitiles response into %T", result).Err()
	}
	return nil
}
