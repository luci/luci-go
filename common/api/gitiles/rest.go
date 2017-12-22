package gitiles

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"google.golang.org/grpc"

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
// The returned client returns an error if a grpc.CallOption is passed in an
// RPC.
func NewRESTClient(httpClient *http.Client, host string, auth bool) gitiles.GitilesClient {
	baseURL := "https://" + host
	if auth {
		baseURL += "/a"
	}
	return &client{
		Client:  httpClient,
		BaseURL: "https://" + host,
	}
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

type validatable interface {
	Validate() error
}

func checkArgs(opts []grpc.CallOption, req validatable) error {
	if len(opts) > 0 {
		return errors.New("gitiles.client does not grpc options")
	}
	if err := req.Validate(); err != nil {
		return errors.Annotate(err, "request is invalid").Err()
	}
	return nil
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

	ref := req.Treeish
	if req.Ancestor != "" {
		ref = fmt.Sprintf("%s..%s", req.Treeish, req.Ancestor)
	}
	// TODO(tandrii,nodir): s/QueryEscape/PathEscape once AE deployments are Go1.8+.
	path := fmt.Sprintf("/%s/+log/%s", url.QueryEscape(req.Project), url.QueryEscape(ref))
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

	// TODO(tandrii): s/QueryEscape/PathEscape once AE deployments are Go1.8+.
	path := fmt.Sprintf("/%s/+%s", url.QueryEscape(req.Project), url.QueryEscape(refsPath))

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
			continue
		case ref == "HEAD":
			ret.Revisions["HEAD"] = v.Target
		case refsPath != "refs":
			// Gitiles omits refsPath from each ref if refsPath != "refs". Undo this
			// inconsistency.
			ret.Revisions[refsPath+"/"+ref] = v.Value
		default:
			ret.Revisions[ref] = v.Value
		}
	}
	return ret, nil
}

func (c *client) get(ctx context.Context, urlPath string, query url.Values, dest interface{}) error {
	if !strings.HasPrefix(urlPath, "/") {
		panic("urlPath does not start with /")
	}
	if query == nil {
		query = make(url.Values, 1)
	}
	query.Set("format", "JSON")

	u := fmt.Sprintf("%s%s?%s", strings.TrimSuffix(c.BaseURL, "/"), urlPath, query.Encode())
	r, err := ctxhttp.Get(ctx, c.Client, u)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return errors.Annotate(err, "could not read response body").Err()
	}

	if r.StatusCode != 200 {
		err = errors.Reason("failed to fetch %s, status code %d", u, r.StatusCode).
			Tag(errors.TagValue{Key: HTTPStatusTagKey, Value: r.StatusCode}).
			Err()
		switch {
		case r.StatusCode == http.StatusTooManyRequests:
			body, _ := ioutil.ReadAll(r.Body)
			logging.Errorf(ctx, "Gitiles quota error.\nResponse headers: %v\nResponse body: %s",
				r.Header, r, body)
		case r.StatusCode >= 500:
			// TODO(tandrii): consider retrying.
			err = transient.Tag.Apply(err)
		}
		return err
	}

	body = bytes.TrimPrefix(body, jsonPrefix)

	return json.Unmarshal(body, dest)
}
