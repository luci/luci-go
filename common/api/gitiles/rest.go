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

package gitiles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/context/ctxhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/retry/transient"
)

// This file implements gitiles proto service client
// on top of Gitiles REST API.

// NewRESTClient creates a new Gitiles client based on Gitiles's REST API.
//
// The host must be a full Gitiles host, e.g. "chromium.googlesource.com".
//
// If auth is true, indicates that the given HTTP client sends authenticated
// requests. If so, the requests to Gitiles will include "/a/" URL path
// prefix.
//
// RPC methods of the returned client return an error if a grpc.CallOption is
// passed.
func NewRESTClient(httpClient *http.Client, host string, auth bool) (gitiles.GitilesClient, error) {
	switch {
	case strings.Contains(host, "/"):
		return nil, errors.Reason("invalid host %q", host).Err()
	case !strings.HasSuffix(host, ".googlesource.com"):
		return nil, errors.New("only .googlesource.com hosts are supported")
	}

	baseURL := "https://" + host
	if auth {
		baseURL += "/a"
	}
	return &client{Client: httpClient, BaseURL: baseURL}, nil
}

// Implementation.

var jsonPrefix = []byte(")]}'")

// client implements gitiles.GitilesClient.
type client struct {
	Client *http.Client
	// BaseURL is the base URL for all API requests,
	// for example "https://chromium.googlesource.com/a".
	BaseURL string
}

func (c *client) Log(ctx context.Context, req *gitiles.LogRequest, opts ...grpc.CallOption) (*gitiles.LogResponse, error) {
	if err := checkArgs(opts, req); err != nil {
		return nil, err
	}

	params := url.Values{}
	if req.PageSize > 0 {
		params.Set("n", strconv.FormatInt(int64(req.PageSize), 10))
	}
	if req.TreeDiff {
		params.Set("name-status", "1")
	}
	if req.PageToken != "" {
		params.Set("s", req.PageToken)
	}

	ref := req.Committish
	if req.ExcludeAncestorsOf != "" {
		ref = fmt.Sprintf("%s..%s", req.ExcludeAncestorsOf, req.Committish)
	}
	path := fmt.Sprintf("/%s/+log/%s", url.PathEscape(req.Project), url.PathEscape(ref))
	var resp struct {
		Log  []commit `json:"log"`
		Next string   `json:"next"`
	}
	if err := c.get(ctx, path, params, &resp); err != nil {
		return nil, err
	}

	ret := &gitiles.LogResponse{
		Log:           make([]*git.Commit, len(resp.Log)),
		NextPageToken: resp.Next,
	}
	for i, c := range resp.Log {
		var err error
		ret.Log[i], err = c.Proto()
		if err != nil {
			return nil, errors.Annotate(err, "could not parse commit %#v", c).Err()
		}
	}
	return ret, nil
}

func (c *client) Refs(ctx context.Context, req *gitiles.RefsRequest, opts ...grpc.CallOption) (*gitiles.RefsResponse, error) {
	if err := checkArgs(opts, req); err != nil {
		return nil, err
	}

	refsPath := strings.TrimRight(req.RefsPath, "/")

	path := fmt.Sprintf("/%s/+%s", url.PathEscape(req.Project), url.PathEscape(refsPath))

	resp := map[string]struct {
		Value  string `json:"value"`
		Target string `json:"target"`
	}{}
	if err := c.get(ctx, path, nil, &resp); err != nil {
		return nil, err
	}

	ret := &gitiles.RefsResponse{
		Revisions: make(map[string]string, len(resp)),
	}
	for ref, v := range resp {
		switch {
		case v.Value == "":
			// Weird case of what looks like hash with a target in at least Chromium
			// repo.
		case ref == "HEAD":
			ret.Revisions["HEAD"] = v.Target
		case refsPath != "refs":
			// Gitiles omits refsPath from each ref if refsPath != "refs".
			// Undo this inconsistency.
			ret.Revisions[refsPath+"/"+ref] = v.Value
		default:
			ret.Revisions[ref] = v.Value
		}
	}
	return ret, nil
}

var archiveExtensions = map[gitiles.ArchiveRequest_Format]string{
	gitiles.ArchiveRequest_BZIP2: ".bzip2",
	gitiles.ArchiveRequest_GZIP:  ".tar.gz",
	gitiles.ArchiveRequest_TAR:   ".tar",
	gitiles.ArchiveRequest_XZ:    ".xz",
}

func (c *client) Archive(ctx context.Context, req *gitiles.ArchiveRequest, opts ...grpc.CallOption) (*gitiles.ArchiveResponse, error) {
	if err := checkArgs(opts, req); err != nil {
		return nil, err
	}
	resp := &gitiles.ArchiveResponse{}

	ref := strings.TrimRight(req.Ref, "/")
	path := fmt.Sprintf("/%s/+archive/%s%s", url.PathEscape(req.Project), url.PathEscape(ref), archiveExtensions[req.Format])
	h, b, err := c.getRaw(ctx, path, nil)
	if err != nil {
		return resp, err
	}
	resp.Contents = b

	filenames := h["Filename"]
	switch len(filenames) {
	case 0:
	case 1:
		resp.Filename = filenames[0]
	default:
		return resp, fmt.Errorf("received %d filenames for archive", len(filenames))
	}
	return resp, nil
}

func (c *client) get(ctx context.Context, urlPath string, query url.Values, dest interface{}) error {
	if query == nil {
		query = make(url.Values, 1)
	}
	query.Set("format", "JSON")

	_, body, err := c.getRaw(ctx, urlPath, query)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dest)
}

// getRaw makes a raw HTTP get request and returns the header and body returned.
//
// In case of errors, getRaw translates the generic HTTP errors to grpc errors.
func (c *client) getRaw(ctx context.Context, urlPath string, query url.Values) (http.Header, []byte, error) {
	u := fmt.Sprintf("%s/%s", strings.TrimSuffix(c.BaseURL, "/"), strings.TrimPrefix(urlPath, "/"))
	if query != nil {
		u = fmt.Sprintf("%s?%s", u, query.Encode())
	}
	r, err := ctxhttp.Get(ctx, c.Client, u)
	if err != nil {
		return http.Header{}, []byte{}, transient.Tag.Apply(err)
	}

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return r.Header, []byte{}, errors.Annotate(err, "could not read response body").Err()
	}

	switch r.StatusCode {
	case http.StatusOK:
		return r.Header, bytes.TrimPrefix(body, jsonPrefix), nil

	case http.StatusTooManyRequests:
		logging.Errorf(ctx, "Gitiles quota error.\nResponse headers: %v\nResponse body: %s",
			r.Header, body)
		return r.Header, body, status.Errorf(codes.ResourceExhausted, "insufficient Gitiles quota")

	case http.StatusNotFound:
		return r.Header, body, status.Errorf(codes.NotFound, "not found")

	default:
		logging.Errorf(ctx, "gitiles: unexpected HTTP %d response.\nResponse headers: %v\nResponse body: %s",
			r.StatusCode,
			r.Header, body)
		return r.Header, body, status.Errorf(codes.Internal, "unexpected HTTP %d from Gitiles", r.StatusCode)
	}
}

type validatable interface {
	Validate() error
}

func checkArgs(opts []grpc.CallOption, req validatable) error {
	if len(opts) > 0 {
		return errors.New("gitiles.client does not support grpc options")
	}
	if err := req.Validate(); err != nil {
		return errors.Annotate(err, "request is invalid").Err()
	}
	return nil
}
