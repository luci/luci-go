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

	"github.com/luci/luci-go/server/auth"
	"golang.org/x/net/context"
)

// User is the author or the committer returned from gitiles.
type User struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Time  string `json:"time"`
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

// LogResponse is the JSON response from querying gitiles for a log request.
type LogResponse struct {
	Log  []Commit `json:"log"`
	Next string   `json:"next"`
}

// fixURL validates and normalizes a repoURL and treeish, and returns the
// log JSON gitiles URL.
func fixURL(repoURL, treeish string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("%s should start with https://", repoURL)
	}
	if !strings.HasSuffix(u.Host, ".googlesource.com") {
		return "", fmt.Errorf("Only .googlesource.com repos supported")
	}
	// Use the authenticated URL
	u.Path = "a/" + u.Path
	URL := fmt.Sprintf("%s/+log/%s?format=JSON", u.String(), treeish)
	return URL, nil
}

// Log returns a list of commits based on a repo and treeish (usually
// a branch). This should be equivilent of a "git log <treeish>" call in
// that repository.
func Log(c context.Context, repoURL, treeish string, limit int) ([]Commit, error) {
	// TODO(hinoka): Respect the limit.
	URL, err := fixURL(repoURL, treeish)
	if err != nil {
		return nil, err
	}
	t, err := auth.GetRPCTransport(c, auth.NoAuth)
	if err != nil {
		return nil, err
	}
	client := http.Client{Transport: t}
	r, err := client.Get(URL)
	if err != nil {
		return nil, err
	}
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("Failed to fetch %s, status code %d", URL, r.StatusCode)
	}
	defer r.Body.Close()
	// Strip out the jsonp header, which is ")]}'"
	trash := make([]byte, 4)
	r.Body.Read(trash) // Read the jsonp header
	commits := LogResponse{}
	if err := json.NewDecoder(r.Body).Decode(&commits); err != nil {
		return nil, err
	}
	// TODO(hinoka): If there is a page and we have gotten less than the limit,
	// keep making requests for the next page until we have enough commits.
	return commits.Log, nil
}
