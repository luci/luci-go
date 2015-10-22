// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dumb_counter provides access to the .
//
// Usage example:
//
//   import "github.com/luci/luci-go/appengine/apigen_examples/dumb_counter/api/dumb_counter/v1"
//   ...
//   dumb_counterService, err := dumb_counter.New(oauthHttpClient)
package dumb_counter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/internal"
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
var _ = googleapi.Version
var _ = errors.New
var _ = strings.Replace
var _ = internal.MarshalJSON

const apiId = "dumb_counter:v1"
const apiName = "dumb_counter"
const apiVersion = "v1"
const basePath = "https://counter.example.com/_ah/api/dumb_counter/v1/"

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

type AddReq struct {
	Delta int64 `json:"Delta,omitempty,string"`

	Name string `json:"Name,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Delta") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *AddReq) MarshalJSON() ([]byte, error) {
	type noMethod AddReq
	raw := noMethod(*s)
	return internal.MarshalJSON(raw, s.ForceSendFields)
}

type AddRsp struct {
	Cur int64 `json:"Cur,omitempty,string"`

	Prev int64 `json:"Prev,omitempty,string"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Cur") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *AddRsp) MarshalJSON() ([]byte, error) {
	type noMethod AddRsp
	raw := noMethod(*s)
	return internal.MarshalJSON(raw, s.ForceSendFields)
}

type CASReq struct {
	Name string `json:"Name,omitempty"`

	NewVal int64 `json:"NewVal,omitempty,string"`

	OldVal int64 `json:"OldVal,omitempty,string"`

	// ForceSendFields is a list of field names (e.g. "Name") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *CASReq) MarshalJSON() ([]byte, error) {
	type noMethod CASReq
	raw := noMethod(*s)
	return internal.MarshalJSON(raw, s.ForceSendFields)
}

type Counter struct {
	Name string `json:"Name,omitempty"`

	Val int64 `json:"Val,omitempty,string"`

	// ForceSendFields is a list of field names (e.g. "Name") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *Counter) MarshalJSON() ([]byte, error) {
	type noMethod Counter
	raw := noMethod(*s)
	return internal.MarshalJSON(raw, s.ForceSendFields)
}

type CurrentValueRsp struct {
	Val int64 `json:"Val,omitempty,string"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Val") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *CurrentValueRsp) MarshalJSON() ([]byte, error) {
	type noMethod CurrentValueRsp
	raw := noMethod(*s)
	return internal.MarshalJSON(raw, s.ForceSendFields)
}

type ListRsp struct {
	Counters []*Counter `json:"Counters,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Counters") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *ListRsp) MarshalJSON() ([]byte, error) {
	type noMethod ListRsp
	raw := noMethod(*s)
	return internal.MarshalJSON(raw, s.ForceSendFields)
}

// method id "dumb_counter.add":

type AddCall struct {
	s      *Service
	Name   string
	addreq *AddReq
	opt_   map[string]interface{}
	ctx_   context.Context
}

// Add: Add an an amount to a particular counter
func (s *Service) Add(Name string, addreq *AddReq) *AddCall {
	c := &AddCall{s: s, opt_: make(map[string]interface{})}
	c.Name = Name
	c.addreq = addreq
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *AddCall) Fields(s ...googleapi.Field) *AddCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

// Context sets the context to be used in this call's Do method.
// Any pending HTTP request will be aborted if the provided context
// is canceled.
func (c *AddCall) Context(ctx context.Context) *AddCall {
	c.ctx_ = ctx
	return c
}

func (c *AddCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.addreq)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "counter/{Name}")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"Name": c.Name,
	})
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "dumb_counter.add" call.
// Exactly one of *AddRsp or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *AddRsp.ServerResponse.Header or (if a response was returned at all)
// in error.(*googleapi.Error).Header. Use googleapi.IsNotModified to
// check whether the returned error was because http.StatusNotModified
// was returned.
func (c *AddCall) Do() (*AddRsp, error) {
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
	ret := &AddRsp{
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
	//   "description": "Add an an amount to a particular counter",
	//   "httpMethod": "POST",
	//   "id": "dumb_counter.add",
	//   "parameterOrder": [
	//     "Name"
	//   ],
	//   "parameters": {
	//     "Name": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "counter/{Name}",
	//   "request": {
	//     "$ref": "AddReq",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "AddRsp"
	//   }
	// }

}

// method id "dumb_counter.cas":

type CasCall struct {
	s      *Service
	Name   string
	casreq *CASReq
	opt_   map[string]interface{}
	ctx_   context.Context
}

// Cas: Compare and swap a counter value
func (s *Service) Cas(Name string, casreq *CASReq) *CasCall {
	c := &CasCall{s: s, opt_: make(map[string]interface{})}
	c.Name = Name
	c.casreq = casreq
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *CasCall) Fields(s ...googleapi.Field) *CasCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

// Context sets the context to be used in this call's Do method.
// Any pending HTTP request will be aborted if the provided context
// is canceled.
func (c *CasCall) Context(ctx context.Context) *CasCall {
	c.ctx_ = ctx
	return c
}

func (c *CasCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.casreq)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "counter/{Name}/cas")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"Name": c.Name,
	})
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "dumb_counter.cas" call.
func (c *CasCall) Do() error {
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
	//   "description": "Compare and swap a counter value",
	//   "httpMethod": "POST",
	//   "id": "dumb_counter.cas",
	//   "parameterOrder": [
	//     "Name"
	//   ],
	//   "parameters": {
	//     "Name": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "counter/{Name}/cas",
	//   "request": {
	//     "$ref": "CASReq",
	//     "parameterName": "resource"
	//   }
	// }

}

// method id "dumb_counter.currentvalue":

type CurrentvalueCall struct {
	s    *Service
	Name string
	opt_ map[string]interface{}
	ctx_ context.Context
}

// Currentvalue: Returns the current value held by the named counter
func (s *Service) Currentvalue(Name string) *CurrentvalueCall {
	c := &CurrentvalueCall{s: s, opt_: make(map[string]interface{})}
	c.Name = Name
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *CurrentvalueCall) Fields(s ...googleapi.Field) *CurrentvalueCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *CurrentvalueCall) IfNoneMatch(entityTag string) *CurrentvalueCall {
	c.opt_["ifNoneMatch"] = entityTag
	return c
}

// Context sets the context to be used in this call's Do method.
// Any pending HTTP request will be aborted if the provided context
// is canceled.
func (c *CurrentvalueCall) Context(ctx context.Context) *CurrentvalueCall {
	c.ctx_ = ctx
	return c
}

func (c *CurrentvalueCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "counter/{Name}")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"Name": c.Name,
	})
	req.Header.Set("User-Agent", c.s.userAgent())
	if v, ok := c.opt_["ifNoneMatch"]; ok {
		req.Header.Set("If-None-Match", fmt.Sprintf("%v", v))
	}
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "dumb_counter.currentvalue" call.
// Exactly one of *CurrentValueRsp or error will be non-nil. Any non-2xx
// status code is an error. Response headers are in either
// *CurrentValueRsp.ServerResponse.Header or (if a response was returned
// at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *CurrentvalueCall) Do() (*CurrentValueRsp, error) {
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
	ret := &CurrentValueRsp{
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
	//   "description": "Returns the current value held by the named counter",
	//   "httpMethod": "GET",
	//   "id": "dumb_counter.currentvalue",
	//   "parameterOrder": [
	//     "Name"
	//   ],
	//   "parameters": {
	//     "Name": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "counter/{Name}",
	//   "response": {
	//     "$ref": "CurrentValueRsp"
	//   }
	// }

}

// method id "dumb_counter.list":

type ListCall struct {
	s    *Service
	opt_ map[string]interface{}
	ctx_ context.Context
}

// List: Returns all of the available counters
func (s *Service) List() *ListCall {
	c := &ListCall{s: s, opt_: make(map[string]interface{})}
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ListCall) Fields(s ...googleapi.Field) *ListCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ListCall) IfNoneMatch(entityTag string) *ListCall {
	c.opt_["ifNoneMatch"] = entityTag
	return c
}

// Context sets the context to be used in this call's Do method.
// Any pending HTTP request will be aborted if the provided context
// is canceled.
func (c *ListCall) Context(ctx context.Context) *ListCall {
	c.ctx_ = ctx
	return c
}

func (c *ListCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "counter")
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

// Do executes the "dumb_counter.list" call.
// Exactly one of *ListRsp or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *ListRsp.ServerResponse.Header or (if a response was returned at all)
// in error.(*googleapi.Error).Header. Use googleapi.IsNotModified to
// check whether the returned error was because http.StatusNotModified
// was returned.
func (c *ListCall) Do() (*ListRsp, error) {
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
	ret := &ListRsp{
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
	//   "description": "Returns all of the available counters",
	//   "httpMethod": "GET",
	//   "id": "dumb_counter.list",
	//   "path": "counter",
	//   "response": {
	//     "$ref": "ListRsp"
	//   }
	// }

}
