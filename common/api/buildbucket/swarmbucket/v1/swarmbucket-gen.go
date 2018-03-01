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

// Package swarmbucket provides access to the Buildbucket-Swarming integration.
//
// Usage example:
//
//   import "go.chromium.org/luci/common/api/buildbucket/swarmbucket/v1"
//   ...
//   swarmbucketService, err := swarmbucket.New(oauthHttpClient)
package swarmbucket // import "go.chromium.org/luci/common/api/buildbucket/swarmbucket/v1"

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

const apiId = "swarmbucket:v1"
const apiName = "swarmbucket"
const apiVersion = "v1"
const basePath = "http://localhost:8080/_ah/api/swarmbucket/v1/"

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

type ApiPubSubCallbackMessage struct {
	AuthToken string `json:"auth_token,omitempty"`

	Topic string `json:"topic,omitempty"`

	UserData string `json:"user_data,omitempty"`

	// ForceSendFields is a list of field names (e.g. "AuthToken") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "AuthToken") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ApiPubSubCallbackMessage) MarshalJSON() ([]byte, error) {
	type NoMethod ApiPubSubCallbackMessage
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

type ApiPutRequestMessage struct {
	Bucket string `json:"bucket,omitempty"`

	// Possible values:
	//   "AUTO"
	//   "CANARY"
	//   "PROD"
	CanaryPreference string `json:"canary_preference,omitempty"`

	ClientOperationId string `json:"client_operation_id,omitempty"`

	Experimental bool `json:"experimental,omitempty"`

	LeaseExpirationTs int64 `json:"lease_expiration_ts,omitempty,string"`

	ParametersJson string `json:"parameters_json,omitempty"`

	PubsubCallback *ApiPubSubCallbackMessage `json:"pubsub_callback,omitempty"`

	Tags []string `json:"tags,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Bucket") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Bucket") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ApiPutRequestMessage) MarshalJSON() ([]byte, error) {
	type NoMethod ApiPutRequestMessage
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

type SwarmingSwarmbucketApiBucketMessage struct {
	Builders []*SwarmingSwarmbucketApiBuilderMessage `json:"builders,omitempty"`

	Name string `json:"name,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Builders") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Builders") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingSwarmbucketApiBucketMessage) MarshalJSON() ([]byte, error) {
	type NoMethod SwarmingSwarmbucketApiBucketMessage
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

type SwarmingSwarmbucketApiBuilderMessage struct {
	Category string `json:"category,omitempty"`

	Name string `json:"name,omitempty"`

	PropertiesJson string `json:"properties_json,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Category") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Category") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingSwarmbucketApiBuilderMessage) MarshalJSON() ([]byte, error) {
	type NoMethod SwarmingSwarmbucketApiBuilderMessage
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

type SwarmingSwarmbucketApiGetBuildersResponseMessage struct {
	Buckets []*SwarmingSwarmbucketApiBucketMessage `json:"buckets,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Buckets") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Buckets") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingSwarmbucketApiGetBuildersResponseMessage) MarshalJSON() ([]byte, error) {
	type NoMethod SwarmingSwarmbucketApiGetBuildersResponseMessage
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

type SwarmingSwarmbucketApiGetTaskDefinitionRequestMessage struct {
	BuildRequest *ApiPutRequestMessage `json:"build_request,omitempty"`

	// ForceSendFields is a list of field names (e.g. "BuildRequest") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "BuildRequest") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingSwarmbucketApiGetTaskDefinitionRequestMessage) MarshalJSON() ([]byte, error) {
	type NoMethod SwarmingSwarmbucketApiGetTaskDefinitionRequestMessage
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

type SwarmingSwarmbucketApiGetTaskDefinitionResponseMessage struct {
	TaskDefinition string `json:"task_definition,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "TaskDefinition") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "TaskDefinition") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingSwarmbucketApiGetTaskDefinitionResponseMessage) MarshalJSON() ([]byte, error) {
	type NoMethod SwarmingSwarmbucketApiGetTaskDefinitionResponseMessage
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

type SwarmingSwarmbucketApiSetNextBuildNumberRequest struct {
	Bucket string `json:"bucket,omitempty"`

	Builder string `json:"builder,omitempty"`

	NextNumber int64 `json:"next_number,omitempty,string"`

	// ForceSendFields is a list of field names (e.g. "Bucket") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Bucket") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingSwarmbucketApiSetNextBuildNumberRequest) MarshalJSON() ([]byte, error) {
	type NoMethod SwarmingSwarmbucketApiSetNextBuildNumberRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// method id "swarmbucket.get_builders":

type GetBuildersCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// GetBuilders: Returns defined swarmbucket builders. Can be used by
// code review tool to discover builders.
func (s *Service) GetBuilders() *GetBuildersCall {
	c := &GetBuildersCall{s: s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *GetBuildersCall) Fields(s ...googleapi.Field) *GetBuildersCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *GetBuildersCall) IfNoneMatch(entityTag string) *GetBuildersCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *GetBuildersCall) Context(ctx context.Context) *GetBuildersCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *GetBuildersCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *GetBuildersCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "builders")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarmbucket.get_builders" call.
// Exactly one of *SwarmingSwarmbucketApiGetBuildersResponseMessage or
// error will be non-nil. Any non-2xx status code is an error. Response
// headers are in either
// *SwarmingSwarmbucketApiGetBuildersResponseMessage.ServerResponse.Heade
// r or (if a response was returned at all) in
// error.(*googleapi.Error).Header. Use googleapi.IsNotModified to check
// whether the returned error was because http.StatusNotModified was
// returned.
func (c *GetBuildersCall) Do(opts ...googleapi.CallOption) (*SwarmingSwarmbucketApiGetBuildersResponseMessage, error) {
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
	ret := &SwarmingSwarmbucketApiGetBuildersResponseMessage{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns defined swarmbucket builders. Can be used by code review tool to discover builders.",
	//   "httpMethod": "GET",
	//   "id": "swarmbucket.get_builders",
	//   "path": "builders",
	//   "response": {
	//     "$ref": "SwarmingSwarmbucketApiGetBuildersResponseMessage"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarmbucket.get_task_def":

type GetTaskDefCall struct {
	s                                                     *Service
	swarmingswarmbucketapigettaskdefinitionrequestmessage *SwarmingSwarmbucketApiGetTaskDefinitionRequestMessage
	urlParams_                                            gensupport.URLParams
	ctx_                                                  context.Context
	header_                                               http.Header
}

// GetTaskDef: Returns a swarming task definition for a build request.
func (s *Service) GetTaskDef(swarmingswarmbucketapigettaskdefinitionrequestmessage *SwarmingSwarmbucketApiGetTaskDefinitionRequestMessage) *GetTaskDefCall {
	c := &GetTaskDefCall{s: s, urlParams_: make(gensupport.URLParams)}
	c.swarmingswarmbucketapigettaskdefinitionrequestmessage = swarmingswarmbucketapigettaskdefinitionrequestmessage
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *GetTaskDefCall) Fields(s ...googleapi.Field) *GetTaskDefCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *GetTaskDefCall) Context(ctx context.Context) *GetTaskDefCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *GetTaskDefCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *GetTaskDefCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.swarmingswarmbucketapigettaskdefinitionrequestmessage)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "get_task_def")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarmbucket.get_task_def" call.
// Exactly one of
// *SwarmingSwarmbucketApiGetTaskDefinitionResponseMessage or error will
// be non-nil. Any non-2xx status code is an error. Response headers are
// in either
// *SwarmingSwarmbucketApiGetTaskDefinitionResponseMessage.ServerResponse
// .Header or (if a response was returned at all) in
// error.(*googleapi.Error).Header. Use googleapi.IsNotModified to check
// whether the returned error was because http.StatusNotModified was
// returned.
func (c *GetTaskDefCall) Do(opts ...googleapi.CallOption) (*SwarmingSwarmbucketApiGetTaskDefinitionResponseMessage, error) {
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
	ret := &SwarmingSwarmbucketApiGetTaskDefinitionResponseMessage{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns a swarming task definition for a build request.",
	//   "httpMethod": "POST",
	//   "id": "swarmbucket.get_task_def",
	//   "path": "get_task_def",
	//   "request": {
	//     "$ref": "SwarmingSwarmbucketApiGetTaskDefinitionRequestMessage",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "SwarmingSwarmbucketApiGetTaskDefinitionResponseMessage"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarmbucket.set_next_build_number":

type SetNextBuildNumberCall struct {
	s                                               *Service
	swarmingswarmbucketapisetnextbuildnumberrequest *SwarmingSwarmbucketApiSetNextBuildNumberRequest
	urlParams_                                      gensupport.URLParams
	ctx_                                            context.Context
	header_                                         http.Header
}

// SetNextBuildNumber: Sets the build number that will be used for the
// next build.
func (s *Service) SetNextBuildNumber(swarmingswarmbucketapisetnextbuildnumberrequest *SwarmingSwarmbucketApiSetNextBuildNumberRequest) *SetNextBuildNumberCall {
	c := &SetNextBuildNumberCall{s: s, urlParams_: make(gensupport.URLParams)}
	c.swarmingswarmbucketapisetnextbuildnumberrequest = swarmingswarmbucketapisetnextbuildnumberrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *SetNextBuildNumberCall) Fields(s ...googleapi.Field) *SetNextBuildNumberCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *SetNextBuildNumberCall) Context(ctx context.Context) *SetNextBuildNumberCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *SetNextBuildNumberCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *SetNextBuildNumberCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.swarmingswarmbucketapisetnextbuildnumberrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "set_next_build_number")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarmbucket.set_next_build_number" call.
func (c *SetNextBuildNumberCall) Do(opts ...googleapi.CallOption) error {
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
	//   "description": "Sets the build number that will be used for the next build.",
	//   "httpMethod": "POST",
	//   "id": "swarmbucket.set_next_build_number",
	//   "path": "set_next_build_number",
	//   "request": {
	//     "$ref": "SwarmingSwarmbucketApiSetNextBuildNumberRequest",
	//     "parameterName": "resource"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}
