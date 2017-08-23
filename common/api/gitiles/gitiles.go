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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const (
	// MaxCommitsPerRequest is a large but reasonable number of commits to
	// get in a single request.
	MaxCommitsPerRequest = 10000
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
	r, err := c.rawLog(ctx, repoURL, treeish, limit, "")
	if err == nil {
		return r.Log, nil
	}
	return nil, err
}

func (c *Client) rawLog(ctx context.Context, repoURL, treeish string, limit int, nextCursor string) (*logResponse, error) {
	repoURL, err := NormalizeRepoURL(repoURL)
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be at least 1, but %d provided", limit)
	}
	// TODO(tandrii): s/QueryEscape/PathEscape once AE deployments are Go1.8+.
	subPath := fmt.Sprintf("+log/%s?format=JSON", url.QueryEscape(treeish))
	combinedLog := []Commit{}
	nextPath := subPath
	for {
		// Allocate a new log response each time because c.get will not clear resp.Next.
		resp := &logResponse{}
		if nextCursor != "" {
			nextPath = subPath + "&s=" + nextCursor
		}
		if err := c.get(ctx, repoURL, nextPath, resp); err != nil {
			return nil, err
		}
		combinedLog = append(combinedLog, resp.Log...)
		if resp.Next == "" || len(combinedLog) >= limit {
			resp.Log = combinedLog
			if len(resp.Log) > limit {
				resp.Log = resp.Log[:limit]
			}
			// Leave .Next in place
			return resp, nil
		}
		nextCursor = resp.Next
	}
}

// LogForward is a hacky wrapper over rawLog to get a list of commits that goes
// forward instead of backwards.
//
// The response for rawLog will always include (for r1..rx) the immediate
// ancestors of rx (rx-1, rx-2, ..., rx-n) but may not reach r1's child
// necessarily if the delta between the commits reachable from rx and those
// reachablefrom r1 is larger than the a given limit; LogForward will keep
// paging back until it reaches r1's child and will return the last page in
// reverse order (r2, r3, r4, ..., rlimit).
//
// If limit > 0, the list of commits will be truncated at that length.
func (c *Client) LogForward(ctx context.Context, repoURL, r1, rx string, limit int) ([]Commit, error) {
	nextCursor := ""
	treeish := fmt.Sprintf("%s..%s", r1, rx)
	pageSize := MaxCommitsPerRequest
	if limit > pageSize {
		pageSize = limit
	}
	pp := []Commit{}
	for {
		r, err := c.rawLog(ctx, repoURL, treeish, pageSize, nextCursor)
		if err != nil {
			return nil, err
		}
		if r.Next == "" {
			r.Log = append(pp, r.Log...)
			// Reverse in place because golang has no builtins for this.
			for i, j := 0, len(r.Log)-1; i < j; i, j = i+1, j-1 {
				r.Log[i], r.Log[j] = r.Log[j], r.Log[i]
			}
			// Truncate response to limit.
			if limit > 0 && len(r.Log) > limit {
				return r.Log[:limit], nil
			}
			return r.Log, nil
		}
		// Keep the previous page.
		pp = r.Log
		// Keep paging back until r.Next is empty string.
		nextCursor = r.Next
	}
}

// Refs returns a map resolving each ref in a repo to git revision.
//
// refsPath limits which refs to resolve to only those matching {refsPath}/*.
// refsPath should start with "refs" and should not include glob '*'.
// Typically, "refs/heads" should be used.
//
// To fetch **all** refs in a repo, specify just "refs" but beware of two
// caveats:
//  * refs returned include a ref for each patchset for each Gerrit change
//    associated with the repo
//  * returned map will contain special "HEAD" ref whose value in resulting map
//    will be name of the actual ref to which "HEAD" points, which is typically
//    "refs/heads/master".
//
// Thus, if you are looking for all tags and all branches of repo, it's
// recommended to issue two Refs calls limited to "refs/tags" and "refs/heads"
// instead of one call for "refs".
//
// Since Gerrit allows per-ref ACLs, it is possible that some refs matching
// refPrefix would not be present in results because current user isn't granted
// read permission on them.
func (c *Client) Refs(ctx context.Context, repoURL, refsPath string) (map[string]string, error) {
	repoURL, err := NormalizeRepoURL(repoURL)
	if err != nil {
		return nil, err
	}
	if refsPath != "refs" && !strings.HasPrefix(refsPath, "refs/") {
		return nil, fmt.Errorf("refsPath must start with \"refs\": %q", refsPath)
	}
	refsPath = strings.TrimRight(refsPath, "/")

	// TODO(tandrii): s/QueryEscape/PathEscape once AE deployments are Go1.8+.
	subPath := fmt.Sprintf("+%s?format=json", url.QueryEscape(refsPath))
	resp := refsResponse{}
	if err := c.get(ctx, repoURL, subPath, &resp); err != nil {
		return nil, err
	}
	r := make(map[string]string, len(resp))
	for ref, v := range resp {
		switch {
		case v.Value == "":
			// Weird case of what looks like hash with a target in at least Chromium
			// repo.
			continue
		case ref == "HEAD":
			r["HEAD"] = v.Target
		case refsPath != "refs":
			// Gitiles omits refsPath from each ref if refsPath != "refs". Undo this
			// inconsistency.
			r[refsPath+"/"+ref] = v.Value
		default:
			r[ref] = v.Value
		}
	}
	return r, nil
}

////////////////////////////////////////////////////////////////////////////////

type refsResponseRefInfo struct {
	Value  string `json:"value"`
	Target string `json:"target"`
}

// refsResponse is the JSON response from querying gitiles for a Refs request.
type refsResponse map[string]refsResponseRefInfo

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
