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
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/sync/semaphore"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc/prpcpb"
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

	// HeaderMaxResponseSize is HTTP request header with the maximum response
	// size the client will accept.
	HeaderMaxResponseSize = "X-Prpc-Max-Response-Size"

	// DefaultMaxResponseSize is the default maximum response size (in bytes)
	// the client is willing to read from the server.
	//
	// It is 32MiB minus 32KiB. Its value is picked to fit into Appengine response
	// size limits (taking into account potential overhead on headers).
	DefaultMaxResponseSize = 32*1024*1024 - 32*1024

	// UnlimitedMaxResponseSize can be used as Client's MaxResponseSize to remove
	// the limit on maximum acceptable response size: any response will be
	// accepted (with the risk of OOMing the client).
	UnlimitedMaxResponseSize = math.MaxInt
)

var (
	// DefaultUserAgent is default User-Agent HTTP header for pRPC requests.
	DefaultUserAgent = "pRPC Client 1.5"

	// ErrNoStreamingSupport is returned if a pRPC client is used to start a
	// streaming RPC. They are not supported.
	ErrNoStreamingSupport = status.Error(codes.Unimplemented, "prpc: no streaming support")
)

// clientFactoryCtxKey is used for the context.Context key.
var clientFactoryCtxKey = "prpc client factory"

// Client can make pRPC calls.
//
// Changing fields after the first Call(...) is undefined behavior.
type Client struct {
	// Host is a hostname (i.e. without "http://" or "https://") and optionally
	// a port number of the target server.
	//
	// Required.
	Host string

	// C is an http.Client to use for making HTTP requests.
	//
	// If nil, some default client will be used (usually http.DefaultClient, but
	// it can be changed by installing a custom client provider into the per-RPC
	// context via SetDefaultHTTPClient).
	C *http.Client

	// Options allow to fine-tune behavior of the client.
	//
	// if nil, DefaultOptions() are used.
	Options *Options

	// ErrBodySize is the number of bytes to truncate error messages from HTTP
	// responses to.
	//
	// If non-positive, defaults to 4096.
	ErrBodySize int

	// MaxResponseSize, if > 0, is the maximum response size, in bytes, that a
	// pRPC client is willing to read from the server. If a larger response is
	// sent by the server, the client will return UNAVAILABLE error with some
	// additional details attached (use ProtocolErrorDetails to extract them
	// from a gRPC error).
	//
	// This value is also sent to the server in a request header. The server MAY
	// use it to skip sending too big response. Instead the server will reply
	// with the same sort of UNAVAILABLE error (with details attached as well).
	//
	// If <= 0, DefaultMaxResponseSize will be used. Use UnlimitedMaxResponseSize
	// to disable the limit (with the risk of OOMing the client).
	MaxResponseSize int

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

	// EnableRequestCompression allows the client to compress requests if they
	// are larger than a certain threshold.
	//
	// This is false by default. Use this option only with servers that understand
	// compressed requests! These are Go servers built after Aug 15 2022. Python
	// servers and olders Go servers would fail to parse the request with
	// INVALID_ARGUMENT error.
	//
	// The response compression is configured independently on the server. The
	// client always accepts compressed responses.
	EnableRequestCompression bool

	// PathPrefix is the prefix of the URL path, "<PathPrefix>/<service>/<method>"
	// when making HTTP requests. If not set, defaults to "/prpc".
	PathPrefix string

	// Semaphore to limit concurrency, initialized in concurrencySem().
	semOnce sync.Once
	sem     *semaphore.Weighted

	// testPostHTTP is a test-installed callback that is invoked after an HTTP
	// request finishes.
	testPostHTTP func(context.Context, error) error
}

var _ grpc.ClientConnInterface = (*Client)(nil)

// prpcAuthInfo implements credentials.AuthInfo.
type prpcAuthInfo struct {
	credentials.CommonAuthInfo
}

// AuthType implements credentials.AuthInfo.
func (prpcAuthInfo) AuthType() string {
	return "prpc"
}

// Invoke performs a unary RPC and returns after the response is received
// into reply.
//
// It is a part of grpc.ClientConnInterface.
func (c *Client) Invoke(ctx context.Context, method string, args any, reply any, opts ...grpc.CallOption) error {
	// 'method' looks like "/service.Name/MethodName".
	parts := strings.Split(method, "/")
	if len(parts) != 3 || parts[0] != "" {
		return status.Errorf(codes.Internal, "prpc: not a valid method name %q", method)
	}
	serviceName, methodName := parts[1], parts[2]

	// Inputs and outputs must be proto messages.
	req, ok := args.(proto.Message)
	if !ok {
		return status.Errorf(codes.Internal, "prpc: bad argument type %T, not a proto", args)
	}
	resp, ok := reply.(proto.Message)
	if !ok {
		return status.Errorf(codes.Internal, "prpc: bad reply type %T, not a proto", reply)
	}

	return c.Call(ctx, serviceName, methodName, req, resp, opts...)
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

// requestCodec decides what codec to use to serialize requests, per format.
func (c *Client) requestCodec(f Format) (protoCodec, error) {
	switch f {
	case FormatBinary:
		return codecWireV2, nil
	case FormatJSONPB:
		return codecJSONV2, nil
	case FormatText:
		return codecTextV2, nil
	default:
		return 0, status.Errorf(codes.Internal, "prpc: unrecognized request format %v", f)
	}
}

// responseCodec decides what codec to use to deserialize responses, per format.
func (c *Client) responseCodec(f Format) (protoCodec, error) {
	switch f {
	case FormatBinary:
		return codecWireV2, nil
	case FormatJSONPB:
		return codecJSONV2, nil
	case FormatText:
		return codecTextV2, nil
	default:
		return 0, status.Errorf(codes.Internal, "prpc: unrecognized response format %v", f)
	}
}

// Call performs a remote procedure call.
//
// Used by the generated code. Calling from multiple goroutines concurrently
// is safe.
//
// `opts` may include pRPC-specific options (like prpc.ExpectedCode), as well as
// some of gRPC standard options (but not all). Passing an unsupported gRPC call
// option will cause a panic.
//
// Following gRPC options are supported:
//   - grpc.Header
//   - grpc.Trailer
//   - grpc.PerRPCCredentials
//   - grpc.StaticMethod
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
func (c *Client) Call(ctx context.Context, serviceName, methodName string, req, resp proto.Message, opts ...grpc.CallOption) error {
	options := c.prepareOptions(opts, serviceName, methodName)

	var err error
	if options.reqCodec, err = c.requestCodec(options.RequestFormat); err != nil {
		return err
	}
	if options.respCodec, err = c.responseCodec(options.ResponseFormat); err != nil {
		return err
	}

	reqBody, err := options.reqCodec.Encode(nil, req)
	if err != nil {
		return status.Errorf(codes.Internal, "prpc: failed to marshal the request: %s", err)
	}
	respBody, err := c.call(ctx, options, reqBody)
	if err != nil {
		return err
	}
	if err := options.respCodec.Decode(respBody, resp); err != nil {
		return status.Errorf(codes.Internal, "prpc: failed to unmarshal the response: %s", err)
	}
	return nil
}

// CallWithFormats is like Call, but sends and returns raw data without
// marshaling it.
//
// Ignores RequestFormat and ResponseFormat options in favor of explicitly
// passed formats. They are used to set corresponding headers on the request.
//
// Trims JSONPBPrefix from the response if necessary.
func (c *Client) CallWithFormats(ctx context.Context, serviceName, methodName string, req []byte, reqFormat, respFormat Format, opts ...grpc.CallOption) ([]byte, error) {
	options := c.prepareOptions(opts, serviceName, methodName)
	var err error
	if options.reqCodec, err = c.requestCodec(reqFormat); err != nil {
		return nil, err
	}
	if options.respCodec, err = c.responseCodec(respFormat); err != nil {
		return nil, err
	}
	return c.call(ctx, options, req)
}

// call implements Call and CallWithFormats.
func (c *Client) call(ctx context.Context, options *Options, req []byte) ([]byte, error) {
	md, _ := metadata.FromOutgoingContext(ctx)
	httpReq, err := c.prepareRequest(options, md, req)
	if err != nil {
		return nil, err
	}
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
	err = retry.Retry(ctx, transient.Only(options.Retry), func() (err error) {
		// Note: `buf` is reset inside, it is safe to reuse it across attempts.
		contentType, err = c.attemptCall(ctx, options, httpReq, buf)
		if err == nil {
			return
		}
		// Do not retry on some protocol errors, regardless of the status code.
		if details := ProtocolErrorDetails(err); details != nil {
			switch details.Error.(type) {
			// Retying is unlikely to reduce the response size.
			case *prpcpb.ErrorDetails_ResponseTooBig:
				return
			}
		}
		// Retry on regular transient errors and on per-RPC deadline. If this is
		// a global deadline (i.e. `ctx` expired), the retry loop will just exit.
		return grpcutil.WrapIfTransientOr(err, codes.DeadlineExceeded)
	}, func(err error, sleepTime time.Duration) {
		logging.Fields{
			"sleepTime": sleepTime,
		}.Warningf(ctx, "RPC failed transiently (retry in %s): %s", sleepTime, err)
	})

	// Parse the response content type, verify it is what we expect.
	if err == nil {
		switch f, formatErr := FormatFromContentType(contentType); {
		case formatErr != nil:
			err = status.Errorf(codes.Internal, "prpc: bad response content type %q: %s", contentType, formatErr)
		case f != options.respCodec.Format():
			err = status.Errorf(codes.Internal, "prpc: output format (%q) doesn't match expected format (%q)",
				f.MediaType(), options.respCodec.Format().MediaType())
		}
	}

	if err != nil {
		// The context error is more interesting if it is present.
		switch cerr := ctx.Err(); {
		case cerr == context.DeadlineExceeded:
			err = status.Errorf(codes.DeadlineExceeded, "prpc: overall deadline exceeded: %s", context.Cause(ctx))
		case cerr == context.Canceled:
			err = status.Errorf(codes.Canceled, "prpc: call canceled: %s", context.Cause(ctx))
		case cerr != nil:
			err = status.Error(codes.Unknown, cerr.Error())
		}

		// Unwrap the error since we wrap it in retry.Retry exclusively to attach
		// a retry signal. call(...) **must** return standard unwrapped gRPC errors.
		err = errors.Unwrap(err)

		// Convert the error into status.Error (with Unknown code) if it wasn't
		// a status before.
		if status, ok := status.FromError(err); !ok {
			err = status.Err()
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
				logging.Warningf(ctx, "RPC failed permanently: %s", err)
				if options.Debug {
					if code == codes.InvalidArgument && strings.Contains(err.Error(), "could not decode body") {
						logging.Warningf(ctx, "Original request size: %d", len(req))
						logging.Warningf(ctx, "Content-type: %s", options.reqCodec.Format().MediaType())
						b64 := base64.StdEncoding.EncodeToString(req)
						logging.Warningf(ctx, "Original request in base64 encoding: %s", b64)
					}
				}
			}
		}

		// Do not return metadata from failed attempts.
		options.resetResponseMetadata()
		return nil, err
	}

	out := buf.Bytes()
	if options.respCodec.Format() == FormatJSONPB {
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
// Returns gRPC errors.
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

	// On errors prefer the context error if the per-RPC context expired. It is
	// a more consistent representation of what is happening. The other error is
	// more chaotic, depending on when exactly the context expires.
	defer func() {
		if err != nil {
			switch cerr := ctx.Err(); {
			case cerr == context.DeadlineExceeded:
				err = status.Errorf(codes.DeadlineExceeded, "prpc: attempt deadline exceeded: %s", context.Cause(ctx))
			case cerr == context.Canceled:
				err = status.Errorf(codes.Canceled, "prpc: attempt canceled: %s", context.Cause(ctx))
			case cerr != nil:
				err = status.Error(codes.Unknown, cerr.Error())
			}
		}
	}()

	// If we have a request deadline, propagate it to the server.
	if !requestDeadline.IsZero() {
		delta := requestDeadline.Sub(now)
		if delta <= 0 {
			// The request has already expired. This will likely never happen, since
			// the outer Retry loop will have expired, but there is a very slight
			// possibility of a race.
			return "", status.Errorf(codes.DeadlineExceeded, "prpc: attempt deadline exceeded: %s", context.Cause(ctx))
		}
		logging.Debugf(ctx, "RPC %s/%s.%s [deadline %s]", options.host, options.serviceName, options.methodName, delta)
		req.Header.Set(HeaderTimeout, EncodeTimeout(delta))
	} else {
		logging.Debugf(ctx, "RPC %s/%s.%s", options.host, options.serviceName, options.methodName)
		req.Header.Del(HeaderTimeout)
	}

	// Grab fresh per-RPC credentials. Make sure not to override req.Header, since
	// original headers may be needed if the call is retried.
	if options.PerRPCCredentials != nil {
		mdctx := credentials.NewContextWithRequestInfo(ctx, credentials.RequestInfo{
			Method: fmt.Sprintf("/%s/%s", options.serviceName, options.methodName),
			AuthInfo: prpcAuthInfo{
				CommonAuthInfo: credentials.CommonAuthInfo{
					// Note: this is not true when using "insecure: true" pRPC mode, but
					// fixing this will break random local tests. "insecure: true" is used
					// only locally in tests, this should be fine.
					SecurityLevel: credentials.PrivacyAndIntegrity,
				},
			},
		})
		md, err := options.PerRPCCredentials.GetRequestMetadata(mdctx, req.URL.String())
		if err != nil {
			return "", status.Errorf(codeForErr(err), "prpc: getting per-RPC credentials: %s", err)
		}
		if len(md) != 0 {
			orig := req.Header
			req.Header = req.Header.Clone()
			defer func() { req.Header = orig }()
			for k, v := range md {
				req.Header.Set(k, v)
			}
		}
	}

	// Send the request.
	req.Body, _ = req.GetBody()
	res, err := c.httpClient(ctx).Do(req.WithContext(ctx))
	if err == nil {
		defer func() {
			// Drain the body before closing it to enable HTTP connection reuse. This
			// is all best effort cleanup, don't check errors.
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
		}()
	}
	if c.testPostHTTP != nil {
		err = c.testPostHTTP(ctx, err)
	}
	if err != nil {
		return "", status.Errorf(codeForErr(err), "prpc: sending request: %s", err)
	}

	if options.resHeaderMetadata != nil {
		md, err := headersIntoMetadata(res.Header)
		if err != nil {
			return "", status.Errorf(codes.Internal, "prpc: decoding headers: %s", err)
		}
		*options.resHeaderMetadata = md
	}
	if err := c.readResponseBody(ctx, buf, res); err != nil {
		return "", err
	}
	if options.resTrailerMetadata != nil {
		md, err := headersIntoMetadata(res.Trailer)
		if err != nil {
			return "", status.Errorf(codes.Internal, "prpc: decoding trailers: %s", err)
		}
		*options.resTrailerMetadata = md
	}

	// Read the RPC status (perhaps with details). This is nil on success. Note
	// that errors always have "text/plain" response Content-Type, so we can't
	// rely on this header to know how to deserialize status details. Instead
	// expect details to be encoded based on the "Accept" header in the request
	// (which matches the format of outCodec).
	err = c.readStatus(res, buf, options.respCodec)

	return res.Header.Get("Content-Type"), err
}

// httpClient returns an http.Client to use for a single RPC call attempt.
func (c *Client) httpClient(ctx context.Context) *http.Client {
	if c.C != nil {
		return c.C
	}
	if f := ctx.Value(&clientFactoryCtxKey); f != nil {
		return f.(func(context.Context) *http.Client)(ctx)
	}
	return http.DefaultClient
}

// maxResponseSize is a maximum length of the uncompressed response to read.
//
// Use int64 (not int) in case the client is 32-bit. We need to be able to
// handle large responses in that case (even if just to reject them).
func (c *Client) maxResponseSize() int64 {
	if c.MaxResponseSize <= 0 {
		return DefaultMaxResponseSize
	}
	return int64(c.MaxResponseSize)
}

// readResponseBody copies the response body into dest.
//
// Returns gRPC errors. If the response body size exceeds the limits or the
// declared size, returns UNAVAILABLE grpc error with ResponseTooBig error
// in the details.
func (c *Client) readResponseBody(ctx context.Context, dest *bytes.Buffer, r *http.Response) error {
	limit := c.maxResponseSize()

	dest.Reset()
	if l := r.ContentLength; l > 0 {
		// Note here `limit` is guaranteed to be <= math.MaxInt (even on a 32-bit
		// architecture), since maxResponseSize() derives it from an int-typed
		// field. Thus `l` will also be <= math.MaxInt and the typecast below is
		// sound.
		if l > limit {
			logging.Errorf(ctx, "ContentLength header exceeds response body limit: %d > %d.", l, limit)
			return errResponseTooBig(l, limit)
		}
		limit = l
		dest.Grow(int(limit))
	}

	limitedBody := io.LimitReader(r.Body, limit)
	if _, err := dest.ReadFrom(limitedBody); err != nil {
		return status.Errorf(codeForErr(err), "prpc: reading response: %s", err)
	}

	// If there is more data in the body Reader, it means that the response
	// size has exceeded our limit.
	var probeB [1]byte
	if n, err := r.Body.Read(probeB[:]); n > 0 || err != io.EOF {
		logging.Errorf(ctx, "Response body limit %d exceeded.", limit)
		return errResponseTooBig(0, limit)
	}

	return nil
}

// codeForErr decided a gRPC status code based on an http.Client error.
//
// In particular it recognizes IO timeouts and returns them as DeadlineExceeded
// code. This is necessary since it appears http.Client can sometimes fail with
// an IO timeout error even before the parent context.Context expires (probably
// has something to do with converting the context deadline into a timeout
// duration for the IO calls). When this happens, we still need to return
// DeadlineExceeded error.
func codeForErr(err error) codes.Code {
	if os.IsTimeout(err) {
		return codes.DeadlineExceeded
	}
	return codes.Internal
}

// readStatus retrieves the detailed status from the response.
//
// Puts it into a status.New(...).Err() error.
func (c *Client) readStatus(r *http.Response, bodyBuf *bytes.Buffer, respCodec protoCodec) error {
	codeHeader := r.Header.Get(HeaderGRPCCode)
	if codeHeader == "" {
		// It is possible that the request did not reach the pRPC server and
		// that's why we don't have the code header. This can happen either due to
		// transient errors (HTTP status >= 500) or if some authenticating proxy
		// aborts the request (HTTP statuses 401 and 403, happens on Cloud Run).
		// It's preferable to convert the error to a gRPC status to make the client
		// code treat the response appropriately.
		if r.StatusCode >= 500 || r.StatusCode == http.StatusUnauthorized || r.StatusCode == http.StatusForbidden {
			code := codes.Internal
			switch r.StatusCode {
			case http.StatusUnauthorized:
				code = codes.Unauthenticated
			case http.StatusForbidden:
				code = codes.PermissionDenied
			case http.StatusServiceUnavailable:
				code = codes.Unavailable
			}
			return status.New(code, recognizeHTMLErr(c.readErrorMessage(bodyBuf))).Err()
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
	if details := r.Header[HeaderStatusDetail]; len(details) != 0 {
		if sp.Details, err = parseStatusDetails(details, respCodec); err != nil {
			return err
		}
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
		limit = 4096
	}
	if len(ret) > limit {
		return strings.ToValidUTF8(string(ret[:limit]), "") + "..."
	}

	return string(ret)
}

// recognizeHTMLErr strips HTML framing from an error message.
//
// Google frontends like to reply with an HTML errors which look very odd inside
// pRPC statuses. This function recognizes some known kinds of HTML responses
// and extracts an error message from them (best effort).
func recognizeHTMLErr(htmlText string) string {
	doc, err := html.Parse(strings.NewReader(htmlText))
	if err != nil {
		return htmlText
	}

	// Find <body>...</body> element.
	var findBody func(*html.Node) *html.Node
	findBody = func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode && n.Data == "body" {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if found := findBody(c); found != nil {
				return found
			}
		}
		return nil
	}
	body := findBody(doc)
	if body == nil {
		return htmlText
	}

	var chunks []string

	// Collect all text within the body ignoring any HTML markup.
	var collectText func(*html.Node)
	collectText = func(n *html.Node) {
		if n.Type == html.TextNode {
			for _, line := range strings.Split(n.Data, "\n") {
				if line = strings.TrimSpace(line); line != "" {
					chunks = append(chunks, line)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collectText(c)
		}
	}
	collectText(body)

	if len(chunks) == 0 {
		return htmlText
	}
	return strings.Join(chunks, " ")
}

// parseStatusDetails parses headers with google.rpc.Status.details.
//
// Returns gRPC errors.
func parseStatusDetails(values []string, respCodec protoCodec) ([]*anypb.Any, error) {
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
		if err := respCodec.Decode(buf[:n], msg); err != nil {
			return nil, status.Errorf(codes.Internal, "prpc: failed to unmarshal the status details header: %s", err)
		}
		ret[i] = msg
	}

	return ret, nil
}

// prepareRequest creates an HTTP request for an RPC.
//
// Initializes GetBody, so that the request can be resent multiple times when
// retrying.
func (c *Client) prepareRequest(options *Options, md metadata.MD, requestMessage []byte) (*http.Request, error) {
	// Convert metadata into HTTP headers in canonical form (i.e. Title-Case).
	// Extract Host header, it is special and must be passed via
	// http.Request.Host. Preallocate 6 more slots (for 5 headers below and for
	// the RPC deadline header).
	headers := make(http.Header, len(md)+6)
	if err := metaIntoHeaders(md, headers); err != nil {
		return nil, status.Errorf(codes.Internal, "prpc: headers: %s", err)
	}
	hostHdr := headers.Get("Host")
	headers.Del("Host")

	// Add protocol-related headers.
	headers.Set("Content-Type", options.reqCodec.Format().MediaType())
	headers.Set("Accept", options.respCodec.Format().MediaType())
	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", options.UserAgent)
	}

	// This tells the server to give up sending very large responses. If the limit
	// is disabled (by setting it to math.MaxInt), just don't set the header
	// (math.MaxInt64 looks very scary as a text header). There's an edge case
	// here: a 32-bit client won't be able to process more than 2GB of data either
	// way (it will just fail to allocate a buffer for it), so omit the header
	// only when the client is 64-bit and still send math.MaxInt32 value. It will
	// tell the 64-bit server that responses larger than 2GB won't work.
	if maxRespSize := c.maxResponseSize(); maxRespSize != math.MaxInt64 {
		headers.Set(HeaderMaxResponseSize, strconv.FormatInt(maxRespSize, 10))
	}

	body := requestMessage
	if c.EnableRequestCompression && len(requestMessage) > gzipThreshold {
		headers.Set("Content-Encoding", "gzip")
		var err error
		if body, err = compressBlob(requestMessage); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// Do not add "Accept-Encoding: gzip". The http package does this
		// automatically, and also decompresses the response.
	}

	headers.Set("Content-Length", strconv.Itoa(len(body)))

	scheme := "https"
	if options.Insecure {
		scheme = "http"
	}

	pathPrefix := c.PathPrefix
	if c.PathPrefix == "" {
		pathPrefix = "/prpc"
	}
	return &http.Request{
		Method: "POST",
		URL: &url.URL{
			Scheme: scheme,
			Host:   options.host,
			Path:   fmt.Sprintf("%s/%s/%s", pathPrefix, options.serviceName, options.methodName),
		},
		Host:          hostHdr,
		Header:        headers,
		ContentLength: int64(len(body)),
		GetBody: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		},
	}, nil
}

// SetDefaultHTTPClient allows to modify what http.Client is used by pRPC
// clients by default (i.e. when they have nil as Client.C).
//
// This kicks in only if the returned context (or its derivative) is used as
// an RPC context. This is primarily used in a server environment where the
// server installs a default HTTP client that has monitoring and tracing
// instrumentation.
func SetDefaultHTTPClient(ctx context.Context, factory func(context.Context) *http.Client) context.Context {
	return context.WithValue(ctx, &clientFactoryCtxKey, factory)
}
