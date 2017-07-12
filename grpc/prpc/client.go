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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/grpc/grpcutil"
)

const (
	// HeaderGRPCCode is a name of the HTTP header that specifies the
	// gRPC code in the response.
	// A pRPC server must always specify it.
	HeaderGRPCCode = "X-Prpc-Grpc-Code"

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
	reqBody, err := proto.Marshal(in)
	if err != nil {
		return err
	}

	resp, err := c.CallRaw(ctx, serviceName, methodName, reqBody, FormatBinary, FormatBinary, opts...)
	if err != nil {
		return err
	}
	return proto.Unmarshal(resp, out)
}

// CallRaw makes an RPC, sending and returning the raw data without
// unmarshalling it.
// Retries on transient errors according to retry options.
// Logs HTTP errors.
// Trims JSONPBPrefix.
//
// opts must be created by this package.
// Calling from multiple goroutines concurrently is safe, unless Client is mutated.
// Called from generated code.
//
// If there is a Deadline applied to the Context, it will be forwarded to the
// server using the HeaderTimeout header.
func (c *Client) CallRaw(ctx context.Context, serviceName, methodName string, in []byte, inf, outf Format,
	opts ...grpc.CallOption) ([]byte, error) {
	options, err := c.renderOptions(opts)
	if err != nil {
		return nil, err
	}

	req := prepareRequest(c.Host, serviceName, methodName, len(in), inf, outf, options)
	ctx = logging.SetFields(ctx, logging.Fields{
		"host":    c.Host,
		"service": serviceName,
		"method":  methodName,
	})

	// Send the request in a retry loop.
	var buf bytes.Buffer
	var contentType string
	err = retry.Retry(
		ctx,
		transient.Only(options.Retry),
		func() error {
			ctx := ctx
			logging.Debugf(ctx, "RPC %s/%s.%s", c.Host, serviceName, methodName)

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
				if delta <= 0 {
					// The request has already expired. This will likely never happen,
					// since the outer Retry loop will have expired, but there is a very
					// slight possibility of a race.
					return context.DeadlineExceeded
				}

				req.Header.Set(HeaderTimeout, EncodeTimeout(delta))
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
			var body io.Reader = res.Body

			limit := c.MaxContentLength
			if limit <= 0 {
				limit = DefaultMaxContentLength
			}
			if l := res.ContentLength; l > 0 {
				if l > int64(limit) {
					logging.Fields{
						"contentLength": l,
						"limit":         limit,
					}.Errorf(ctx, "ContentLength header exceeds soft response body limit.")
					return ErrResponseTooBig
				}
				limit = int(l)
				buf.Grow(limit)
			}
			body = io.LimitReader(body, int64(limit))
			if _, err = buf.ReadFrom(body); err != nil {
				return fmt.Errorf("failed to read response body: %s", err)
			}

			// If there is more data in the body Reader, it means that the response
			// size has exceeded our limit.
			var probeB [1]byte
			if amt, err := body.Read(probeB[:]); amt > 0 || err != io.EOF {
				logging.Fields{
					"limit": limit,
				}.Errorf(ctx, "Soft response body limit exceeded.")
				return ErrResponseTooBig
			}

			if options.resTrailerMetadata != nil {
				*options.resTrailerMetadata = metadataFromHeaders(res.Trailer)
			}

			codeHeader := res.Header.Get(HeaderGRPCCode)
			if codeHeader == "" {
				// Not a valid pRPC response.
				body := buf.String()
				bodySize := c.ErrBodySize
				if bodySize <= 0 {
					bodySize = 256
				}
				if len(body) > bodySize {
					body = body[:bodySize] + "..."
				}
				err := fmt.Errorf("HTTP %d: no gRPC code. Body: %q", res.StatusCode, body)

				// Some HTTP codes are returned directly by hosting platforms (e.g.,
				// AppEngine), and should be automatically retried even if a gRPC code
				// header is not supplied.
				if res.StatusCode >= http.StatusInternalServerError {
					err = transient.Tag.Apply(err)
				}
				return err
			}

			codeInt, err := strconv.Atoi(codeHeader)
			if err != nil {
				// Not a valid pRPC response.
				return fmt.Errorf("invalid grpc code %q: %s", codeHeader, err)
			}

			code := codes.Code(codeInt)
			if code != codes.OK {
				desc := strings.TrimSuffix(buf.String(), "\n")
				err := grpcutil.Errf(code, "%s", desc)
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

	// We have to unwrap gRPC errors because
	// grpc.Code and grpc.ErrorDesc functions do not work with error wrappers.
	// https://github.com/grpc/grpc-go/issues/494
	if err != nil {
		logging.WithError(err).Warningf(ctx, "RPC failed permanently: %s", err)
		return nil, errors.Unwrap(err)
	}

	// Parse the response content type.
	f, err := FormatFromContentType(contentType)
	if err != nil {
		return nil, err
	}
	if f != outf {
		return nil, fmt.Errorf("output format (%s) doesn't match expected format (%s)", f.ContentType(), outf.ContentType())
	}

	out := buf.Bytes()
	if outf == FormatJSONPB {
		out = bytes.TrimPrefix(out, bytesJSONPBPrefix)
	}
	return out, nil
}

// prepareRequest creates an HTTP request for an RPC,
// except it does not set the request body.
func prepareRequest(host, serviceName, methodName string, contentLength int, inf, outf Format, options *Options) *http.Request {
	if host == "" {
		panic("Host is not set")
	}
	req := &http.Request{
		Method: "POST",
		URL: &url.URL{
			Scheme: "https",
			Host:   host,
			Path:   fmt.Sprintf("/prpc/%s/%s", serviceName, methodName),
		},
		Header: http.Header{},
	}
	if options.Insecure {
		req.URL.Scheme = "http"
	}

	// Set headers.
	req.Header.Set("Content-Type", inf.ContentType())
	req.Header.Set("Accept", outf.ContentType())
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
