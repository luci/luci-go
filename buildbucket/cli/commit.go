// Copyright 2019 The LUCI Authors.
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

package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/context/ctxhttp"

	pb "go.chromium.org/luci/buildbucket/proto"
)

type commitFlag struct {
	commit string
}

func (f *commitFlag) Register(fs *flag.FlagSet, help string) {
	fs.StringVar(&f.commit, "commit", "", help)
}

var regexCommitCRRev = regexp.MustCompile(`crrev\.com/([a-f0-9]+)`)

// retrieveCommit retrieves a GitilesCommit from f.commit.
// Might make RPCs.
// Interacts with the user if necessary.
func (f *commitFlag) retrieveCommit(ctx context.Context, client *http.Client) (*pb.GitilesCommit, error) {
	s := f.commit
	if s == "" {
		return nil, nil
	}

	if m := regexCommitCRRev.FindStringSubmatch(s); m != nil {
		redirect, err := crrevRedirect(ctx, client, m[1])
		if err != nil {
			return nil, fmt.Errorf("crrev.com response for %q: %s", m[1], err)
		}
		s = redirect
	}

	commit, mustConfirmRef, err := parseCommit(s)
	switch {
	case err == errMalformedCommitString:
		return nil, fmt.Errorf("does not match r%q or r%q", regexCommit, regexCommitCRRev)
	case err != nil:
		return nil, fmt.Errorf("invalid -commit: %s", err)

	case mustConfirmRef:
		fmt.Printf("Please confirm the git ref [%s] ", commit.Ref)
		var reply string
		fmt.Scanln(&reply)
		reply = strings.TrimSpace(reply)
		if reply != "" {
			commit.Ref = reply
		}
	}

	return commit, nil
}

var regexCommit = regexp.MustCompile(`([\w\-]+\.googlesource\.com)/([^\+]+)/\+/(([a-f0-9]{40})|([\w\-/\.]+))`)

// crrevRedirect returns a URL that crrev.com redirects the query to.
func crrevRedirect(ctx context.Context, client *http.Client, query string) (redirect string, err error) {
	// API Explorer for this API:
	// https://cr-rev.appspot.com/_ah/api/explorer#p/crrev/v1/crrev.redirect.get
	u := "https://cr-rev.appspot.com/_ah/api/crrev/v1/redirect/" + url.PathEscape(query)
	res, err := ctxhttp.Get(ctx, client, u)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s", res.Status)
	}

	var body struct {
		RedirectURL string `json:"redirect_url"`
	}
	switch err := json.NewDecoder(res.Body).Decode(&body); {
	case err != nil:
		return "", fmt.Errorf("decoding crrev.com response: %s", err)
	case body.RedirectURL == "":
		return "", fmt.Errorf("crrev.com redirected to nothing")
	default:
		return body.RedirectURL, nil
	}
}

var errMalformedCommitString = fmt.Errorf("does not match regexp %q", regexCommit)

// parseCommit tries to retrieve a Gitiles Commit from a string.
//
// It is not strict or precise, and can consume noisy strings, e.g.
// https://chromium.googlesource.com/chromium/src/+/34e1ee99cc34fa86a1c2699977e223a13eb5f96c/chrome/test/chromedriver/test/run_py_tests.py
// If err is nil, returned commit is guaranteed to have Host, Project and
// either Ref or Id.
//
// If mustConfirmRef is true, commit.Ref needs to be confirmed.
func parseCommit(s string) (commit *pb.GitilesCommit, mustConfirmRef bool, err error) {
	m := regexCommit.FindStringSubmatch(s)
	if m == nil {
		return nil, false, errMalformedCommitString
	}
	commit = &pb.GitilesCommit{
		Host:    m[1],
		Project: m[2],
		Id:      m[4],
		Ref:     m[5],
	}

	ref := strings.Split(commit.Ref, "/")
	// Apply heuristic.
	switch {
	case commit.Id != "":
		// This is a precise commit.

	case strings.HasPrefix(commit.Ref, "refs/heads/"):
		// Take one component after refs/heads/
		commit.Ref = strings.Join(ref[:3], "/")
		mustConfirmRef = len(ref) > 3

	case !strings.HasPrefix(commit.Ref, "refs/"):
		// Suspect a branch.
		commit.Ref = "refs/heads/" + ref[0]
		mustConfirmRef = true

	default:
		// The user has specified something with prefix "refs/", but without
		// prefix "refs/heads/". They must know what they is doing.
		mustConfirmRef = true
	}
	return
}
