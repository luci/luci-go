// Copyright 2022 The LUCI Authors.
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

// Package gitiles contains logic of interacting with Gitiles.
package gitiles

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util"
)

var MockedGitilesClientKey = "mocked gitiles client"

// GetChangeLogs gets a list of ChangeLogs in revision range by batch.
// The changelogs contain revisions in (startRevision, endRevision]
func GetChangeLogs(c context.Context, repoUrl string, startRevision string, endRevision string) ([]*model.ChangeLog, error) {
	changeLogs := []*model.ChangeLog{}
	gitilesClient := GetClient(c)
	for {
		url := getChangeLogsUrl(repoUrl, startRevision, endRevision)
		params := map[string]string{
			"n":           "100", // Number of revisions to return in each batch
			"name-status": "1",   // We set name-status so the detailed changelogs are return
			"format":      "json",
		}
		data, err := gitilesClient.sendRequest(c, url, params)
		if err != nil {
			return nil, err
		}

		// Gerrit prepends )]}' to json-formatted response.
		// We need to remove it before converting to struct
		prefix := ")]}'\n"
		data = strings.TrimPrefix(data, prefix)

		resp := &model.ChangeLogResponse{}
		if err = json.Unmarshal([]byte(data), resp); err != nil {
			return nil, fmt.Errorf("Failed to unmarshal data %w. Data: %s", err, data)
		}

		changeLogs = append(changeLogs, resp.Log...)
		if resp.Next == "" { // No more revision
			break
		}
		endRevision = resp.Next // Gitiles returns the most recent revision first
	}
	return changeLogs, nil
}

// GetChangeLogsForSingleRevision returns the change log for a single revision
func GetChangeLogsForSingleRevision(c context.Context, repoURL string, revision string) (*model.ChangeLog, error) {
	startRevision := fmt.Sprintf("%s^", revision)
	changeLogs, err := GetChangeLogs(c, repoURL, startRevision, revision)
	if err != nil {
		return nil, err
	}
	if len(changeLogs) != 1 {
		return nil, fmt.Errorf("expected exactly 1 changelog for revision %s. Got %d", revision, len(changeLogs))
	}
	return changeLogs[0], nil
}

// GetParentCommit queries gitiles for the parent commit of a particular commit.
// Parent commit is the commit right before the child commit.
func GetParentCommit(c context.Context, repoURL string, childCommit string) (string, error) {
	startRevision := fmt.Sprintf("%s~2", childCommit)
	endRevision := fmt.Sprintf("%s^", childCommit)
	changeLogs, err := GetChangeLogs(c, repoURL, startRevision, endRevision)
	if err != nil {
		return "", err
	}
	// There should be exactly 1 element in changeLogs
	if len(changeLogs) != 1 {
		return "", fmt.Errorf("couldn't find parent for commit %s. Changelog has %d elements", childCommit, len(changeLogs))
	}
	return changeLogs[0].Commit, nil
}

func GetClient(c context.Context) Client {
	if mockClient, ok := c.Value(MockedGitilesClientKey).(*MockedGitilesClient); ok {
		return mockClient
	}
	return &GitilesClient{}
}

func GetRepoUrl(c context.Context, commit *buildbucketpb.GitilesCommit) string {
	return fmt.Sprintf("https://%s/%s", commit.Host, commit.Project)
}

// getChangeLogsUrl generates a URL for change logs in (startRevision, endRevision]
func getChangeLogsUrl(repoUrl string, startRevision string, endRevision string) string {
	return fmt.Sprintf("%s/+log/%s..%s", repoUrl, startRevision, endRevision)
}

// We need the interface for testing purpose
type Client interface {
	sendRequest(c context.Context, url string, params map[string]string) (string, error)
}

type GitilesClient struct{}

func (cl *GitilesClient) sendRequest(c context.Context, url string, params map[string]string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	for k, v := range params {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()

	logging.Infof(c, "Sending request to gitiles %s", req.URL.String())
	return util.SendHTTPRequest(c, req, 30*time.Second)
}
