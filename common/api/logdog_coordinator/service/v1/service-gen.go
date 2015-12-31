// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package service provides access to the .
//
// Usage example:
//
//   import "github.com/luci/luci-go/common/api/logdog_coordinator/service/v1"
//   ...
//   serviceService, err := service.New(oauthHttpClient)
package service // import "github.com/luci/luci-go/common/api/logdog_coordinator/service/v1"

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

const apiId = "service:v1"
const apiName = "service"
const apiVersion = "v1"
const basePath = "http://localhost:8080/api/service/v1/"

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

type GetConfigResponse struct {
	ConfigPath string `json:"configPath,omitempty"`

	ConfigService string `json:"configService,omitempty"`

	ConfigSet string `json:"configSet,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "ConfigPath") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *GetConfigResponse) MarshalJSON() ([]byte, error) {
	type noMethod GetConfigResponse
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type LoadStreamResponse struct {
	Descriptor string `json:"descriptor,omitempty"`

	Path string `json:"path,omitempty"`

	Secret string `json:"secret,omitempty"`

	State *LogStreamState `json:"state,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Descriptor") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *LoadStreamResponse) MarshalJSON() ([]byte, error) {
	type noMethod LoadStreamResponse
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type LogStreamState struct {
	ArchiveDataURL string `json:"archiveDataURL,omitempty"`

	ArchiveIndexURL string `json:"archiveIndexURL,omitempty"`

	ArchiveStreamURL string `json:"archiveStreamURL,omitempty"`

	Created string `json:"created,omitempty"`

	ProtoVersion string `json:"protoVersion,omitempty"`

	Purged bool `json:"purged,omitempty"`

	TerminalIndex int64 `json:"terminalIndex,omitempty,string"`

	Updated string `json:"updated,omitempty"`

	// ForceSendFields is a list of field names (e.g. "ArchiveDataURL") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *LogStreamState) MarshalJSON() ([]byte, error) {
	type noMethod LogStreamState
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type RegisterStreamRequest struct {
	Descriptor string `json:"descriptor,omitempty"`

	Path string `json:"path,omitempty"`

	ProtoVersion string `json:"protoVersion,omitempty"`

	Secret string `json:"secret,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Descriptor") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *RegisterStreamRequest) MarshalJSON() ([]byte, error) {
	type noMethod RegisterStreamRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type RegisterStreamResponse struct {
	Path string `json:"path,omitempty"`

	Secret string `json:"secret,omitempty"`

	State *LogStreamState `json:"state,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Path") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *RegisterStreamResponse) MarshalJSON() ([]byte, error) {
	type noMethod RegisterStreamResponse
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type TerminateStreamRequest struct {
	Path string `json:"path,omitempty"`

	Secret string `json:"secret,omitempty"`

	TerminalIndex int64 `json:"terminalIndex,omitempty,string"`

	// ForceSendFields is a list of field names (e.g. "Path") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *TerminateStreamRequest) MarshalJSON() ([]byte, error) {
	type noMethod TerminateStreamRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// method id "service.GetConfig":

type GetConfigCall struct {
	s    *Service
	opt_ map[string]interface{}
	ctx_ context.Context
}

// GetConfig: Load service configuration parameters.
func (s *Service) GetConfig() *GetConfigCall {
	c := &GetConfigCall{s: s, opt_: make(map[string]interface{})}
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *GetConfigCall) Fields(s ...googleapi.Field) *GetConfigCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *GetConfigCall) IfNoneMatch(entityTag string) *GetConfigCall {
	c.opt_["ifNoneMatch"] = entityTag
	return c
}

// Context sets the context to be used in this call's Do method.
// Any pending HTTP request will be aborted if the provided context
// is canceled.
func (c *GetConfigCall) Context(ctx context.Context) *GetConfigCall {
	c.ctx_ = ctx
	return c
}

func (c *GetConfigCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "getConfig")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("User-Agent", c.s.userAgent())
	if v, ok := c.opt_["ifNoneMatch"]; ok {
		req.Header.Set("If-None-Match", fmt.Sprintf("%v", v))
	}
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "service.GetConfig" call.
// Exactly one of *GetConfigResponse or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *GetConfigResponse.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *GetConfigCall) Do() (*GetConfigResponse, error) {
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
	ret := &GetConfigResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Load service configuration parameters.",
	//   "httpMethod": "GET",
	//   "id": "service.GetConfig",
	//   "path": "getConfig",
	//   "response": {
	//     "$ref": "GetConfigResponse",
	//     "parameterName": "resource"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "service.LoadStream":

type LoadStreamCall struct {
	s    *Service
	opt_ map[string]interface{}
	ctx_ context.Context
}

// LoadStream: Loads log stream metadata.
func (s *Service) LoadStream() *LoadStreamCall {
	c := &LoadStreamCall{s: s, opt_: make(map[string]interface{})}
	return c
}

// Path sets the optional parameter "Path":
func (c *LoadStreamCall) Path(Path string) *LoadStreamCall {
	c.opt_["Path"] = Path
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *LoadStreamCall) Fields(s ...googleapi.Field) *LoadStreamCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *LoadStreamCall) IfNoneMatch(entityTag string) *LoadStreamCall {
	c.opt_["ifNoneMatch"] = entityTag
	return c
}

// Context sets the context to be used in this call's Do method.
// Any pending HTTP request will be aborted if the provided context
// is canceled.
func (c *LoadStreamCall) Context(ctx context.Context) *LoadStreamCall {
	c.ctx_ = ctx
	return c
}

func (c *LoadStreamCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["Path"]; ok {
		params.Set("Path", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "loadStream")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("User-Agent", c.s.userAgent())
	if v, ok := c.opt_["ifNoneMatch"]; ok {
		req.Header.Set("If-None-Match", fmt.Sprintf("%v", v))
	}
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "service.LoadStream" call.
// Exactly one of *LoadStreamResponse or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *LoadStreamResponse.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *LoadStreamCall) Do() (*LoadStreamResponse, error) {
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
	ret := &LoadStreamResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Loads log stream metadata.",
	//   "httpMethod": "GET",
	//   "id": "service.LoadStream",
	//   "parameters": {
	//     "Path": {
	//       "location": "query",
	//       "type": "string"
	//     }
	//   },
	//   "path": "loadStream",
	//   "response": {
	//     "$ref": "LoadStreamResponse",
	//     "parameterName": "resource"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "service.RegisterStream":

type RegisterStreamCall struct {
	s                     *Service
	registerstreamrequest *RegisterStreamRequest
	opt_                  map[string]interface{}
	ctx_                  context.Context
}

// RegisterStream: Registers a log stream.
func (s *Service) RegisterStream(registerstreamrequest *RegisterStreamRequest) *RegisterStreamCall {
	c := &RegisterStreamCall{s: s, opt_: make(map[string]interface{})}
	c.registerstreamrequest = registerstreamrequest
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *RegisterStreamCall) Fields(s ...googleapi.Field) *RegisterStreamCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

// Context sets the context to be used in this call's Do method.
// Any pending HTTP request will be aborted if the provided context
// is canceled.
func (c *RegisterStreamCall) Context(ctx context.Context) *RegisterStreamCall {
	c.ctx_ = ctx
	return c
}

func (c *RegisterStreamCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.registerstreamrequest)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "registerStream")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("PUT", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "service.RegisterStream" call.
// Exactly one of *RegisterStreamResponse or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *RegisterStreamResponse.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *RegisterStreamCall) Do() (*RegisterStreamResponse, error) {
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
	ret := &RegisterStreamResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Registers a log stream.",
	//   "httpMethod": "PUT",
	//   "id": "service.RegisterStream",
	//   "path": "registerStream",
	//   "request": {
	//     "$ref": "RegisterStreamRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "RegisterStreamResponse",
	//     "parameterName": "resource"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "service.TerminateStream":

type TerminateStreamCall struct {
	s                      *Service
	terminatestreamrequest *TerminateStreamRequest
	opt_                   map[string]interface{}
	ctx_                   context.Context
}

// TerminateStream: Register a log stream's terminal index.
func (s *Service) TerminateStream(terminatestreamrequest *TerminateStreamRequest) *TerminateStreamCall {
	c := &TerminateStreamCall{s: s, opt_: make(map[string]interface{})}
	c.terminatestreamrequest = terminatestreamrequest
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *TerminateStreamCall) Fields(s ...googleapi.Field) *TerminateStreamCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

// Context sets the context to be used in this call's Do method.
// Any pending HTTP request will be aborted if the provided context
// is canceled.
func (c *TerminateStreamCall) Context(ctx context.Context) *TerminateStreamCall {
	c.ctx_ = ctx
	return c
}

func (c *TerminateStreamCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.terminatestreamrequest)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "terminateStream")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("PUT", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "service.TerminateStream" call.
func (c *TerminateStreamCall) Do() error {
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
	//   "description": "Register a log stream's terminal index.",
	//   "httpMethod": "PUT",
	//   "id": "service.TerminateStream",
	//   "path": "terminateStream",
	//   "request": {
	//     "$ref": "TerminateStreamRequest",
	//     "parameterName": "resource"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}
