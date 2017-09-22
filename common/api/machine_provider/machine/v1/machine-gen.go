// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package machine provides access to the .
//
// Usage example:
//
//   import "go.chromium.org/luci/common/api/machine_provider/machine/v1"
//   ...
//   machineService, err := machine.New(oauthHttpClient)
package machine // import "go.chromium.org/luci/common/api/machine_provider/machine/v1"

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	context "golang.org/x/net/context"
	ctxhttp "golang.org/x/net/context/ctxhttp"
	gensupport "google.golang.org/api/gensupport"
	googleapi "google.golang.org/api/googleapi"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Always reference these packages, just in case the auto-generated code
// below doesn't.
var _ = bytes.NewBuffer
var _ = strconv.Itoa
var _ = fmt.Sprintf
var _ = json.NewDecoder
var _ = io.Copy
var _ = url.Parse
var _ = gensupport.MarshalJSON
var _ = googleapi.Version
var _ = errors.New
var _ = strings.Replace
var _ = context.Canceled
var _ = ctxhttp.Do

const apiId = "machine:v1"
const apiName = "machine"
const apiVersion = "v1"
const basePath = "http://localhost:8080/_ah/api/machine/v1/"

// OAuth2 scopes used by this API.
const (
	// View your email address
	UserinfoEmailScope = "https://www.googleapis.com/auth/userinfo.email"
)

func New(client *http.Client) (*Service, error) {
	if client == nil {
		return nil, errors.New("client is nil")
	}
	s := &Service{client: client, BasePath: basePath}
	return s, nil
}

type Service struct {
	client    *http.Client
	BasePath  string // API endpoint base URL
	UserAgent string // optional additional User-Agent fragment
}

func (s *Service) userAgent() string {
	if s.UserAgent == "" {
		return googleapi.UserAgent
	}
	return googleapi.UserAgent + " " + s.UserAgent
}

// ComponentsMachineProviderInstructionsInstruction: Represents
// instructions for a machine.
type ComponentsMachineProviderInstructionsInstruction struct {
	SwarmingServer string `json:"swarming_server,omitempty"`

	// ForceSendFields is a list of field names (e.g. "SwarmingServer") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "SwarmingServer") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *ComponentsMachineProviderInstructionsInstruction) MarshalJSON() ([]byte, error) {
	type noMethod ComponentsMachineProviderInstructionsInstruction
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ComponentsMachineProviderRpcMessagesAckRequest: Represents a request
// to ack an instruction received by a machine.
type ComponentsMachineProviderRpcMessagesAckRequest struct {
	// Possible values:
	//   "DUMMY"
	//   "GCE"
	//   "VSPHERE"
	Backend string `json:"backend,omitempty"`

	Hostname string `json:"hostname,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Backend") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Backend") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ComponentsMachineProviderRpcMessagesAckRequest) MarshalJSON() ([]byte, error) {
	type noMethod ComponentsMachineProviderRpcMessagesAckRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ComponentsMachineProviderRpcMessagesPollRequest: Represents a request
// to poll for instructions given to a machine.
type ComponentsMachineProviderRpcMessagesPollRequest struct {
	// Possible values:
	//   "DUMMY"
	//   "GCE"
	//   "VSPHERE"
	Backend string `json:"backend,omitempty"`

	Hostname string `json:"hostname,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Backend") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Backend") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ComponentsMachineProviderRpcMessagesPollRequest) MarshalJSON() ([]byte, error) {
	type noMethod ComponentsMachineProviderRpcMessagesPollRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ComponentsMachineProviderRpcMessagesPollResponse: Represents a
// response to a request for instructions given to a machine.
type ComponentsMachineProviderRpcMessagesPollResponse struct {
	// Instruction: Represents instructions for a machine.
	Instruction *ComponentsMachineProviderInstructionsInstruction `json:"instruction,omitempty"`

	State string `json:"state,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Instruction") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Instruction") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ComponentsMachineProviderRpcMessagesPollResponse) MarshalJSON() ([]byte, error) {
	type noMethod ComponentsMachineProviderRpcMessagesPollResponse
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// method id "machine.ack":

type AckCall struct {
	s                                              *Service
	componentsmachineproviderrpcmessagesackrequest *ComponentsMachineProviderRpcMessagesAckRequest
	urlParams_                                     gensupport.URLParams
	ctx_                                           context.Context
	header_                                        http.Header
}

// Ack: Handles an incoming AckRequest.
func (s *Service) Ack(componentsmachineproviderrpcmessagesackrequest *ComponentsMachineProviderRpcMessagesAckRequest) *AckCall {
	c := &AckCall{s: s, urlParams_: make(gensupport.URLParams)}
	c.componentsmachineproviderrpcmessagesackrequest = componentsmachineproviderrpcmessagesackrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *AckCall) Fields(s ...googleapi.Field) *AckCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *AckCall) Context(ctx context.Context) *AckCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *AckCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *AckCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.componentsmachineproviderrpcmessagesackrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "ack")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "machine.ack" call.
func (c *AckCall) Do(opts ...googleapi.CallOption) error {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if err != nil {
		return err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return err
	}
	return nil
	// {
	//   "description": "Handles an incoming AckRequest.",
	//   "httpMethod": "POST",
	//   "id": "machine.ack",
	//   "path": "ack",
	//   "request": {
	//     "$ref": "ComponentsMachineProviderRpcMessagesAckRequest",
	//     "parameterName": "resource"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "machine.poll":

type PollCall struct {
	s                                               *Service
	componentsmachineproviderrpcmessagespollrequest *ComponentsMachineProviderRpcMessagesPollRequest
	urlParams_                                      gensupport.URLParams
	ctx_                                            context.Context
	header_                                         http.Header
}

// Poll: Handles an incoming PollRequest.
func (s *Service) Poll(componentsmachineproviderrpcmessagespollrequest *ComponentsMachineProviderRpcMessagesPollRequest) *PollCall {
	c := &PollCall{s: s, urlParams_: make(gensupport.URLParams)}
	c.componentsmachineproviderrpcmessagespollrequest = componentsmachineproviderrpcmessagespollrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *PollCall) Fields(s ...googleapi.Field) *PollCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *PollCall) Context(ctx context.Context) *PollCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *PollCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *PollCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.componentsmachineproviderrpcmessagespollrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "poll")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "machine.poll" call.
// Exactly one of *ComponentsMachineProviderRpcMessagesPollResponse or
// error will be non-nil. Any non-2xx status code is an error. Response
// headers are in either
// *ComponentsMachineProviderRpcMessagesPollResponse.ServerResponse.Heade
// r or (if a response was returned at all) in
// error.(*googleapi.Error).Header. Use googleapi.IsNotModified to check
// whether the returned error was because http.StatusNotModified was
// returned.
func (c *PollCall) Do(opts ...googleapi.CallOption) (*ComponentsMachineProviderRpcMessagesPollResponse, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &ComponentsMachineProviderRpcMessagesPollResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Handles an incoming PollRequest.",
	//   "httpMethod": "POST",
	//   "id": "machine.poll",
	//   "path": "poll",
	//   "request": {
	//     "$ref": "ComponentsMachineProviderRpcMessagesPollRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "ComponentsMachineProviderRpcMessagesPollResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}
