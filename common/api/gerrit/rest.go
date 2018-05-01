package gerrit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

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

func (c *client) CheckAccess(ctx context.Context, req *gerritpb.CheckAccessRequest, opts ...grpc.CallOption) (
	*gerritpb.CheckAccessResponse, error) {
	if err := checkArgs(opts, req); err != nil {
		return nil, err
	}

	// https://gerrit-review.googlesource.com/Documentation/rest-api-projects.html#access-check-input
	input := map[string]string{
		"account": req.Account,
		"ref":     req.Ref,
	}
	if req.Permission != "" {
		input["permission"] = req.Permission
	}
	var resp struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}
	// TODO(tandrii,nodir): s/QueryEscape/PathEscape once AE deployments are Go1.8+.
	path := fmt.Sprintf("/projects/%s/check.access", url.QueryEscape(req.Project))
	if _, err := c.post(ctx, path, &input, &resp); err != nil {
		return nil, err
	}

	res := &gerritpb.CheckAccessResponse{Reason: resp.Message}
	switch resp.Status {
	case 200:
		res.Status = gerritpb.CheckAccessResponse_ALLOWED
	case 403:
		res.Status = gerritpb.CheckAccessResponse_FORBIDDEN
	case 404:
		res.Status = gerritpb.CheckAccessResponse_PROJECT_NOT_FOUND
	default:
		return nil, status.Errorf(codes.Internal, "unexpected AccessCheckInfo.Status %d response", resp.Status)
	}
	return res, nil
}

// post executes POST request to Gerrit REST API with JSON input/output.
//
// post returns HTTP status code and gRPC error.
// If error happens before HTTP status code was determined, HTTP status code
// will be -1.
func (c *client) post(ctx context.Context, urlPath string, data, dest interface{}, expectedHTTPCodes ...int) (int, error) {
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(data); err != nil {
		return -1, status.Errorf(codes.Internal, "failed to serialize request message: %s", err)
	}
	r, err := ctxhttp.Post(ctx, c.Client, c.BaseURL+urlPath, contentType, &buffer)
	if err != nil {
		return -1, status.Errorf(codes.Internal, "failed to execute Post HTTP request: %s", err)
	}
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return r.StatusCode, status.Errorf(codes.Internal, "failed to read response: %s", err)
	}

	expectedHTTPCodes = append(expectedHTTPCodes, http.StatusOK)
	for _, s := range expectedHTTPCodes {
		if r.StatusCode == s {
			body = bytes.TrimPrefix(body, jsonPrefix)
			if err = json.Unmarshal(body, dest); err != nil {
				return r.StatusCode, status.Errorf(codes.Internal, "failed to desirealize response: %s", err)
			}
			return r.StatusCode, nil
		}
	}

	switch r.StatusCode {
	case http.StatusTooManyRequests:
		logging.Errorf(ctx, "Gerrit quota error.\nResponse headers: %v\nResponse body: %s",
			r.Header, body)
		return r.StatusCode, status.Errorf(codes.ResourceExhausted, "insufficient Gerrit quota")

	case http.StatusForbidden:
		return r.StatusCode, status.Errorf(codes.PermissionDenied, "permission denied")

	case http.StatusNotFound:
		return r.StatusCode, status.Errorf(codes.NotFound, "not found")

	default:
		logging.Errorf(ctx, "gerrit: unexpected HTTP %d response.\nResponse headers: %v\nResponse body: %s",
			r.StatusCode,
			r.Header, body)
		return r.StatusCode, status.Errorf(codes.Internal, "unexpected HTTP %d from Gerrit", r.StatusCode)
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
