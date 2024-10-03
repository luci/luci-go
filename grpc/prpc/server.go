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
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

var (
	// Describes the default permitted headers for cross-origin requests.
	//
	// It explicitly includes a subset of the default CORS-safelisted request
	// headers that are relevant to RPCs, as well as "Authorization" header plus
	// some headers used in the pRPC protocol.
	//
	// Custom AccessControl implementations may allow more headers.
	//
	// See https://developer.mozilla.org/en-US/docs/Glossary/CORS-safelisted_request_header
	allowHeaders = strings.Join([]string{
		"Origin",
		"Content-Type",
		"Accept",
		"Authorization",
		HeaderTimeout,
		HeaderMaxResponseSize,
	}, ", ")

	// What HTTP verbs are allowed for cross-origin requests.
	allowMethods = strings.Join([]string{"OPTIONS", "POST"}, ", ")

	// allowPreflightCacheAgeSecs is the amount of time to enable the browser to
	// cache the preflight access control response, in seconds.
	//
	// 600 seconds is 10 minutes.
	allowPreflightCacheAgeSecs = "600"

	// exposeHeaders lists the non-standard response headers that are exposed to
	// clients that make cross-origin calls.
	exposeHeaders = strings.Join([]string{
		HeaderGRPCCode,
		HeaderStatusDetail,
	}, ", ")
)

// DefaultMaxRequestSize is the default maximum request size (in bytes)
// the server is willing to read from the client.
const DefaultMaxRequestSize = 64 * 1024 * 1024

// ResponseCompression controls how the server compresses responses.
//
// See Server doc for details.
type ResponseCompression string

// Possible values for ResponseCompression.
const (
	CompressDefault ResponseCompression = "" // same as "never"
	CompressNever   ResponseCompression = "never"
	CompressAlways  ResponseCompression = "always"
	CompressNotJSON ResponseCompression = "not JSON"
)

// AccessControlDecision describes how to handle a cross-origin request.
//
// It is returned by AccessControl server callback based on the origin of the
// call and defines what CORS policy headers to add to responses to preflight
// requests and actual cross-origin requests.
//
// See https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS for the general
// overview of CORS policies.
type AccessControlDecision struct {
	// AllowCrossOriginRequests defines if CORS policies should be enabled at all.
	//
	// If false, the server will not send any CORS policy headers and the browser
	// will use the default "same-origin" policy. All fields below will be
	// ignored.
	//
	// See https://developer.mozilla.org/en-US/docs/Web/Security/Same-origin_policy
	AllowCrossOriginRequests bool

	// AllowCredentials defines if a cross-origin request is allowed to use
	// credentials associated with the pRPC server's domain.
	//
	// This setting controls if the client can use `credentials: include` option
	// in Fetch init parameters or `withCredentials = true` in XMLHttpRequest.
	//
	// Credentials, as defined by Web APIs, are cookies, HTTP authentication
	// entries, and TLS client certificates.
	//
	// See
	//   https://developer.mozilla.org/en-US/docs/Web/API/fetch#parameters
	//   https://developer.mozilla.org/en-US/docs/Web/API/XMLHttpRequest/withCredentials
	//   https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Access-Control-Allow-Credentials
	AllowCredentials bool

	// AllowHeaders is a list of request headers that a cross-origin request is
	// allowed to have.
	//
	// Extends the default list of allowed headers.
	//
	// Use this option if the authentication interceptor checks some custom
	// headers.
	AllowHeaders []string
}

// AllowOriginAll is a CORS policy that allows any origin to send cross-origin
// requests that can use credentials (e.g. cookies) and "Authorization" header.
//
// Other non-standard headers are forbidden. Use a custom AccessControl callback
// to allow them.
func AllowOriginAll(ctx context.Context, origin string) AccessControlDecision {
	return AccessControlDecision{
		AllowCrossOriginRequests: true,
		AllowCredentials:         true,
	}
}

// Override is a pRPC method-specific override which may optionally handle the
// entire pRPC method call. If it returns true, the override is assumed to
// have fully handled the pRPC method call and processing of the request does
// not continue. In this case it's the override's responsibility to adhere to
// all pRPC semantics. However if it returns false, processing continues as
// normal, allowing the override to act as a preprocessor. In this case it's
// the override's responsibility to ensure it hasn't done anything that will
// be incompatible with pRPC semantics (such as writing garbage to the response
// writer in the router context).
//
// Override receives `body` callback as an argument that can, on demand, read
// and decode the request body using the decoder constructed based on the
// request headers. This callback can be used to "peek" inside the deserialized
// request to decide if the override should be activated. Note that `req.Body`
// can be read only once, and the callback reads it. To allow reusing `req`
// after the callback is done, it, as a side effect, replaces `req.Body` with a
// byte buffer containing the data it just read. The callback can be called at
// most once.
type Override func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (stop bool, err error)

// Server is a pRPC server to serve RPC requests.
// Zero value is valid.
type Server struct {
	// AccessControl, if not nil, is a callback that is invoked per request to
	// determine what CORS access control headers, if any, should be added to
	// the response.
	//
	// It controls what cross-origin RPC calls are allowed from a browser and what
	// responses cross-origin clients may see. Does not effect calls made by
	// non-browser RPC clients.
	//
	// This callback is invoked for each CORS pre-flight request as well as for
	// actual RPC requests. It can make its decision based on the origin of
	// the request.
	//
	// If nil, the server will not send any CORS headers and the browser will
	// use the default "same-origin" policy.
	//
	// See https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS for the general
	// overview of CORS policies.
	AccessControl func(ctx context.Context, origin string) AccessControlDecision

	// HackFixFieldMasksForJSON indicates whether to attempt a workaround for
	// https://github.com/golang/protobuf/issues/745 when the request has
	// Content-Type: application/json. This hack is scheduled for removal.
	// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
	HackFixFieldMasksForJSON bool

	// UnaryServerInterceptor provides a hook to intercept the execution of
	// a unary RPC on the server. It is the responsibility of the interceptor to
	// invoke handler to complete the RPC.
	UnaryServerInterceptor grpc.UnaryServerInterceptor

	// ResponseCompression controls how the server compresses responses.
	//
	// It applies only to eligible responses. A response is eligible for
	// compression if the client sends "Accept-Encoding: gzip" request header
	// (default for all Go clients, including the standard pRPC client) and
	// the response size is larger than a certain threshold.
	//
	// If ResponseCompression is CompressNever or CompressDefault, responses are
	// never compressed. This is a conservative default (since the server
	// generally doesn't know the trade off between CPU and network in the
	// environment it runs in and assumes network is cheaper; in particular this
	// situation is realized when the server is running on the localhost network).
	//
	// If ResponseCompression is CompressAlways, eligible responses are always
	// compressed.
	//
	// If ResponseCompression is CompressNotJSON, only eligible responses that
	// do **not** use JSON encoding will be compressed. JSON responses are left
	// uncompressed at this layer. This is primarily added for the Appengine
	// environment: GAE runtime compresses JSON responses on its own already,
	// presumably using a more efficient compressor (C++ and all). Note that the
	// client will see all eligible responses compressed (JSON ones will be
	// compressed by the GAE, and non-JSON ones will be compressed by the pRPC
	// server).
	//
	// A compressed response is accompanied by "Content-Encoding: gzip" response
	// header, telling the client that it should decompress it.
	//
	// The request compression is configured independently on the client. The
	// server always accepts compressed requests.
	ResponseCompression ResponseCompression

	// MaxRequestSize is how many bytes the server will read from the request body
	// before rejecting it as too big.
	//
	// This is a protection against OOMing the server by big requests.
	//
	// When using request compression the limit is checked twice: when reading
	// the original compressed request, and when decompressing it. Checking the
	// decompressed request size is done to to prevent attacks like "zip bombs"
	// where very small payloads decompress to enormous buffer sizes.
	//
	// Note that some environments impose their own request size limits. In
	// particular, AppEngine and Cloud Run (but only when using HTTP/1), have
	// a 32MiB request size limit.
	//
	// If <= 0, DefaultMaxRequestSize will be used.
	MaxRequestSize int

	mu        sync.RWMutex
	services  map[string]*service
	overrides map[string]map[string]Override
}

type service struct {
	methods map[string]grpc.MethodDesc
	impl    any
}

// RegisterService registers a service implementation.
// Called from the generated code.
//
// desc must contain description of the service, its message types
// and all transitive dependencies.
//
// Panics if a service of the same name is already registered.
func (s *Server) RegisterService(desc *grpc.ServiceDesc, impl any) {
	serv := &service{
		impl:    impl,
		methods: make(map[string]grpc.MethodDesc, len(desc.Methods)),
	}
	for _, m := range desc.Methods {
		serv.methods[m.MethodName] = m
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.services == nil {
		s.services = map[string]*service{}
	} else if _, ok := s.services[desc.ServiceName]; ok {
		panic(fmt.Errorf("service %q is already registered", desc.ServiceName))
	}

	s.services[desc.ServiceName] = serv
}

// RegisterOverride registers an overriding function.
//
// Panics if an override for the given service method is already registered.
func (s *Server) RegisterOverride(serviceName, methodName string, fn Override) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.overrides == nil {
		s.overrides = map[string]map[string]Override{}
	}
	if _, ok := s.overrides[serviceName]; !ok {
		s.overrides[serviceName] = map[string]Override{}
	}
	if _, ok := s.overrides[serviceName][methodName]; ok {
		panic(fmt.Errorf("method %q of service %q is already overridden", methodName, serviceName))
	}

	s.overrides[serviceName][methodName] = fn
}

// InstallHandlers installs HTTP handlers at /prpc/:service/:method.
//
// See https://godoc.org/go.chromium.org/luci/grpc/prpc#hdr-Protocol
// for pRPC protocol.
//
// Assumes incoming requests are not authenticated yet. Authentication should be
// performed by an interceptor (just like when using a gRPC server).
func (s *Server) InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	r.POST("/prpc/:service/:method", base, s.handlePOST)
	r.OPTIONS("/prpc/:service/:method", base, s.handleOPTIONS)
}

// maxRequestSize is a limit on the request size pre and post decompression.
func (s *Server) maxRequestSize() int {
	if s.MaxRequestSize > 0 {
		return s.MaxRequestSize
	}
	return DefaultMaxRequestSize
}

// maxBytesReader wraps `r` with a reader that reads up to a maximum allowed
// request size.
//
// When the limit is reached, it marks the request connection as "faulty" to
// close it as soon as the response is written. Note that `w` must be an actual
// http.Server's ResponseWriter for this to work (http.MaxBytesReader uses
// special private API inside).
func (s *Server) maxBytesReader(w http.ResponseWriter, r io.ReadCloser) io.ReadCloser {
	// If `r` is a *reproducingReader, then we have applied the limit already.
	// Applying it twice may do weird things to `w`.
	if _, replaced := r.(*reproducingReader); replaced {
		return r
	}
	return http.MaxBytesReader(w, r, int64(s.maxRequestSize()))
}

// handlePOST handles POST requests with pRPC messages.
//
// See https://godoc.org/go.chromium.org/luci/grpc/prpc#hdr-Protocol
// for pRPC protocol.
func (s *Server) handlePOST(c *router.Context) {
	serviceName := c.Params.ByName("service")
	methodName := c.Params.ByName("method")

	override, service, method, methodFound := s.lookup(serviceName, methodName)

	// Override takes precedence over notImplementedErr.
	if override != nil {
		var originalReader io.ReadCloser

		decode := func(msg proto.Message) error {
			if originalReader != nil {
				panic("the body callback should not be called more than once")
			}
			// Read as many bytes as we can (all of them if there's no IO errors).
			body, err := io.ReadAll(s.maxBytesReader(c.Writer, c.Request.Body))
			// Replace c.Request.Body with a reader that reproduces whatever we just
			// read, including the final error (if any). That way this IO error will
			// correctly be handled later (regardless if the override happens or not).
			originalReader = c.Request.Body
			c.Request.Body = &reproducingReader{r: bytes.NewReader(body), eofErr: err}
			if err != nil {
				return err
			}
			// Try to decode the request body as an RPC message.
			return readMessage(
				bytes.NewReader(body),
				c.Request.Header,
				msg,
				s.maxRequestSize(),
				s.HackFixFieldMasksForJSON,
			)
		}

		// This will potentially call `decode` inside.
		stop, err := override(c.Writer, c.Request, decode)

		// It appears http.Server holds a reference to the original req.Body and
		// closes it properly when the request ends (even if we replace req.Body).
		// But this is not documented. To be safe, restore req.Body to the original
		// value before exiting.
		if originalReader != nil {
			defer func() { c.Request.Body = originalReader }()
		}

		switch {
		case err != nil:
			s.writeInitialHeaders(c.Request, c.Writer.Header())
			writeError(
				c.Request.Context(),
				c.Writer,
				requestReadErr(err, "the override check"),
				FormatText,
			)
			return
		case stop:
			return
		}
	}

	res := response{}
	switch {
	case service == nil:
		res.err = status.Errorf(
			codes.Unimplemented,
			"service %q is not implemented",
			serviceName)
	case !methodFound:
		res.err = status.Errorf(
			codes.Unimplemented,
			"method %q in service %q is not implemented",
			methodName, serviceName)
	default:
		// Note: this at most populates some of c.Writer.Header(). The actual RPC
		// response or error are placed into `res` and flushed below.
		s.parseAndCall(c.Request, c.Writer, service, method, &res)
	}

	// Ignore client's gzip preference if the server doesn't want to do
	// compression for this particular request.
	if res.acceptsGZip {
		switch s.ResponseCompression {
		case CompressAlways:
			res.acceptsGZip = true
		case CompressNotJSON:
			res.acceptsGZip = res.fmt != FormatJSONPB
		default:
			res.acceptsGZip = false
		}
	}

	s.writeInitialHeaders(c.Request, c.Writer.Header())
	writeResponse(c.Request.Context(), c.Writer, &res)
}

func (s *Server) handleOPTIONS(c *router.Context) {
	s.writeAccessControlHeaders(c.Request, c.Writer.Header(), true)
	c.Writer.WriteHeader(http.StatusOK)
}

// reproducingReader returns all of `r` but then fails with `eofErr` instead of
// io.EOF.
//
// This makes it reproduce a potentially faulty io.Reader from the original
// request.
type reproducingReader struct {
	r      *bytes.Reader
	eofErr error
}

func (r *reproducingReader) Read(p []byte) (n int, err error) {
	n, err = r.r.Read(p)
	if err == io.EOF {
		if r.eofErr != nil {
			err = r.eofErr
		}
	}
	return
}

func (r *reproducingReader) Close() error {
	return nil
}

var requestContextKey = "context key with *requestContext"

type requestContext struct {
	// Additional headers that will be sent in the response.
	header http.Header
}

// SetHeader sets the header metadata.
//
// When called multiple times, all the provided metadata will be merged.
// Headers set this way will be sent whenever the RPC handler finishes
// (successfully or not).
//
// Some headers are reserved for internal pRPC or HTTP needs and can't be
// set this way. This includes all "x-prpc-*" headers, various core HTTP
// headers (like "content-type") and all "access-control-*" headers (if you want
// to configure CORS policies, use [Server.AccessControl] field instead).
//
// If ctx is not a pRPC server context, then SetHeader calls grpc.SetHeader
// such that calling prpc.SetHeader works for both pRPC and gRPC.
func SetHeader(ctx context.Context, md metadata.MD) error {
	if rctx, ok := ctx.Value(&requestContextKey).(*requestContext); ok {
		if err := metaIntoHeaders(md, rctx.header); err != nil {
			return err
		}
		return nil
	}
	return grpc.SetHeader(ctx, md)
}

type response struct {
	out             proto.Message
	fmt             Format
	acceptsGZip     bool
	maxResponseSize int64 // 0 => no limit
	err             error
}

func (s *Server) lookup(serviceName, methodName string) (override Override, service *service, method grpc.MethodDesc, methodFound bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if methods, ok := s.overrides[serviceName]; ok {
		override = methods[methodName]
	}
	service = s.services[serviceName]
	if service == nil {
		return
	}
	method, methodFound = service.methods[methodName]
	return
}

// parseAndCall parses the request and calls the RPC implementation.
//
// All errors (either protocol-level or RPC errors) are placed into `r`.
//
// `rw` is used to pass it to http.MaxBytesReader(...) and to write headers to
// via SetHeader(...).
func (s *Server) parseAndCall(req *http.Request, rw http.ResponseWriter, service *service, method grpc.MethodDesc, r *response) {
	var perr *protocolError
	r.fmt, perr = responseFormat(req.Header.Get(headerAccept))
	if perr != nil {
		r.err = perr
		return
	}

	var err error
	if r.acceptsGZip, err = acceptsGZipResponse(req.Header); err != nil {
		r.err = protocolErr(codes.InvalidArgument, http.StatusBadRequest, "bad Accept header: %s", err)
		return
	}

	if val := req.Header.Get(HeaderMaxResponseSize); val != "" {
		r.maxResponseSize, err = strconv.ParseInt(val, 10, 64)
		if err == nil && r.maxResponseSize <= 0 {
			err = fmt.Errorf("must be positive")
		}
		if err != nil {
			r.err = protocolErr(codes.InvalidArgument, http.StatusBadRequest, "bad %s value %q: %s", HeaderMaxResponseSize, val, err)
			return
		}
	}

	methodCtx, cancelFunc, err := parseHeader(req.Context(), req.Header, req.Host)
	if err != nil {
		r.err = protocolErr(codes.InvalidArgument, http.StatusBadRequest, "bad request headers: %s", err)
		return
	}
	defer cancelFunc()

	methodCtx = context.WithValue(methodCtx, &requestContextKey, &requestContext{header: rw.Header()})

	// Populate peer.Peer if we can manage to parse the address. This may fail
	// if the server is exposed via a Unix socket, for example.
	if addr, err := netip.ParseAddrPort(req.RemoteAddr); err == nil {
		methodCtx = peer.NewContext(methodCtx, &peer.Peer{
			Addr: net.TCPAddrFromAddrPort(addr),
		})
	}

	out, err := method.Handler(service.impl, methodCtx, func(in any) error {
		if in == nil {
			return status.Errorf(codes.Internal, "input message is nil")
		}
		return readMessage(
			s.maxBytesReader(rw, req.Body),
			req.Header,
			in.(proto.Message),
			s.maxRequestSize(),
			s.HackFixFieldMasksForJSON,
		)
	}, s.UnaryServerInterceptor)

	switch {
	case err != nil:
		r.err = err
	case out == nil:
		r.err = status.Error(codes.Internal, "service returned nil message")
	default:
		r.out = out.(proto.Message)
	}
}

// writeInitialHeaders populates headers present in all pRPC responses
// (successful or not).
func (s *Server) writeInitialHeaders(req *http.Request, headers http.Header) {
	// CORS headers are necessary for successes and errors.
	s.writeAccessControlHeaders(req, headers, false)

	// Never interpret the response as e.g. HTML.
	headers.Set("X-Content-Type-Options", "nosniff")

	// This tells the http.Server not to auto-populate Date header. We do this
	// because Date is not part of the pRPC protocol and its presence in responses
	// is generally unexpected and should not be relied upon. If the header is set
	// already, it means it was emitted via SetHeader(...) in the RPC handler and
	// we should respect that.
	if _, present := headers["Date"]; !present {
		headers["Date"] = nil
	}
}

// writeAccessControlHeaders populates Access-Control-* headers.
func (s *Server) writeAccessControlHeaders(req *http.Request, headers http.Header, preflight bool) {
	// Don't write out access control headers if the origin is unspecified.
	const originHeader = "Origin"
	origin := req.Header.Get(originHeader)
	if origin == "" || s.AccessControl == nil {
		return
	}
	accessControl := s.AccessControl(req.Context(), origin)
	if !accessControl.AllowCrossOriginRequests {
		if accessControl.AllowCredentials {
			logging.Warningf(req.Context(), "pRPC AccessControl: ignoring AllowCredentials since AllowCrossOriginRequests is false")
		}
		if len(accessControl.AllowHeaders) != 0 {
			logging.Warningf(req.Context(), "pRPC AccessControl: ignoring AllowHeaders since AllowCrossOriginRequests is false")
		}
		return
	}

	headers.Add("Access-Control-Allow-Origin", origin)
	headers.Add("Vary", originHeader) // indicate the response depends on Origin header
	if accessControl.AllowCredentials {
		headers.Add("Access-Control-Allow-Credentials", "true")
	}

	if preflight {
		if len(accessControl.AllowHeaders) == 0 {
			headers.Add("Access-Control-Allow-Headers", allowHeaders)
		} else {
			headers.Add("Access-Control-Allow-Headers",
				strings.Join(accessControl.AllowHeaders, ", ")+", "+allowHeaders)
		}
		headers.Add("Access-Control-Allow-Methods", allowMethods)
		headers.Add("Access-Control-Max-Age", allowPreflightCacheAgeSecs)
	} else {
		headers.Add("Access-Control-Expose-Headers", exposeHeaders)
	}
}

// ServiceNames returns a sorted list of full names of all registered services.
func (s *Server) ServiceNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.services))
	for name := range s.services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
