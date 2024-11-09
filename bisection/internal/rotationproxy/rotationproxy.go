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

// Package rotationproxy contains the logic to query for the on-call arborists
// from the Chrome Ops Rotation Proxy
package rotationproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/bisection/util"
)

var MockedRotationProxyClientKey = "mocked chrome-ops-rotation-proxy client"

// GetOnCallEmails returns the emails of the on-call arborists for the given
// project.
func GetOnCallEmails(ctx context.Context, project string) ([]string, error) {
	switch project {
	case "chromium/src":
		return getOnCallEmails(ctx, "oncallator:chrome-build-sheriff")
	default:
		// getting on-call rotation not supported
	}

	return nil, fmt.Errorf("could not get on-call rotation for project %s", project)
}

type rotationResponse struct {
	Emails           []string `json:"emails"`
	UpdatedTimestamp int64    `json:"updated_unix_timestamp"`
}

// getOnCallEmails is a helper function to get the emails of the on-call
// arborists using the given rotation proxy name.
func getOnCallEmails(ctx context.Context, rotationProxyName string) ([]string, error) {
	client := GetClient(ctx)
	data, err := client.sendRequest(ctx, rotationProxyName)
	if err != nil {
		return nil, errors.Annotate(err,
			"error when querying for on-call rotation").Err()
	}

	res := &rotationResponse{}
	if err = json.Unmarshal([]byte(data), res); err != nil {
		return nil, errors.Annotate(err,
			"failed to unmarshal rotation response (data = %s)", data).Err()
	}

	return res.Emails, nil
}

func GetClient(c context.Context) Client {
	if mockClient, ok := c.Value(MockedRotationProxyClientKey).(*MockedRotationProxyClient); ok {
		return mockClient
	}

	return &RotationProxyClient{}
}

// Client interface is needed for testing purposes
type Client interface {
	sendRequest(ctx context.Context, rotationProxyName string) (string, error)
}

type RotationProxyClient struct{}

func (client *RotationProxyClient) sendRequest(ctx context.Context, rotationProxyName string) (string, error) {
	url := fmt.Sprintf("https://chrome-ops-rotation-proxy.appspot.com/current/%s", rotationProxyName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", errors.Annotate(err,
			"failed to construct request when getting on-call rotation with name '%s'", rotationProxyName).Err()
	}

	// Get the on-call rotation (timeout of 30s)
	return util.SendHTTPRequest(ctx, req, 30*time.Second)
}
