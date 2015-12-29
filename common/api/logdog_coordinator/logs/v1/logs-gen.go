// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package logs provides access to the .
//
// Usage example:
//
//   import "github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
//   ...
//   logsService, err := logs.New(oauthHttpClient)
package logs // import "github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"

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

const apiId = "logs:v1"
const apiName = "logs"
const apiVersion = "v1"
const basePath = "http://localhost:8080/api/logs/v1/"

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

type GetLogEntry struct {
	Entry *LogEntry `json:"entry,omitempty"`

	Proto string `json:"proto,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Entry") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *GetLogEntry) MarshalJSON() ([]byte, error) {
	type noMethod GetLogEntry
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type GetResponse struct {
	Descriptor *LogStreamDescriptor `json:"descriptor,omitempty"`

	DescriptorProto string `json:"descriptorProto,omitempty"`

	Logs []*GetLogEntry `json:"logs,omitempty"`

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

func (s *GetResponse) MarshalJSON() ([]byte, error) {
	type noMethod GetResponse
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type LogEntry struct {
	Binary string `json:"binary,omitempty"`

	Datagram *LogEntryDatagram `json:"datagram,omitempty"`

	PrefixIndex int64 `json:"prefix_index,omitempty,string"`

	Sequence int64 `json:"sequence,omitempty,string"`

	StreamIndex int64 `json:"stream_index,omitempty,string"`

	Text []string `json:"text,omitempty"`

	Timestamp string `json:"timestamp,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Binary") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *LogEntry) MarshalJSON() ([]byte, error) {
	type noMethod LogEntry
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type LogEntryDatagram struct {
	Partial *LogEntryDatagramPartial `json:"Partial,omitempty"`

	Data string `json:"data,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Partial") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *LogEntryDatagram) MarshalJSON() ([]byte, error) {
	type noMethod LogEntryDatagram
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type LogEntryDatagramPartial struct {
	Index int64 `json:"index,omitempty"`

	Last bool `json:"last,omitempty"`

	Size int64 `json:"size,omitempty,string"`

	// ForceSendFields is a list of field names (e.g. "Index") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *LogEntryDatagramPartial) MarshalJSON() ([]byte, error) {
	type noMethod LogEntryDatagramPartial
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type LogStreamDescriptor struct {
	BinaryFileExt string `json:"binary_file_ext,omitempty"`

	ContentType string `json:"content_type,omitempty"`

	Name string `json:"name,omitempty"`

	Prefix string `json:"prefix,omitempty"`

	StreamType string `json:"stream_type,omitempty"`

	Tags []*LogStreamDescriptorTag `json:"tags,omitempty"`

	Timestamp string `json:"timestamp,omitempty"`

	// ForceSendFields is a list of field names (e.g. "BinaryFileExt") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *LogStreamDescriptor) MarshalJSON() ([]byte, error) {
	type noMethod LogStreamDescriptor
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type LogStreamDescriptorTag struct {
	Key string `json:"key,omitempty"`

	Value string `json:"value,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Key") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *LogStreamDescriptorTag) MarshalJSON() ([]byte, error) {
	type noMethod LogStreamDescriptorTag
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

type QueryRequest struct {
	Archived string `json:"archived,omitempty"`

	ContentType string `json:"contentType,omitempty"`

	MaxResults int64 `json:"maxResults,omitempty"`

	Newer string `json:"newer,omitempty"`

	Next string `json:"next,omitempty"`

	Older string `json:"older,omitempty"`

	Path string `json:"path,omitempty"`

	Proto bool `json:"proto,omitempty"`

	ProtoVersion string `json:"protoVersion,omitempty"`

	Purged string `json:"purged,omitempty"`

	State bool `json:"state,omitempty"`

	StreamType string `json:"streamType,omitempty"`

	Tags []*LogStreamDescriptorTag `json:"tags,omitempty"`

	Terminated string `json:"terminated,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Archived") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *QueryRequest) MarshalJSON() ([]byte, error) {
	type noMethod QueryRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type QueryResponse struct {
	Next string `json:"next,omitempty"`

	Streams []*QueryResponseStream `json:"streams,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Next") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *QueryResponse) MarshalJSON() ([]byte, error) {
	type noMethod QueryResponse
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type QueryResponseStream struct {
	Descriptor *LogStreamDescriptor `json:"descriptor,omitempty"`

	DescriptorProto string `json:"descriptorProto,omitempty"`

	Path string `json:"path,omitempty"`

	State *LogStreamState `json:"state,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Descriptor") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *QueryResponseStream) MarshalJSON() ([]byte, error) {
	type noMethod QueryResponseStream
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// method id "logs.get":

type GetCall struct {
	s    *Service
	Path string
	opt_ map[string]interface{}
	ctx_ context.Context
}

// Get: Get log stream data.
func (s *Service) Get(Path string) *GetCall {
	c := &GetCall{s: s, opt_: make(map[string]interface{})}
	c.Path = Path
	return c
}

// Bytes sets the optional parameter "bytes":
func (c *GetCall) Bytes(bytes int64) *GetCall {
	c.opt_["bytes"] = bytes
	return c
}

// Count sets the optional parameter "count":
func (c *GetCall) Count(count int64) *GetCall {
	c.opt_["count"] = count
	return c
}

// Index sets the optional parameter "index":
func (c *GetCall) Index(index int64) *GetCall {
	c.opt_["index"] = index
	return c
}

// Newlines sets the optional parameter "newlines":
func (c *GetCall) Newlines(newlines bool) *GetCall {
	c.opt_["newlines"] = newlines
	return c
}

// Noncontiguous sets the optional parameter "noncontiguous":
func (c *GetCall) Noncontiguous(noncontiguous bool) *GetCall {
	c.opt_["noncontiguous"] = noncontiguous
	return c
}

// Proto sets the optional parameter "proto":
func (c *GetCall) Proto(proto bool) *GetCall {
	c.opt_["proto"] = proto
	return c
}

// State sets the optional parameter "state":
func (c *GetCall) State(state bool) *GetCall {
	c.opt_["state"] = state
	return c
}

// Tail sets the optional parameter "tail":
func (c *GetCall) Tail(tail bool) *GetCall {
	c.opt_["tail"] = tail
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *GetCall) Fields(s ...googleapi.Field) *GetCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *GetCall) IfNoneMatch(entityTag string) *GetCall {
	c.opt_["ifNoneMatch"] = entityTag
	return c
}

// Context sets the context to be used in this call's Do method.
// Any pending HTTP request will be aborted if the provided context
// is canceled.
func (c *GetCall) Context(ctx context.Context) *GetCall {
	c.ctx_ = ctx
	return c
}

func (c *GetCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["bytes"]; ok {
		params.Set("bytes", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["count"]; ok {
		params.Set("count", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["index"]; ok {
		params.Set("index", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["newlines"]; ok {
		params.Set("newlines", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["noncontiguous"]; ok {
		params.Set("noncontiguous", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["proto"]; ok {
		params.Set("proto", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["state"]; ok {
		params.Set("state", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["tail"]; ok {
		params.Set("tail", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "get/{Path}")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"Path": c.Path,
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

// Do executes the "logs.get" call.
// Exactly one of *GetResponse or error will be non-nil. Any non-2xx
// status code is an error. Response headers are in either
// *GetResponse.ServerResponse.Header or (if a response was returned at
// all) in error.(*googleapi.Error).Header. Use googleapi.IsNotModified
// to check whether the returned error was because
// http.StatusNotModified was returned.
func (c *GetCall) Do() (*GetResponse, error) {
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
	ret := &GetResponse{
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
	//   "description": "Get log stream data.",
	//   "httpMethod": "GET",
	//   "id": "logs.get",
	//   "parameterOrder": [
	//     "Path"
	//   ],
	//   "parameters": {
	//     "Path": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     },
	//     "bytes": {
	//       "format": "int32",
	//       "location": "query",
	//       "type": "integer"
	//     },
	//     "count": {
	//       "format": "int32",
	//       "location": "query",
	//       "type": "integer"
	//     },
	//     "index": {
	//       "format": "int64",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "newlines": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "noncontiguous": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "proto": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "state": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "tail": {
	//       "location": "query",
	//       "type": "boolean"
	//     }
	//   },
	//   "path": "get/{Path}",
	//   "response": {
	//     "$ref": "GetResponse",
	//     "parameterName": "resource"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "logs.query":

type QueryCall struct {
	s            *Service
	queryrequest *QueryRequest
	opt_         map[string]interface{}
	ctx_         context.Context
}

// Query: Query for log streams.
func (s *Service) Query(queryrequest *QueryRequest) *QueryCall {
	c := &QueryCall{s: s, opt_: make(map[string]interface{})}
	c.queryrequest = queryrequest
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *QueryCall) Fields(s ...googleapi.Field) *QueryCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

// Context sets the context to be used in this call's Do method.
// Any pending HTTP request will be aborted if the provided context
// is canceled.
func (c *QueryCall) Context(ctx context.Context) *QueryCall {
	c.ctx_ = ctx
	return c
}

func (c *QueryCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.queryrequest)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "query")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "logs.query" call.
// Exactly one of *QueryResponse or error will be non-nil. Any non-2xx
// status code is an error. Response headers are in either
// *QueryResponse.ServerResponse.Header or (if a response was returned
// at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *QueryCall) Do() (*QueryResponse, error) {
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
	ret := &QueryResponse{
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
	//   "description": "Query for log streams.",
	//   "httpMethod": "POST",
	//   "id": "logs.query",
	//   "path": "query",
	//   "request": {
	//     "$ref": "QueryRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "QueryResponse",
	//     "parameterName": "resource"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}
