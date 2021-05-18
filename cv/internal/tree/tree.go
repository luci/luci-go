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

// Package tree implements fetching tree status from Tree Status App.
package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
)

// Client defines the interface that interacts with Tree status App.
type Client interface {
	// FetchLatest fetches the latest tree status.
	FetchLatest(ctx context.Context, endpoint string) (Status, error)
}

// Status models the status returned by tree status app.
//
// Note that only fields that are needed in CV are included.
// Source of Truth: https://source.chromium.org/chromium/infra/infra/+/52a8cfcb436b0012e668630a2f261237046a033a:appengine/chromium_status/appengine_module/chromium_status/status.py;l=209-252
type Status struct {
	// State describes the Tree state.
	State State
	// Since is the timestamp when the tree obtains the current state.
	Since time.Time
}

// State enumerates possible values for tree state.
//
// Source of Truth: https://source.chromium.org/chromium/infra/infra/+/52a8cfcb436b0012e668630a2f261237046a033a:appengine/chromium_status/appengine_module/chromium_status/status.py;l=233-248
type State int8

const (
	StateUnknown State = iota
	Open
	Closed
	Throttled
	InMaintenance
)

func convertToTreeState(s string) State {
	switch s {
	case "open":
		return Open
	case "close":
		return Closed
	case "throttled":
		return Throttled
	case "maintenance":
		return InMaintenance
	default:
		return StateUnknown
	}
}

var clientCtxKey = "go.chromium.org/luci/cv/internal/tree.Client"

// InstallProd puts a production `Client` implementation.
func InstallProd(ctx context.Context) (context.Context, error) {
	t, err := auth.GetRPCTransport(ctx, auth.NoAuth)
	if err != nil {
		return nil, err
	}
	return Install(ctx, httpClientImpl{&http.Client{Transport: t}}), nil
}

// Install puts the given `Client` implementation into the context.
func Install(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, &clientCtxKey, c)
}

// MustClient returns the `Client` implementation stored in the context.
//
// Panics if not found.
func MustClient(ctx context.Context) Client {
	c := ctx.Value(&clientCtxKey)
	if c == nil {
		panic("Tree Status Client not found in the context")
	}
	return c.(Client)
}

// FetchLatest fetches the latest tree status.
//
// This is a shortcut of `tree.mustClient(ctx).FetchLatest(ctx, endpoint)`.
func FetchLatest(ctx context.Context, endpoint string) (Status, error) {
	return MustClient(ctx).FetchLatest(ctx, endpoint)
}

type httpClientImpl struct {
	*http.Client
}

// FetchLatest fetches the latest tree status.
func (c httpClientImpl) FetchLatest(ctx context.Context, endpoint string) (Status, error) {
	url := fmt.Sprintf("%s/current?format=json", strings.TrimSuffix(endpoint, "/"))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return Status{}, errors.Annotate(err, "failed to create new request").Err()
	}
	resp, err := c.Do(req)
	if err != nil {
		return Status{}, errors.Annotate(err, "failed to get latest tree status from %s", url).Tag(transient.Tag).Err()
	}
	defer resp.Body.Close()
	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return Status{}, errors.Annotate(err, "failed to read response body from %s", url).Tag(transient.Tag).Err()
	}
	if resp.StatusCode >= 400 {
		logging.Errorf(ctx, "received error response when calling %s; response body: %q", url, string(bs))
		return Status{}, errors.Reason("received error when caling %s", url).Err()
	}
	var raw struct {
		State string `json:"general_state"`
		Date  string
	}
	if err := json.Unmarshal(bs, &raw); err != nil {
		return Status{}, errors.Annotate(err, "failed to unmarshal JSON %q", string(bs)).Err()
	}
	const dateFormat = "2006-01-02 15:04:05.999999"
	t, err := time.Parse(dateFormat, raw.Date)
	if err != nil {
		return Status{}, errors.Annotate(err, "failed to parse date %s", raw.Date).Err()
	}
	return Status{
		State: convertToTreeState(raw.State),
		Since: t,
	}, nil
}
