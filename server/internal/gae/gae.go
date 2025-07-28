// Copyright 2021 The LUCI Authors.
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

// Package gae implements minimal support for using some bundled GAE APIs.
//
// It is essentially a reimplementation of a small subset of Golang GAE SDK v2,
// since the SDK is not very interoperable with non-GAE code,
// see e.g. https://github.com/golang/appengine/issues/265.
//
// Following sources were used as a base:
//   - Repo: https://github.com/golang/appengine
//   - Revision: ef2135aad62454e588006ef8beb7247022461e6c
//
// Some proto files were copied, and their proto package and `go_package`
// updated to reflect their new location to avoid clashing with real GAE SDK
// protos in the registry and to conform to modern protoc-gen-go requirement
// of using full Go package paths:
//   - v2/internal/base/api_base.proto => base/
//   - v2/internal/mail/mail_service.proto => mail/
//   - v2/internal/memcache/memcache_service.proto => memcache/
//   - v2/internal/remote_api/remote_api.proto => remote_api/
//
// The rest is written from scratch based on what the SDK is doing.
package gae

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	remotepb "go.chromium.org/luci/server/internal/gae/remote_api"
)

//go:generate cproto base
//go:generate cproto mail
//go:generate cproto memcache
//go:generate cproto remote_api

var (
	ticketsContextKey = "go.chromium.org/luci/server/internal/gae.Tickets"
	tracer            = otel.Tracer("go.chromium.org/luci/server/internal/gae")
)

// Note: Go GAE SDK attempts to limit the number of concurrent connections using
// a hand-rolled semaphore-based dialer. It is not clear why it can't just use
// MaxConnsPerHost. We use MaxConnsPerHost below for simplicity.
var apiHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxConnsPerHost:     200,
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 1000,
		IdleConnTimeout:     90 * time.Second,
	},
}

// Tickets lives in context.Context and carries per-request information.
type Tickets struct {
	api         string   // API ticket identifying the incoming HTTP request
	dapperTrace string   // Dapper Trace ticket
	apiURL      *url.URL // URL of the service bridge (overridden in tests)
}

// Headers knows how to return request headers.
type Headers interface {
	Header(string) string
}

// DefaultTickets generates default background tickets.
//
// They are used for calls outside of request handlers. Uses GAE environment
// variables.
func DefaultTickets() *Tickets {
	return &Tickets{
		api: fmt.Sprintf("%s/%s.%s.%s",
			strings.Replace(strings.Replace(os.Getenv("GOOGLE_CLOUD_PROJECT"), ":", "_", -1), ".", "_", -1),
			os.Getenv("GAE_SERVICE"),
			os.Getenv("GAE_VERSION"),
			os.Getenv("GAE_INSTANCE"),
		),
	}
}

// RequestTickets extracts tickets from incoming request headers.
func RequestTickets(headers Headers) *Tickets {
	return &Tickets{
		api:         headers.Header("X-Appengine-Api-Ticket"),
		dapperTrace: headers.Header("X-Google-Dappertraceinfo"),
	}
}

// WithTickets puts the tickets into the context.Context.
func WithTickets(ctx context.Context, tickets *Tickets) context.Context {
	return context.WithValue(ctx, &ticketsContextKey, tickets)
}

// Call makes an RPC to the GAE service bridge.
//
// Uses tickets in the context (see WithTickets). Returns an error if they are
// not there.
//
// Note: currently returns opaque stringy errors. Refactor if you need to
// distinguish API errors from transport errors or need error codes, etc.
func Call(ctx context.Context, service, method string, in, out proto.Message) (err error) {
	ctx, span := tracer.Start(ctx, fmt.Sprintf("go.chromium.org/luci/gae.Call/%s.%s", service, method))
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	tickets, _ := ctx.Value(&ticketsContextKey).(*Tickets)
	if tickets == nil {
		return errors.Fmt("no GAE API ticket in the context when calling %s.%s", service, method)
	}

	data, err := proto.Marshal(in)
	if err != nil {
		return errors.Fmt("failed to marshal RPC request to %s.%s: %w", service, method, err)
	}

	postBody, err := proto.Marshal(&remotepb.Request{
		ServiceName: &service,
		Method:      &method,
		Request:     data,
		RequestId:   &tickets.api,
	})
	if err != nil {
		return errors.Fmt("failed to marshal RPC request to %s.%s: %w", service, method, err)
	}

	respBody, err := postToServiceBridge(ctx, tickets, postBody)
	if err != nil {
		return errors.Fmt("failed to call GAE service bridge for %s.%s: %w", service, method, err)
	}

	res := &remotepb.Response{}
	if err := proto.Unmarshal(respBody, res); err != nil {
		return errors.Fmt("unexpected response from GAE service bridge for %s.%s: %w", service, method, err)
	}

	if res.RpcError != nil {
		return errors.Fmt("RPC error %s calling %s.%s: %s",
			remotepb.RpcError_ErrorCode(res.RpcError.GetCode()),
			service, method, res.RpcError.GetDetail())
	}

	if res.ApplicationError != nil {
		return errors.Fmt("API error %d calling %s.%s: %s",
			res.ApplicationError.GetCode(),
			service, method, res.ApplicationError.GetDetail())
	}

	// This should not be happening.
	if res.Exception != nil || res.JavaException != nil {
		return errors.Fmt("service bridge returned unexpected exception from %s.%s",
			service, method)
	}

	if err := proto.Unmarshal(res.Response, out); err != nil {
		return errors.Fmt("failed to unmarshal response of %s.%s: %w", service, method, err)
	}
	return nil
}

// apiURL is the URL of the local GAE service bridge.
func apiURL() *url.URL {
	host, port := "appengine.googleapis.internal", "10001"
	if h := os.Getenv("API_HOST"); h != "" {
		host = h
	}
	if p := os.Getenv("API_PORT"); p != "" {
		port = p
	}
	return &url.URL{
		Scheme: "http",
		Host:   host + ":" + port,
		Path:   "/rpc_http",
	}
}

// postToServiceBridge makes an HTTP POST request to the GAE service bridge.
func postToServiceBridge(ctx context.Context, tickets *Tickets, body []byte) ([]byte, error) {
	// Either get the existing context timeout or create the default 60 sec one.
	timeout := time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	} else {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	url := tickets.apiURL
	if url == nil {
		url = apiURL()
	}

	req := &http.Request{
		Method: "POST",
		URL:    url,
		Header: http.Header{
			"X-Google-Rpc-Service-Endpoint": []string{"app-engine-apis"},
			"X-Google-Rpc-Service-Method":   []string{"/VMRemoteAPI.CallRemoteAPI"},
			"X-Google-Rpc-Service-Deadline": []string{strconv.FormatFloat(timeout.Seconds(), 'f', -1, 64)},
			"Content-Type":                  []string{"application/octet-stream"},
		},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Host:          url.Host,
	}
	if tickets.dapperTrace != "" {
		req.Header.Set("X-Google-Dappertraceinfo", tickets.dapperTrace)
	}
	// This populates X-Cloud-Trace-Context header with the current trace and span
	// IDs. That way internal GAE spans are attached to the correct spans in our
	// code.
	(propagator.CloudTraceFormatPropagator{}).Inject(ctx, propagation.HeaderCarrier(req.Header))

	res, err := apiHTTPClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, errors.Fmt("failed to make HTTP call: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	switch body, err := io.ReadAll(res.Body); {
	case err != nil:
		return nil, errors.Fmt("failed to read HTTP %d response: %w", res.StatusCode, err)
	case res.StatusCode != 200:
		return nil, errors.Fmt("unexpected HTTP %d: %q", res.StatusCode, body)
	default:
		return body, nil
	}
}
