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

package prpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/semaphore"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	anypb "github.com/golang/protobuf/ptypes/any"
	spb "google.golang.org/genproto/googleapis/rpc/status"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
)

const (
	// HeaderGRPCCode is a name of the HTTP header that specifies the
	// gRPC code in the response.
	// A pRPC server must always specify it.
	HeaderGRPCCode = "X-Prpc-Grpc-Code"

	// HeaderStatusDetail is a name of the HTTP header that contains
	// elements of google.rpc.Status.details field, one value per element,
	// in the same order.
	// The header value is a standard-base64 string of the encoded google.protobuf.Any,
	// where the message encoding is the same as the response message encoding,
	// i.e. depends on Accept request header.
	HeaderStatusDetail = "X-Prpc-Status-Details-Bin"

	// HeaderTimeout is HTTP header used to set pRPC request timeout.
	// The single value should match regexp `\d+[HMSmun]`.
	HeaderTimeout = "X-Prpc-Grpc-Timeout"

	// DefaultMaxContentLength is the default maximum content length (in bytes)
	// for a Client. It is 32MiB.
	DefaultMaxContentLength = 32 * 1024 * 1024
)

var (
	// DefaultUserAgent is default User-Agent HTTP header for pRPC requests.
	DefaultUserAgent = "pRPC Client 1.0"

	// ErrResponseTooBig is returned by Call when the Response's body size exceeds
	// the Client's MaxContentLength limit.
	ErrResponseTooBig = status.Error(codes.Unavailable, "prpc: response too big")

	// ErrNoStreamingSupport is returned if a pRPC client is used to start a
	// streaming RPC. They are not supported.
	ErrNoStreamingSupport = status.Error(codes.Unimplemented, "prpc: no streaming support")
)

// Client can make pRPC calls.
//
// Changing fields after the first Call(...) is undefined behavior.
type Client struct {
	C       *http.Client // if nil, uses http.DefaultClient
	Host    string       // host and optionally a port number of the target server
	Options *Options     // if nil, DefaultOptions() are used

	// ErrBodySize is the number of bytes to truncate error messages from HTTP
	// responses to.
	//
	// If non-positive, defaults to 256.
	ErrBodySize int

	// MaxContentLength, if > 0, is the maximum content length, in bytes, that a
	// pRPC is willing to read from the server. If a larger content length is
	// present in the response, ErrResponseTooBig will be returned.
	//
	// If <= 0, DefaultMaxContentLength will be used.
	MaxContentLength int

	// MaxConcurrentRequests, if > 0, limits how many requests to the server can
	// execute at the same time. If 0 (default), there's no limit.
	//
	// If there are more concurrent Call(...) calls than the limit, excessive ones
	// will block until there are execution "slots" available or the context is
	// canceled. This waiting does not count towards PerRPCTimeout.
	//
	// The primary purpose of this mechanism is to reduce strain on the local
	// network resources such as number of HTTP connections and HTTP2 streams.
	// Note that it will not help with OOM problems, since blocked calls (and
	// their bodies) all queue up in memory anyway.
	MaxConcurrentRequests int

	// Semaphore to limit concurrency, initialized in concurrencySem().
	semOnce sync.Once
	sem     *semaphore.Weighted

	// testPostHTTP is a test-installed callback that is invoked after an HTTP
	// request finishes.
	testPostHTTP func(context.Context, error) error
}

var _ grpc.ClientConnInterface = (*Client)(nil)

// Invoke performs a unary RPC and returns after the response is received
// into reply.
//
// It is a part of grpc.ClientConnInterface.
func (c *Client) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	// 'method' looks like "/service.Name/MethodName".
	parts := strings.Split(method, "/")
	if len(parts) != 3 || parts[0] != "" {
		return status.Errorf(codes.Internal, "prpc: not a valid method name %q", method)
	}
	serviceName, methodName := parts[1], parts[2]

	// Inputs and outputs must be proto messages.
	in, ok := args.(proto.Message)
	if !ok {
		return status.Errorf(codes.Internal, "prpc: bad argument type %T, not a proto", args)
	}
	out, ok := reply.(proto.Message)
	if !ok {
		return status.Errorf(codes.Internal, "prpc: bad reply type %T, not a proto", reply)
	}

	return c.Call(ctx, serviceName, methodName, in, out, opts...)
}

// NewStream begins a streaming RPC.
//
// It is a part of grpc.ClientConnInterface.
func (c *Client) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, ErrNoStreamingSupport
}

// prepareOptions copies client options and applies opts.
func (c *Client) prepareOptions(opts []grpc.CallOption, serviceName, methodName string) *Options {
	var options *Options
	if c.Options != nil {
		cpy := *c.Options
		options = &cpy
	} else {
		options = DefaultOptions()
	}
	options.apply(opts)
	options.host = c.Host
	options.serviceName = serviceName
	options.methodName = methodName
	if options.UserAgent == "" {
		options.UserAgent = DefaultUserAgent
	}
	return options
}

// Call performs a remote procedure call.
//
// Used by the generated code. Calling from multiple goroutines concurrently
// is safe.
//
// `opts` must be created by this package. Options from google.golang.org/grpc
// package are not supported. Panics if they are used.
//
// Propagates outgoing gRPC metadata provided via metadata.NewOutgoingContext.
// It will be available via metadata.FromIncomingContext on the other side.
// Similarly, if there is a deadline in the Context, it is be propagated
// to the server and applied to the context of the request handler there.
//
// Retries on internal transient errors and on gRPC codes considered transient
// by grpcutil.IsTransientCode. Logs unexpected errors (see ExpectedCode call
// option).
//
// Returns gRPC errors, perhaps with extra structured details if the server
// provided them. Context errors are converted into gRPC errors as well.
// See google.golang.org/grpc/status package.
func (c *Client) Call(ctx context.Context, serviceName, methodName string, in, out proto.Message, opts ...grpc.CallOption) error {
	options := c.prepareOptions(opts, serviceName, methodName)

	// Due to https://github.com/golang/protobuf/issues/745 bug
	// in jsonpb handling of FieldMask, which are typically present in the
	// request, not the response, do request via binary format.
	options.inFormat = FormatBinary
	reqBody, err := proto.Marshal(in)
	if err != nil {
		return status.Errorf(codes.Internal, "prpc: failed to marshal the request: %s", err)
	}

	switch options.AcceptContentSubtype {
	case "", mtPRPCEncodingBinary:
		options.outFormat = FormatBinary
	case mtPRPCEncodingJSONPB:
		options.outFormat = FormatJSONPB
	case mtPRPCEncodingText:
		return status.Errorf(codes.Internal, "prpc: text encoding for pRPC calls is not implemented")
	default:
		return status.Errorf(codes.Internal, "prpc: unrecognized contentSubtype %q of CallAcceptContentSubtype", options.AcceptContentSubtype)
	}

	resp, err := c.call(ctx, options, reqBody)
	if err != nil {
		return err
	}

	switch options.outFormat {
	case FormatBinary:
		err = proto.Unmarshal(resp, out)
	case FormatJSONPB:
		err = jsonpb.UnmarshalString(string(resp), out)
	default:
		panic("impossible")
	}
	if err != nil {
		return status.Errorf(codes.Internal, "prpc: failed to unmarshal the response: %s", err)
	}

	return nil
}

// CallWithFormats is like Call, but sends and returns raw data without
// marshaling it.
//
// Trims JSONPBPrefix from the response if necessary.
func (c *Client) CallWithFormats(ctx context.Context, serviceName, methodName string, in []byte, inf, outf Format, opts ...grpc.CallOption) ([]byte, error) {
	options := c.prepareOptions(opts, serviceName, methodName)
	if options.AcceptContentSubtype != "" {
		return nil, status.Errorf(codes.Internal,
			"prpc: CallAcceptContentSubtype option is not allowed with CallWithFormats "+
				"because input/output formats are already specified")
	}
	options.inFormat = inf
	options.outFormat = outf
	return c.call(ctx, options, in)
}

// call implements Call and CallWithFormats.
func (c *Client) call(ctx context.Context, options *Options, in []byte) ([]byte, error) {
	md, _ := metadata.FromOutgoingContext(ctx)
	req := prepareRequest(options, md, in)
	ctx = logging.SetFields(ctx, logging.Fields{
		"host":    options.host,
		"service": options.serviceName,
		"method":  options.methodName,
	})

	// These are populated below based on the response.
	buf := &bytes.Buffer{}
	contentType := ""

	// Send the request in a retry loop. Use transient.Tag to propagate the retry
	// signal from the loop body.
	err := retry.Retry(ctx, transient.Only(options.Retry), func() (err error) {
		contentType, err = c.attemptCall(ctx, options, req, buf)
		return
	}, func(err error, sleepTime time.Duration) {
		logging.Fields{
			logging.ErrorKey: err,
			"sleepTime":      sleepTime,
		}.Warningf(ctx, "RPC failed transiently. Will retry in %s", sleepTime)
	})

	// Parse the response content type, verify it is what we expect.
	if err == nil {
		switch f, formatErr := FormatFromContentType(contentType); {
		case formatErr != nil:
			err = status.Errorf(codes.Internal, "prpc: bad response content type %q: %s", contentType, formatErr)
		case f != options.outFormat:
			err = status.Errorf(codes.Internal, "prpc: output format (%q) doesn't match expected format (%q)",
				f.MediaType(), options.outFormat.MediaType())
		}
	}

	if err != nil {
		// The context error is more interesting if it is present.
		switch cerr := ctx.Err(); {
		case cerr == context.DeadlineExceeded:
			err = status.Error(codes.DeadlineExceeded, "prpc: overall deadline exceeded")
		case cerr == context.Canceled:
			err = status.Error(codes.Canceled, "prpc: call canceled")
		case cerr != nil:
			err = status.Error(codes.Unknown, cerr.Error())
		}

		// Unwrap the error since we wrap it in attemptCall exclusively to attach
		// a retry signal. call(...) **must** return standard unwrapped gRPC errors.
		err = errors.Unwrap(err)

		// Convert the error into status.Error if it wasn't a status before.
		if status.Code(err) == codes.Unknown {
			err = status.Convert(err).Err()
		}

		// Log only on unexpected codes.
		if code := status.Code(err); code != codes.Canceled {
			ignore := false
			for _, expected := range options.expectedCodes {
				if code == expected {
					ignore = true
					break
				}
			}
			if !ignore {
				logging.WithError(err).Warningf(ctx, "RPC failed permanently: %s", err)
			}
		}

		// Do not return metadata from failed attempts.
		options.resetResponseMetadata()
		return nil, err
	}

	out := buf.Bytes()
	if options.outFormat == FormatJSONPB {
		out = bytes.TrimPrefix(out, bytesJSONPBPrefix)
	}
	return out, nil
}

// concurrencySem returns a semaphore to use to limit concurrency or nil if
// the concurrency is unlimited.
func (c *Client) concurrencySem() *semaphore.Weighted {
	c.semOnce.Do(func() {
		if c.MaxConcurrentRequests > 0 {
			c.sem = semaphore.NewWeighted(int64(c.MaxConcurrentRequests))
		}
	})
	return c.sem
}

// attemptCall makes one attempt at performing an RPC.
//
// Writes the raw response to the provided buffer, returns its content type.
//
// Returns gRPC errors. They may be wrapped and tagged with transient.Tag if
// they should be retried.
func (c *Client) attemptCall(ctx context.Context, options *Options, req *http.Request, buf *bytes.Buffer) (contentType string, err error) {
	// Wait until there's an execution slot available.
	if sem := c.concurrencySem(); sem != nil {
		if err := sem.Acquire(ctx, 1); err != nil {
			return "", status.FromContextError(err).Err()
		}
		defer sem.Release(1)
	}

	// Respect PerRPCTimeout option.
	now := clock.Now(ctx)
	var requestDeadline time.Time
	if options.PerRPCTimeout > 0 {
		requestDeadline = now.Add(options.PerRPCTimeout)
	}

	// Does our parent Context have a deadline?
	if deadline, ok := ctx.Deadline(); ok && (requestDeadline.IsZero() || deadline.Before(requestDeadline)) {
		// Outer Context has a shorter deadline than our per-RPC deadline, so
		// use it.
		requestDeadline = deadline
	} else if !requestDeadline.IsZero() {
		// We have a shorter request deadline. Create a context for this attempt.
		var cancel context.CancelFunc
		ctx, cancel = clock.WithDeadline(ctx, requestDeadline)
		defer cancel()
	}

	// If we have a request deadline, propagate it to the server.
	if !requestDeadline.IsZero() {
		delta := requestDeadline.Sub(now)
		if delta <= 0 {
			// The request has already expired. This will likely never happen, since
			// the outer Retry loop will have expired, but there is a very slight
			// possibility of a race. No need to tag as transient, there's no time
			// left to retry it.
			return "", status.Error(codes.DeadlineExceeded, "prpc: overall deadline exceeded")
		}
		logging.Debugf(ctx, "RPC %s/%s.%s [deadline %s]", options.host, options.serviceName, options.methodName, delta)
		req.Header.Set(HeaderTimeout, EncodeTimeout(delta))
	} else {
		logging.Debugf(ctx, "RPC %s/%s.%s", options.host, options.serviceName, options.methodName)
		req.Header.Del(HeaderTimeout)
	}

	client := c.C
	if client == nil {
		client = http.DefaultClient
	}

	// Send the request.
	req.Body, _ = req.GetBody()
	res, err := client.Do(req.WithContext(ctx))
	if res != nil && res.Body != nil {
		// TODO(vadimsh): Maybe we should drain the body first to enable the
		// connection reuse. Mostly relevant for RPCs aborted via the context
		// cancelation.
		defer res.Body.Close()
	}
	if c.testPostHTTP != nil {
		err = c.testPostHTTP(ctx, err)
	}
	if err != nil {
		return "", transientHTTPError(err, "when sending request")
	}

	if options.resHeaderMetadata != nil {
		*options.resHeaderMetadata = metadataFromHeaders(res.Header)
	}
	if err := c.readResponseBody(ctx, buf, res); err != nil {
		return "", err
	}
	if options.resTrailerMetadata != nil {
		*options.resTrailerMetadata = metadataFromHeaders(res.Trailer)
	}

	// Read the RPC status (perhaps with details). This is nil on success.
	err = c.readStatus(res, buf)
	if grpcutil.IsTransientCode(status.Code(err)) {
		err = transient.Tag.Apply(err)
	}
	return res.Header.Get("Content-Type"), err
}

// readResponseBody copies the response body into dest.
//
// If the response body size exceeds the limits or the declared size, returns
// ErrResponseTooBig. All other errors (including context errors) when reading
// the body are tagged as transient.
func (c *Client) readResponseBody(ctx context.Context, dest *bytes.Buffer, r *http.Response) error {
	limit := c.MaxContentLength
	if limit <= 0 {
		limit = DefaultMaxContentLength
	}

	dest.Reset()
	if l := r.ContentLength; l > 0 {
		if l > int64(limit) {
			logging.Errorf(ctx, "ContentLength header exceeds response body limit: %d > %d.", l, limit)
			return ErrResponseTooBig
		}
		limit = int(l)
		dest.Grow(limit)
	}

	limitedBody := io.LimitReader(r.Body, int64(limit))
	if _, err := dest.ReadFrom(limitedBody); err != nil {
		return transientHTTPError(err, "when reading response")
	}

	// If there is more data in the body Reader, it means that the response
	// size has exceeded our limit.
	var probeB [1]byte
	if n, err := r.Body.Read(probeB[:]); n > 0 || err != io.EOF {
		logging.Errorf(ctx, "Response body limit %d exceeded.", limit)
		return ErrResponseTooBig
	}

	return nil
}

// readStatus retrieves the detailed status from the response.
//
// Puts it into a status.New(...).Err() error.
func (c *Client) readStatus(r *http.Response, bodyBuf *bytes.Buffer) error {
	codeHeader := r.Header.Get(HeaderGRPCCode)
	if codeHeader == "" {
		if r.StatusCode >= 500 {
			// It is possible that the request did not reach the pRPC server and
			// that's why we don't have the code header. It's preferable to convert it
			// to a gRPC status so that the client code treats the response
			// appropriately.
			code := codes.Internal
			if r.StatusCode == http.StatusServiceUnavailable {
				code = codes.Unavailable
			}
			return status.New(code, c.readErrorMessage(bodyBuf)).Err()
		}

		// Not a valid pRPC response.
		return status.Errorf(codes.Internal,
			"prpc: no %s header, HTTP status %d, body: %q",
			HeaderGRPCCode, r.StatusCode, c.readErrorMessage(bodyBuf))
	}

	code, err := strconv.Atoi(codeHeader)
	if err != nil {
		return status.Errorf(codes.Internal, "prpc: invalid %s header value %q", HeaderGRPCCode, codeHeader)
	}

	if codes.Code(code) == codes.OK {
		return nil
	}

	sp := &spb.Status{
		Code:    int32(code),
		Message: strings.TrimSuffix(c.readErrorMessage(bodyBuf), "\n"),
	}
	if sp.Details, err = c.readStatusDetails(r); err != nil {
		return err
	}
	return status.FromProto(sp).Err()
}

// readErrorMessage reads an error message from a body buffer.
//
// Respects c.ErrBodySize. If the error message is too long, trims it and
// appends "...".
func (c *Client) readErrorMessage(bodyBuf *bytes.Buffer) string {
	ret := bodyBuf.Bytes()

	// Apply limits.
	limit := c.ErrBodySize
	if limit <= 0 {
		limit = 256
	}
	if len(ret) > limit {
		// Note: we can't use strings.ToValidUTF8 here yet because it exists only
		// in go1.13 and this code still needs to execute on go1.11 (for GAE). See
		// also https://stackoverflow.com/a/52784785.
		return strings.Map(func(r rune) rune {
			if r == utf8.RuneError {
				return -1
			}
			return r
		}, string(ret[:limit])) + "..."
	}

	return string(ret)
}

// readStatusDetails reads google.rpc.Status.details from the response headers.
//
// Returns gRPC errors.
func (c *Client) readStatusDetails(r *http.Response) ([]*anypb.Any, error) {
	values := r.Header[HeaderStatusDetail]
	if len(values) == 0 {
		return nil, nil
	}

	ret := make([]*anypb.Any, len(values))
	var buf []byte
	for i, v := range values {
		sz := base64.StdEncoding.DecodedLen(len(v))
		if cap(buf) < sz {
			buf = make([]byte, sz)
		}

		n, err := base64.StdEncoding.Decode(buf[:sz], []byte(v))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "prpc: invalid header %s: %q", HeaderStatusDetail, v)
		}

		msg := &anypb.Any{}
		if err := proto.Unmarshal(buf[:n], msg); err != nil {
			return nil, status.Errorf(codes.Internal, "prpc: failed to unmarshal status detail: %s", err)
		}
		ret[i] = msg
	}

	return ret, nil
}

// prepareRequest creates an HTTP request for an RPC.
//
// Initializes GetBody, so that the request can be resent multiple times when
// retrying.
func prepareRequest(options *Options, md metadata.MD, body []byte) *http.Request {
	// Convert headers to HTTP canonical form (i.e. Title-Case). Extract Host
	// header, it is special and must be passed via http.Request.Host. Preallocate
	// 5 more slots (for 4 headers below and for the RPC deadline header).
	headers := make(http.Header, len(md)+5)
	hostHdr := ""
	for key, vals := range md {
		if len(vals) == 0 {
			continue
		}
		key := http.CanonicalHeaderKey(key)
		if key == "Host" {
			hostHdr = vals[0]
		} else {
			headers[key] = vals
		}
	}

	// Add protocol-related headers.
	headers.Set("Content-Type", options.inFormat.MediaType())
	headers.Set("Accept", options.outFormat.MediaType())
	headers.Set("User-Agent", options.UserAgent)
	headers.Set("Content-Length", strconv.Itoa(len(body)))
	// TODO(nodir): add "Accept-Encoding: gzip" when pRPC server supports it.

	scheme := "https"
	if options.Insecure {
		scheme = "http"
	}

	return &http.Request{
		Method: "POST",
		URL: &url.URL{
			Scheme: scheme,
			Host:   options.host,
			Path:   fmt.Sprintf("/prpc/%s/%s", options.serviceName, options.methodName),
		},
		Host:          hostHdr,
		Header:        headers,
		ContentLength: int64(len(body)),
		GetBody: func() (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewReader(body)), nil
		},
	}
}

// metadataFromHeaders copies an http.Header object into a metadata.MD map.
//
// In order to conform with gRPC, which relies on HTTP/2's forced lower-case
// headers, we convert the headers to lower-case before entering them in the
// metadata map.
func metadataFromHeaders(h http.Header) metadata.MD {
	if len(h) == 0 {
		return nil
	}

	md := make(metadata.MD, len(h))
	for k, v := range h {
		md[strings.ToLower(k)] = v
	}
	return md
}

// transientHTTPError transforms and annotates errors from http.Client.
//
// Tries to recognize context errors.
func transientHTTPError(err error, msg string) error {
	switch errors.Unwrap(err) {
	case context.DeadlineExceeded:
		err = status.Errorf(codes.DeadlineExceeded, "prpc: %s: attempt deadline exceeded", msg)
	case context.Canceled:
		err = status.Errorf(codes.Canceled, "prpc: %s: attempt canceled", msg)
	default:
		err = status.Errorf(codes.Internal, "prpc: %s: %s", msg, err)
	}
	return transient.Tag.Apply(err)
}
