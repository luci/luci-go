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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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
	if err := ValidateRepoHost(host); err != nil {
		return nil, err
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
	if req.Path != "" {
		path = fmt.Sprintf("%s/%s", path, req.Path)
	}
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
			return nil, status.Errorf(codes.Internal, "could not parse commit %#v: %s", c, err)
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

func (c *client) DownloadFile(ctx context.Context, req *gitiles.DownloadFileRequest, opts ...grpc.CallOption) (*gitiles.DownloadFileResponse, error) {
	if err := checkArgs(opts, req); err != nil {
		return nil, err
	}

	base64Encoded := false
	var format string
	switch req.Format {
	case gitiles.DownloadFileRequest_JSON:
		format = "JSON"
	case gitiles.DownloadFileRequest_TEXT:
		fallthrough
	default:
		format = "TEXT"
		base64Encoded = true
	}

	query := make(url.Values, 1)
	query.Set("format", format)
	ref := strings.TrimRight(req.Committish, "/")
	path := fmt.Sprintf("/%s/+/%s/%s", url.PathEscape(req.Project), url.PathEscape(ref), req.Path)
	_, b, err := c.getRaw(ctx, path, query)
	if err != nil {
		return nil, err
	}

	contents := string(b)
	if base64Encoded {
		d, err := base64.StdEncoding.DecodeString(contents)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to decode response: %s", err)
		}
		contents = string(d)
	}

	return &gitiles.DownloadFileResponse{Contents: contents}, nil
}

func (c *client) DownloadDiff(ctx context.Context, req *gitiles.DownloadDiffRequest, opts ...grpc.CallOption) (*gitiles.DownloadDiffResponse, error) {
	if err := checkArgs(opts, req); err != nil {
		return nil, err
	}
	query := make(url.Values, 1)
	query.Set("format", "TEXT")
	var path string
	if req.Base != "" {
		path = fmt.Sprintf("/%s/+diff/%s..%s/%s", url.PathEscape(req.Project), url.PathEscape(req.Base), url.PathEscape(req.Committish), req.Path)
	} else {
		path = fmt.Sprintf("/%s/+/%s%s/%s", url.PathEscape(req.Project), url.PathEscape(req.Committish), url.PathEscape("^!"), req.Path)
	}
	_, b, err := c.getRaw(ctx, path, query)
	if err != nil {
		return nil, err
	}

	d, err := base64.StdEncoding.DecodeString(string(b))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to decode response: %s", err)
	}

	return &gitiles.DownloadDiffResponse{Contents: string(d)}, nil
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
	path := strings.TrimRight(req.Path, "/")
	if path != "" {
		path = fmt.Sprintf("/%s", path)
	}
	urlPath := fmt.Sprintf("/%s/+archive/%s%s%s", url.PathEscape(req.Project), url.PathEscape(ref), url.PathEscape(path), archiveExtensions[req.Format])
	h, b, err := c.getRaw(ctx, urlPath, nil)
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
		return resp, status.Errorf(codes.Internal, "received too many (%d) filenames for archive", len(filenames))
	}
	return resp, nil
}

func (c *client) Projects(ctx context.Context, req *gitiles.ProjectsRequest, opts ...grpc.CallOption) (*gitiles.ProjectsResponse, error) {
	var resp map[string]project

	if err := c.get(ctx, "/", url.Values{}, &resp); err != nil {
		return nil, err
	}
	ret := &gitiles.ProjectsResponse{}
	for name := range resp {
		ret.Projects = append(ret.Projects, name)
	}
	sort.Strings(ret.Projects)
	return ret, nil
}

func (c *client) ListFiles(ctx context.Context, req *gitiles.ListFilesRequest, opts ...grpc.CallOption) (*gitiles.ListFilesResponse, error) {
	if err := checkArgs(opts, req); err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/%s/+show/%s/%s", url.PathEscape(req.Project), url.PathEscape(req.Committish), req.Path)
	type file struct {
		Mode uint32 `json:"mode"`
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	var data struct {
		Files []file `json:"entries"`
	}
	err := c.get(ctx, path, nil, &data)
	if err != nil {
		return nil, err
	}
	resp := &gitiles.ListFilesResponse{
		Files: make([]*git.File, len(data.Files)),
	}

	for i, f := range data.Files {
		resp.Files[i] = &git.File{
			Mode: f.Mode,
			Id:   f.ID,
			Path: f.Name,
			Type: git.File_Type(git.File_Type_value[strings.ToUpper(f.Type)]),
		}
	}

	return resp, nil
}

func (c *client) get(ctx context.Context, urlPath string, query url.Values, dest any) error {
	if query == nil {
		query = make(url.Values, 1)
	}
	query.Set("format", "JSON")

	_, body, err := c.getRaw(ctx, urlPath, query)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(body, dest); err != nil {
		return status.Errorf(codes.Internal, "could not deserialize response: %s", err)
	}
	return nil
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
		if errors.Contains(err, context.DeadlineExceeded) {
			return http.Header{}, nil, status.Errorf(codes.DeadlineExceeded, "%s", err)
		}
		return http.Header{}, nil, status.Errorf(codes.Unknown, "%s", err)
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return r.Header, nil, status.Errorf(codes.Internal, "could not read response body: %s", err)
	}

	switch r.StatusCode {
	case http.StatusOK:
		return r.Header, bytes.TrimPrefix(body, jsonPrefix), nil

	case http.StatusBadRequest:
		return r.Header, body, status.Errorf(codes.InvalidArgument, "%s", string(body))

	case http.StatusForbidden:
		return r.Header, body, status.Errorf(codes.PermissionDenied, "permission denied")

	case http.StatusNotFound:
		return r.Header, body, status.Errorf(codes.NotFound, "not found")

	case http.StatusTooManyRequests:
		logging.Errorf(ctx, "Gitiles quota error.\nResponse headers: %v\nResponse body: %s", r.Header, body)
		return r.Header, body, status.Errorf(codes.ResourceExhausted, "insufficient Gitiles quota")

	case http.StatusBadGateway:
		return r.Header, body, status.Errorf(codes.Unavailable, "bad gateway")

	case http.StatusServiceUnavailable:
		return r.Header, body, status.Errorf(codes.Unavailable, "service unavailable")

	default:
		logging.Errorf(ctx, "Gitiles: unexpected HTTP %d response.\nResponse headers: %v\nResponse body: %s", r.StatusCode, r.Header, body)
		return r.Header, body, status.Errorf(codes.Internal, "unexpected HTTP %d from Gitiles", r.StatusCode)
	}
}

type validatable interface {
	Validate() error
}

func checkArgs(opts []grpc.CallOption, req validatable) error {
	if len(opts) > 0 {
		return status.Errorf(codes.Internal, "gitiles.client does not support grpc options")
	}
	if err := req.Validate(); err != nil {
		return status.Errorf(codes.InvalidArgument, "request is invalid: %s", err)
	}
	return nil
}
