// Copyright 2021 The LUCI Authors.
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

package submit

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
)

// TreeStatusClient defines the interface that interacts with Tree status App
type TreeStatusClient interface {
	// FetchLatest fetches the latest tree status.
	FetchLatest(ctx context.Context, endpoint string) (TreeStatus, error)
}

// TreeStatus models the status returned by tree status app.
//
// Note that only fields that are needed in CV are included.
// Source of Truth: https://source.chromium.org/chromium/infra/infra/+/master:appengine/chromium_status/appengine_module/chromium_status/status.py;l=209-252;drc=09d53b324dd786b96d69c6cbb8ee6c389b22fd3f
type TreeStatus struct {
	// State describes the Tree state.
	State TreeState
	// Since is the timestamp when the tree obtains the current state.
	Since time.Time
}

// TreeState enumerates possible values for tree state.
//
// Source of Truth: https://source.chromium.org/chromium/infra/infra/+/master:appengine/chromium_status/appengine_module/chromium_status/status.py;l=233-248;drc=09d53b324dd786b96d69c6cbb8ee6c389b22fd3f
type TreeState int8

const (
	TreeStateUnknown TreeState = iota
	TreeOpen
	TreeClosed
	TreeThrottled
	TreeInMaintenance
)

func convertToTreeState(s string) TreeState {
	switch s {
	case "open":
		return TreeOpen
	case "close":
		return TreeClosed
	case "throttled":
		return TreeThrottled
	case "maintenance":
		return TreeInMaintenance
	default:
		return TreeStateUnknown
	}
}

type treeStatusHTTPClient struct {
	*http.Client
}

var (
	client       TreeStatusClient
	initOnce     sync.Once
	clientCtxKey = "go.chromium.org/luci/cv/internal/impl/submit.TreeStatusClient"
)

// InstallProdTreeStatusClient puts a production Tree status client
// implementation into the context
func InstallProdTreeStatusClient(ctx context.Context) context.Context {
	initOnce.Do(func() {
		client = treeStatusHTTPClient{
			&http.Client{
				Transport: &http.Transport{},
			},
		}
	})
	return InstallTreeStatusClient(ctx, client)
}

// InstallTreeStatusClient puts a given `TreeStatusClient` implemeation
// into the context into the context
func InstallTreeStatusClient(ctx context.Context, c TreeStatusClient) context.Context {
	return context.WithValue(ctx, &clientCtxKey, c)
}

// MustTreeStatusClient returns the TreeStatusClient stored in the context.
//
// Panics if not found.
func MustTreeStatusClient(ctx context.Context) TreeStatusClient {
	c, _ := ctx.Value(&clientCtxKey).(TreeStatusClient)
	if c == nil {
		panic("TreeStatusClient not found in the context")
	}
	return c
}

// FetchLatest fetches the latest tree status.
func (c treeStatusHTTPClient) FetchLatest(ctx context.Context, endpoint string) (TreeStatus, error) {
	url := fmt.Sprintf("%s/current?format=json", strings.TrimSuffix(endpoint, "/"))
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return TreeStatus{}, errors.Annotate(err, "failed to create new request").Err()
	}
	resp, err := c.Do(req.WithContext(ctx))
	if err != nil {
		return TreeStatus{}, errors.Annotate(err, "failed to get latest tree status from %s", url).Tag(transient.Tag).Err()
	}
	defer resp.Body.Close()
	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return TreeStatus{}, errors.Annotate(err, "failed to read response body from %s", url).Err()
	}
	if resp.StatusCode >= 400 {
		logging.Errorf(ctx, "received error response when calling %s; response body: %q", url, string(bs))
		return TreeStatus{}, errors.Reason("received error when caling %s", url).Err()
	}
	var raw struct {
		State string `json:"general_state"`
		Date  string
	}
	if err := json.Unmarshal(bs, &raw); err != nil {
		return TreeStatus{}, errors.Annotate(err, "failed to unmarshal JSON %q", string(bs)).Err()
	}
	const dateFormat = "2006-01-02 15:04:05.999999"
	t, err := time.Parse(dateFormat, raw.Date)
	if err != nil {
		return TreeStatus{}, errors.Annotate(err, "failed to parse date %s", raw.Date).Err()
	}
	return TreeStatus{
		State: convertToTreeState(raw.State),
		Since: t,
	}, nil
}
