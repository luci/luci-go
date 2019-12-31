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
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	spb "google.golang.org/genproto/googleapis/rpc/status"

	"golang.org/x/net/context/ctxhttp"
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
	// the Client's soft limit, MaxContentLength.
	ErrResponseTooBig = errors.New("response too big")
)

// Client can make pRPC calls.
type Client struct {
	C *http.Client // if nil, uses http.DefaultClient

	// ErrBodySize is the number of bytes to read from a HTTP response
	// with error status and include in the error.
	// If non-positive, defaults to 256.
	ErrBodySize int

	// MaxContentLength, if > 0, is the maximum content length, in bytes, that a
	// pRPC is willing to read from the server. If a larger content length is
	// present in the response, ErrResponseTooBig will be returned.
	//
	// If <= 0, DefaultMaxContentLength will be used.
	MaxContentLength int

	Host    string   // host and optionally a port number of the target server.
	Options *Options // if nil, DefaultOptions() are used.

	// testPostHTTP is a test-installed callback that is invoked after an HTTP
	// request finishes.
	testPostHTTP func(context.Context, error) error
}

// renderOptions copies client options and applies opts.
func (c *Client) renderOptions(opts []grpc.CallOption) (*Options, error) {
	var options *Options
	if c.Options != nil {
		cpy := *c.Options
		options = &cpy
	} else {
		options = DefaultOptions()
	}
	if err := options.apply(opts); err != nil {
		return nil, err
	}
	return options, nil
}

func (c *Client) getHTTPClient() *http.Client {
	if c.C == nil {
		return http.DefaultClient
	}
	return c.C
}

// Call makes an RPC.
// Retries on transient errors according to retry options.
// Logs HTTP errors.
//
// opts must be created by this package.
// Calling from multiple goroutines concurrently is safe, unless Client is mutated.
// Called from generated code.
//
// If there is a Deadline applied to the Context, it will be forwarded to the
// server using the HeaderTimeout header.
func (c *Client) Call(ctx context.Context, serviceName, methodName string, in, out proto.Message, opts ...grpc.CallOption) error {
	options, err := c.renderOptions(opts)
	if err != nil {
		return err
	}

	// Due to https://github.com/golang/protobuf/issues/745 bug
	// in jsonpb handling of FieldMask, which are typically present in the
	// request, not the response, do request via binary format.
	inFormat := FormatBinary
	reqBody, err := proto.Marshal(in)
	if err != nil {
		return err
	}

	var outFormat Format
	switch options.AcceptContentSubtype {
	case "", mtPRPCEncodingBinary:
		outFormat = FormatBinary
	case mtPRPCEncodingJSONPB:
		outFormat = FormatJSONPB
	case mtPRPCEncodingText:
		return errors.New("text encoding for pRPC calls is not implemented")
	default:
		return fmt.Errorf("unrecognized contentSubtype %q of CallAcceptContentSubtype", options.AcceptContentSubtype)
	}

	resp, err := c.call(ctx, serviceName, methodName, reqBody, inFormat, outFormat, options)
	if err != nil {
		return err
	}

	switch outFormat {
	case FormatBinary:
		return proto.Unmarshal(resp, out)
	case FormatJSONPB:
		return jsonpb.UnmarshalString(string(resp), out)
	default:
		return errors.New("unreachable")
	}
}

// CallWithFormats makes an RPC, sending and returning the raw data without
// unmarshalling it.
// Retries on transient errors according to retry options.
// Logs HTTP errors.
// Trims JSONPBPrefix.
//
// opts must be created by this package.
// Calling from multiple goroutines concurrently is safe, unless Client is mutated.
//
// If there is a Deadline applied to the Context, it will be forwarded to the
// server using the HeaderTimeout header.
func (c *Client) CallWithFormats(ctx context.Context, serviceName, methodName string, in []byte, inf, outf Format,
	opts ...grpc.CallOption) ([]byte, error) {
	options, err := c.renderOptions(opts)
	if err != nil {
		return nil, err
	}
	if options.AcceptContentSubtype != "" {
		return nil, errors.New("prpc.CallAcceptContentSubtype call option not allowed with `CallWithFormats`" +
			" because input/output formats are already specified")
	}
	return c.call(ctx, serviceName, methodName, in, inf, outf, options)
}

func (c *Client) call(ctx context.Context, serviceName, methodName string, in []byte, inf, outf Format,
	options *Options) ([]byte, error) {

	md, _ := metadata.FromOutgoingContext(ctx)
	req := prepareRequest(c.Host, serviceName, methodName, md, len(in), inf, outf, options)
	ctx = logging.SetFields(ctx, logging.Fields{
		"host":    c.Host,
		"service": serviceName,
		"method":  methodName,
	})

	// Send the request in a retry loop.
	buf := &bytes.Buffer{}
	var contentType string
	err := retry.Retry(
		ctx,
		transient.Only(options.Retry),
		func() error {
			ctx := ctx

			// If there is a deadline on our Context, set the timeout header on the
			// request.
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
				// We have a shorter request deadline. Create a derivative Context for
				// this request round.
				var cancelFunc context.CancelFunc
				ctx, cancelFunc = clock.WithDeadline(ctx, requestDeadline)
				defer cancelFunc()
			}

			// If we have a request deadline, apply it to our header.
			if !requestDeadline.IsZero() {
				delta := requestDeadline.Sub(now)
				logging.Debugf(ctx, "RPC %s/%s.%s [deadline %s]", c.Host, serviceName, methodName, delta)
				if delta <= 0 {
					// The request has already expired. This will likely never happen,
					// since the outer Retry loop will have expired, but there is a very
					// slight possibility of a race.
					return context.DeadlineExceeded
				}
				req.Header.Set(HeaderTimeout, EncodeTimeout(delta))
			} else {
				logging.Debugf(ctx, "RPC %s/%s.%s", c.Host, serviceName, methodName)
			}

			// Send the request.
			req.Body = ioutil.NopCloser(bytes.NewReader(in))
			res, err := ctxhttp.Do(ctx, c.getHTTPClient(), req)
			if res != nil && res.Body != nil {
				defer res.Body.Close()
			}
			if c.testPostHTTP != nil {
				err = c.testPostHTTP(ctx, err)
			}
			if err != nil {
				// Treat all errors here as transient.
				return errors.Annotate(err, "failed to send request").Tag(transient.Tag).Err()
			}

			if options.resHeaderMetadata != nil {
				*options.resHeaderMetadata = metadataFromHeaders(res.Header)
			}
			contentType = res.Header.Get("Content-Type")

			// Read the response body.
			buf.Reset()
			if err := c.readResponseBody(ctx, buf, res); err != nil {
				return err
			}

			if options.resTrailerMetadata != nil {
				*options.resTrailerMetadata = metadataFromHeaders(res.Trailer)
			}

			code, err := c.readStatusCode(res, buf)
			if err != nil {
				return err
			}
			if code != codes.OK {
				sp := &spb.Status{
					Code:    int32(code),
					Message: strings.TrimSuffix(c.readErrorMessage(buf), "\n"),
				}
				var detErr error
				sp.Details, detErr = c.readStatusDetails(res)
				if detErr != nil {
					return detErr
				}
				err = status.FromProto(sp).Err()
				if grpcutil.IsTransientCode(code) {
					err = transient.Tag.Apply(err)
				}
				return err
			}
			return nil
		},
		func(err error, sleepTime time.Duration) {
			logging.Fields{
				logging.ErrorKey: err,
				"sleepTime":      sleepTime,
			}.Warningf(ctx, "RPC failed transiently. Will retry in %s", sleepTime)
		},
	)

	// We have to unwrap gRPC errors because grpc.Code and grpc.ErrorDesc
	// functions do not work with error wrappers.
	// https://github.com/grpc/grpc-go/issues/494
	if err != nil {
		innerErr := errors.Unwrap(err)
		code := grpc.Code(innerErr)

		// Log only unexpected codes.
		ignore := innerErr == context.Canceled
		for _, expected := range options.expectedCodes {
			if code == expected {
				ignore = true
				break
			}
		}
		if !ignore {
			logging.WithError(err).Warningf(ctx, "RPC failed permanently: %s", err)
		}

		return nil, innerErr
	}

	// Parse the response content type.
	f, err := FormatFromContentType(contentType)
	if err != nil {
		return nil, err
	}
	if f != outf {
		return nil, fmt.Errorf("output format (%s) doesn't match expected format (%s)", f.MediaType(), outf.MediaType())
	}

	out := buf.Bytes()
	if outf == FormatJSONPB {
		out = bytes.TrimPrefix(out, bytesJSONPBPrefix)
	}
	return out, nil
}

// readResponseBody reads the response body to dest.
// If the response body size exceeds the limits or the declared size, returns
// a non-nil error.
func (c *Client) readResponseBody(ctx context.Context, dest *bytes.Buffer, r *http.Response) error {
	limit := c.MaxContentLength
	if limit <= 0 {
		limit = DefaultMaxContentLength
	}

	if l := r.ContentLength; l > 0 {
		if l > int64(limit) {
			logging.Errorf(ctx, "ContentLength header exceeds soft response body limit: %d > %d.", l, limit)
			return ErrResponseTooBig
		}
		limit = int(l)
		dest.Grow(limit)
	}

	limitedBody := io.LimitReader(r.Body, int64(limit))
	if _, err := dest.ReadFrom(limitedBody); err != nil {
		return fmt.Errorf("failed to read response body: %s", err)
	}

	// If there is more data in the body Reader, it means that the response
	// size has exceeded our limit.
	var probeB [1]byte
	if n, err := r.Body.Read(probeB[:]); n > 0 || err != io.EOF {
		logging.Errorf(ctx, "Soft response body limit %d exceeded.", limit)
		return ErrResponseTooBig
	}

	return nil
}

// readStatusCode retrieves the status code from the response.
func (c *Client) readStatusCode(r *http.Response, bodyBuf *bytes.Buffer) (codes.Code, error) {
	codeHeader := r.Header.Get(HeaderGRPCCode)
	if codeHeader == "" {
		// Not a valid pRPC response.
		err := fmt.Errorf("HTTP %d: no gRPC code. Body: %q", r.StatusCode, c.readErrorMessage(bodyBuf))

		// Some HTTP codes are returned directly by hosting platforms (e.g.,
		// AppEngine), and should be automatically retried even if a gRPC code
		// header is not supplied.
		if r.StatusCode >= http.StatusInternalServerError {
			err = transient.Tag.Apply(err)
		}
		return 0, err
	}

	codeInt, err := strconv.Atoi(codeHeader)
	if err != nil {
		// Not a valid pRPC response.
		return 0, fmt.Errorf("invalid grpc code %q: %s", codeHeader, err)
	}

	return codes.Code(codeInt), nil
}

// readErrorMessage reads an error message from a body buffer.
// Respects c.ErrBodySize.
// If the error message is too long, trims it and appends "...".
func (c *Client) readErrorMessage(bodyBuf *bytes.Buffer) string {
	ret := bodyBuf.String()

	// Apply limits.
	limit := c.ErrBodySize
	if limit <= 0 {
		limit = 256
	}
	if len(ret) > limit {
		ret = strings.ToValidUTF8(ret[:limit], "") + "..."
	}

	return ret
}

// readStatusDetails reads google.rpc.Status.details from the response headers.
func (c *Client) readStatusDetails(r *http.Response) ([]*any.Any, error) {
	values := r.Header[HeaderStatusDetail]
	if len(values) == 0 {
		return nil, nil
	}

	b64 := base64.StdEncoding

	ret := make([]*any.Any, len(values))
	var buf []byte
	for i, v := range values {
		sz := b64.DecodedLen(len(v))
		if cap(buf) < sz {
			buf = make([]byte, sz)
		}

		n, err := b64.Decode(buf[:sz], []byte(v))
		if err != nil {
			return nil, fmt.Errorf("invalid header %s: %s", HeaderStatusDetail, v)
		}

		msg := &any.Any{}
		if err := proto.Unmarshal(buf[:n], msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal a status detail: %s", err)
		}
		ret[i] = msg
	}

	return ret, nil
}

// prepareRequest creates an HTTP request for an RPC,
// except it does not set the request body.
func prepareRequest(host, serviceName, methodName string, md metadata.MD, contentLength int, inf, outf Format, options *Options) *http.Request {
	if host == "" {
		panic("Host is not set")
	}

	// Convert headers to HTTP canonical form (i.e. Title-Case). Extract Host
	// header, it is special and must be passed via http.Request.Host.
	headers := make(http.Header, len(md))
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

	req := &http.Request{
		Method: "POST",
		URL: &url.URL{
			Scheme: "https",
			Host:   host,
			Path:   fmt.Sprintf("/prpc/%s/%s", serviceName, methodName),
		},
		Host:   hostHdr,
		Header: headers,
	}
	if options.Insecure {
		req.URL.Scheme = "http"
	}

	// Set headers.
	req.Header.Set("Content-Type", inf.MediaType())
	req.Header.Set("Accept", outf.MediaType())
	userAgent := options.UserAgent
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	req.Header.Set("User-Agent", userAgent)
	req.ContentLength = int64(contentLength)
	req.Header.Set("Content-Length", strconv.Itoa(contentLength))
	// TODO(nodir): add "Accept-Encoding: gzip" when pRPC server supports it.
	return req
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
