// Copyright 2018 The LUCI Authors.
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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/context/ctxhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// OAuthScope is the OAuth 2.0 scope that must be included when acquiring an
// access token for Gerrit RPCs.
const OAuthScope = "https://www.googleapis.com/auth/gerritcodereview"

// This file implements Gerrit proto service client
// on top of Gerrit REST API.

// NewRESTClient creates a new Gerrit client based on Gerrit's REST API.
//
// The host must be a full Gerrit host, e.g. "chromium-review.googlesource.com".
//
// If auth is true, indicates that the given HTTP client sends authenticated
// requests. If so, the requests to Gerrit will include "/a/" URL path
// prefix.
//
// RPC methods of the returned client return an error if a grpc.CallOption is
// passed.
func NewRESTClient(httpClient *http.Client, host string, auth bool) (gerritpb.GerritClient, error) {
	switch {
	case strings.Contains(host, "/"):
		return nil, errors.Reason("invalid host %q", host).Err()
	case !strings.HasSuffix(host, "-review.googlesource.com"):
		return nil, errors.New("Gerrit at googlesource subdomains end with '-review'")
	}

	baseURL := "https://" + host
	if auth {
		baseURL += "/a"
	}
	return &client{Client: httpClient, BaseURL: baseURL}, nil
}

// Implementation.

var jsonPrefix = []byte(")]}'")

// client implements gerritpb.GerritClient.
type client struct {
	Client *http.Client
	// BaseURL is the base URL for all API requests,
	// for example "https://chromium-review.googlesource.com/a".
	BaseURL string
}

// changeInfo is JSON representation of gerritpb.ChangeInfo on the wire.
type changeInfo struct {
	Number  int64                 `json:"_number"`
	Owner   *gerritpb.AccountInfo `json:"owner"`
	Project string                `json:"project"`
}

func (c *client) GetChange(ctx context.Context, req *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (
	*gerritpb.ChangeInfo, error) {

	if err := checkArgs(opts, req); err != nil {
		return nil, err
	}

	var resp changeInfo
	path := fmt.Sprintf("/changes/%d", req.Number)

	params := url.Values{}
	for _, o := range req.Options {
		params.Add("o", o.String())
	}
	if _, err := c.call(ctx, "GET", path, params, nil, &resp); err != nil {
		return nil, err
	}

	return &gerritpb.ChangeInfo{
		Number:  resp.Number,
		Owner:   resp.Owner,
		Project: resp.Project,
	}, nil
}

// call executes a request to Gerrit REST API with JSON input/output.
//
// call returns HTTP status code and gRPC error.
// If error happens before HTTP status code was determined, HTTP status code
// will be -1.
func (c *client) call(ctx context.Context, method, urlPath string, params url.Values, data, dest interface{}, expectedHTTPCodes ...int) (int, error) {
	url := c.BaseURL + urlPath
	if len(params) > 0 {
		url += "?" + params.Encode()
	}

	var buffer bytes.Buffer
	req, err := http.NewRequest(method, url, &buffer)
	if err != nil {
		return 0, status.Errorf(codes.Internal, "failed to create an HTTP request: %s", err)
	}
	if data != nil {
		req.Header.Set("Content-Type", contentType)
		if err := json.NewEncoder(&buffer).Encode(data); err != nil {
			return -1, status.Errorf(codes.Internal, "failed to serialize request message: %s", err)
		}
	}

	res, err := ctxhttp.Do(ctx, c.Client, req)
	if err != nil {
		return -1, status.Errorf(codes.Internal, "failed to execute Post HTTP request: %s", err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, status.Errorf(codes.Internal, "failed to read response: %s", err)
	}

	expectedHTTPCodes = append(expectedHTTPCodes, http.StatusOK)
	for _, s := range expectedHTTPCodes {
		if res.StatusCode == s {
			body = bytes.TrimPrefix(body, jsonPrefix)
			if err = json.Unmarshal(body, dest); err != nil {
				return res.StatusCode, status.Errorf(codes.Internal, "failed to desirealize response: %s", err)
			}
			return res.StatusCode, nil
		}
	}

	switch res.StatusCode {
	case http.StatusTooManyRequests:
		logging.Errorf(ctx, "Gerrit quota error.\nResponse headers: %v\nResponse body: %s",
			res.Header, body)
		return res.StatusCode, status.Errorf(codes.ResourceExhausted, "insufficient Gerrit quota")

	case http.StatusForbidden:
		return res.StatusCode, status.Errorf(codes.PermissionDenied, "permission denied")

	case http.StatusNotFound:
		return res.StatusCode, status.Errorf(codes.NotFound, "not found")

	default:
		logging.Errorf(ctx, "gerrit: unexpected HTTP %d response.\nResponse headers: %v\nResponse body: %s",
			res.StatusCode,
			res.Header, body)
		return res.StatusCode, status.Errorf(codes.Internal, "unexpected HTTP %d from Gerrit", res.StatusCode)
	}
}

type validatable interface {
	Validate() error
}

func checkArgs(opts []grpc.CallOption, req validatable) error {
	if len(opts) > 0 {
		return errors.New("gerrit.client does not support grpc options")
	}
	if err := req.Validate(); err != nil {
		return errors.Annotate(err, "request is invalid").Err()
	}
	return nil
}
