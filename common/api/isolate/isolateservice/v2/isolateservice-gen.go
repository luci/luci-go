// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package isolateservice provides access to the .
//
// Usage example:
//
//   import "github.com/luci/luci-go/common/api/isolate/isolateservice/v2"
//   ...
//   isolateserviceService, err := isolateservice.New(oauthHttpClient)
package isolateservice // import "github.com/luci/luci-go/common/api/isolate/isolateservice/v2"

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

const apiId = "isolateservice:v2"
const apiName = "isolateservice"
const apiVersion = "v2"
const basePath = "http://localhost:8080/_ah/api/isolateservice/v2/"

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

// HandlersEndpointsV2Digest: ProtoRPC message containing digest
// information.
type HandlersEndpointsV2Digest struct {
	Digest string `json:"digest,omitempty"`

	IsIsolated bool `json:"is_isolated,omitempty"`

	Size int64 `json:"size,omitempty,string"`

	// ForceSendFields is a list of field names (e.g. "Digest") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *HandlersEndpointsV2Digest) MarshalJSON() ([]byte, error) {
	type noMethod HandlersEndpointsV2Digest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// HandlersEndpointsV2DigestCollection: Endpoints request type analogous
// to the existing JSON post body.
type HandlersEndpointsV2DigestCollection struct {
	// Items: ProtoRPC message containing digest information.
	Items []*HandlersEndpointsV2Digest `json:"items,omitempty"`

	Namespace string `json:"namespace,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Items") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *HandlersEndpointsV2DigestCollection) MarshalJSON() ([]byte, error) {
	type noMethod HandlersEndpointsV2DigestCollection
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// HandlersEndpointsV2FinalizeRequest: Request to validate upload of
// large Google storage entities.
type HandlersEndpointsV2FinalizeRequest struct {
	UploadTicket string `json:"upload_ticket,omitempty"`

	// ForceSendFields is a list of field names (e.g. "UploadTicket") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *HandlersEndpointsV2FinalizeRequest) MarshalJSON() ([]byte, error) {
	type noMethod HandlersEndpointsV2FinalizeRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// HandlersEndpointsV2PreuploadStatus: Endpoints response type for a
// single URL or pair of URLs.
type HandlersEndpointsV2PreuploadStatus struct {
	GsUploadUrl string `json:"gs_upload_url,omitempty"`

	Index int64 `json:"index,omitempty,string"`

	UploadTicket string `json:"upload_ticket,omitempty"`

	// ForceSendFields is a list of field names (e.g. "GsUploadUrl") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *HandlersEndpointsV2PreuploadStatus) MarshalJSON() ([]byte, error) {
	type noMethod HandlersEndpointsV2PreuploadStatus
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// HandlersEndpointsV2PushPing: Indicates whether data storage executed
// successfully.
type HandlersEndpointsV2PushPing struct {
	Ok bool `json:"ok,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Ok") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *HandlersEndpointsV2PushPing) MarshalJSON() ([]byte, error) {
	type noMethod HandlersEndpointsV2PushPing
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// HandlersEndpointsV2RetrieveRequest: Request to retrieve content from
// memcache, datastore, or GS.
type HandlersEndpointsV2RetrieveRequest struct {
	Digest string `json:"digest,omitempty"`

	Namespace string `json:"namespace,omitempty"`

	Offset int64 `json:"offset,omitempty,string"`

	// ForceSendFields is a list of field names (e.g. "Digest") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *HandlersEndpointsV2RetrieveRequest) MarshalJSON() ([]byte, error) {
	type noMethod HandlersEndpointsV2RetrieveRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// HandlersEndpointsV2RetrievedContent: Content retrieved from DB, or GS
// URL.
type HandlersEndpointsV2RetrievedContent struct {
	Content string `json:"content,omitempty"`

	Url string `json:"url,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Content") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *HandlersEndpointsV2RetrievedContent) MarshalJSON() ([]byte, error) {
	type noMethod HandlersEndpointsV2RetrievedContent
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// HandlersEndpointsV2ServerDetails: Reports the current API version.
type HandlersEndpointsV2ServerDetails struct {
	ServerVersion string `json:"server_version,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "ServerVersion") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *HandlersEndpointsV2ServerDetails) MarshalJSON() ([]byte, error) {
	type noMethod HandlersEndpointsV2ServerDetails
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// HandlersEndpointsV2StorageRequest: ProtoRPC message representing an
// entity to be added to the data store.
type HandlersEndpointsV2StorageRequest struct {
	Content string `json:"content,omitempty"`

	UploadTicket string `json:"upload_ticket,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Content") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *HandlersEndpointsV2StorageRequest) MarshalJSON() ([]byte, error) {
	type noMethod HandlersEndpointsV2StorageRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// HandlersEndpointsV2UrlCollection: Endpoints response type analogous
// to existing JSON response.
type HandlersEndpointsV2UrlCollection struct {
	// Items: Endpoints response type for a single URL or pair of URLs.
	Items []*HandlersEndpointsV2PreuploadStatus `json:"items,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Items") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *HandlersEndpointsV2UrlCollection) MarshalJSON() ([]byte, error) {
	type noMethod HandlersEndpointsV2UrlCollection
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// method id "isolateservice.finalize_gs_upload":

type FinalizeGsUploadCall struct {
	s                                  *Service
	handlersendpointsv2finalizerequest *HandlersEndpointsV2FinalizeRequest
	urlParams_                         gensupport.URLParams
	ctx_                               context.Context
}

// FinalizeGsUpload: Informs client that large entities have been
// uploaded to GCS.
func (s *Service) FinalizeGsUpload(handlersendpointsv2finalizerequest *HandlersEndpointsV2FinalizeRequest) *FinalizeGsUploadCall {
	c := &FinalizeGsUploadCall{s: s, urlParams_: make(gensupport.URLParams)}
	c.handlersendpointsv2finalizerequest = handlersendpointsv2finalizerequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *FinalizeGsUploadCall) Fields(s ...googleapi.Field) *FinalizeGsUploadCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *FinalizeGsUploadCall) Context(ctx context.Context) *FinalizeGsUploadCall {
	c.ctx_ = ctx
	return c
}

func (c *FinalizeGsUploadCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.handlersendpointsv2finalizerequest)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "finalize_gs_upload")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "isolateservice.finalize_gs_upload" call.
// Exactly one of *HandlersEndpointsV2PushPing or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *HandlersEndpointsV2PushPing.ServerResponse.Header or (if a response
// was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *FinalizeGsUploadCall) Do() (*HandlersEndpointsV2PushPing, error) {
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
	ret := &HandlersEndpointsV2PushPing{
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
	//   "description": "Informs client that large entities have been uploaded to GCS.",
	//   "httpMethod": "POST",
	//   "id": "isolateservice.finalize_gs_upload",
	//   "path": "finalize_gs_upload",
	//   "request": {
	//     "$ref": "HandlersEndpointsV2FinalizeRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "HandlersEndpointsV2PushPing"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "isolateservice.preupload":

type PreuploadCall struct {
	s                                   *Service
	handlersendpointsv2digestcollection *HandlersEndpointsV2DigestCollection
	urlParams_                          gensupport.URLParams
	ctx_                                context.Context
}

// Preupload: Checks for entry's existence and generates upload URLs.
// Arguments: request: the DigestCollection to be posted Returns: the
// UrlCollection corresponding to the uploaded digests The response list
// is commensurate to the request's; each UrlMessage has * if an entry
// is missing: two URLs: the URL to upload a file to and the URL to call
// when the upload is done (can be null). * if the entry is already
// present: null URLs (''). UrlCollection([ UrlMessage( upload_url = ""
// finalize_url = "" ) UrlMessage( upload_url = '') ... ])
func (s *Service) Preupload(handlersendpointsv2digestcollection *HandlersEndpointsV2DigestCollection) *PreuploadCall {
	c := &PreuploadCall{s: s, urlParams_: make(gensupport.URLParams)}
	c.handlersendpointsv2digestcollection = handlersendpointsv2digestcollection
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *PreuploadCall) Fields(s ...googleapi.Field) *PreuploadCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *PreuploadCall) Context(ctx context.Context) *PreuploadCall {
	c.ctx_ = ctx
	return c
}

func (c *PreuploadCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.handlersendpointsv2digestcollection)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "preupload")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "isolateservice.preupload" call.
// Exactly one of *HandlersEndpointsV2UrlCollection or error will be
// non-nil. Any non-2xx status code is an error. Response headers are in
// either *HandlersEndpointsV2UrlCollection.ServerResponse.Header or (if
// a response was returned at all) in error.(*googleapi.Error).Header.
// Use googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *PreuploadCall) Do() (*HandlersEndpointsV2UrlCollection, error) {
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
	ret := &HandlersEndpointsV2UrlCollection{
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
	//   "description": "Checks for entry's existence and generates upload URLs. Arguments: request: the DigestCollection to be posted Returns: the UrlCollection corresponding to the uploaded digests The response list is commensurate to the request's; each UrlMessage has * if an entry is missing: two URLs: the URL to upload a file to and the URL to call when the upload is done (can be null). * if the entry is already present: null URLs (''). UrlCollection([ UrlMessage( upload_url = \"\" finalize_url = \"\" ) UrlMessage( upload_url = '') ... ])",
	//   "httpMethod": "POST",
	//   "id": "isolateservice.preupload",
	//   "path": "preupload",
	//   "request": {
	//     "$ref": "HandlersEndpointsV2DigestCollection",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "HandlersEndpointsV2UrlCollection"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "isolateservice.retrieve":

type RetrieveCall struct {
	s                                  *Service
	handlersendpointsv2retrieverequest *HandlersEndpointsV2RetrieveRequest
	urlParams_                         gensupport.URLParams
	ctx_                               context.Context
}

// Retrieve: Retrieves content from a storage location.
func (s *Service) Retrieve(handlersendpointsv2retrieverequest *HandlersEndpointsV2RetrieveRequest) *RetrieveCall {
	c := &RetrieveCall{s: s, urlParams_: make(gensupport.URLParams)}
	c.handlersendpointsv2retrieverequest = handlersendpointsv2retrieverequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *RetrieveCall) Fields(s ...googleapi.Field) *RetrieveCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *RetrieveCall) Context(ctx context.Context) *RetrieveCall {
	c.ctx_ = ctx
	return c
}

func (c *RetrieveCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.handlersendpointsv2retrieverequest)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "retrieve")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "isolateservice.retrieve" call.
// Exactly one of *HandlersEndpointsV2RetrievedContent or error will be
// non-nil. Any non-2xx status code is an error. Response headers are in
// either *HandlersEndpointsV2RetrievedContent.ServerResponse.Header or
// (if a response was returned at all) in
// error.(*googleapi.Error).Header. Use googleapi.IsNotModified to check
// whether the returned error was because http.StatusNotModified was
// returned.
func (c *RetrieveCall) Do() (*HandlersEndpointsV2RetrievedContent, error) {
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
	ret := &HandlersEndpointsV2RetrievedContent{
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
	//   "description": "Retrieves content from a storage location.",
	//   "httpMethod": "POST",
	//   "id": "isolateservice.retrieve",
	//   "path": "retrieve",
	//   "request": {
	//     "$ref": "HandlersEndpointsV2RetrieveRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "HandlersEndpointsV2RetrievedContent"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "isolateservice.server_details":

type ServerDetailsCall struct {
	s          *Service
	urlParams_ gensupport.URLParams
	ctx_       context.Context
}

// ServerDetails:
func (s *Service) ServerDetails() *ServerDetailsCall {
	c := &ServerDetailsCall{s: s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ServerDetailsCall) Fields(s ...googleapi.Field) *ServerDetailsCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ServerDetailsCall) Context(ctx context.Context) *ServerDetailsCall {
	c.ctx_ = ctx
	return c
}

func (c *ServerDetailsCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "server_details")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "isolateservice.server_details" call.
// Exactly one of *HandlersEndpointsV2ServerDetails or error will be
// non-nil. Any non-2xx status code is an error. Response headers are in
// either *HandlersEndpointsV2ServerDetails.ServerResponse.Header or (if
// a response was returned at all) in error.(*googleapi.Error).Header.
// Use googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ServerDetailsCall) Do() (*HandlersEndpointsV2ServerDetails, error) {
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
	ret := &HandlersEndpointsV2ServerDetails{
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
	//   "httpMethod": "POST",
	//   "id": "isolateservice.server_details",
	//   "path": "server_details",
	//   "response": {
	//     "$ref": "HandlersEndpointsV2ServerDetails"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "isolateservice.store_inline":

type StoreInlineCall struct {
	s                                 *Service
	handlersendpointsv2storagerequest *HandlersEndpointsV2StorageRequest
	urlParams_                        gensupport.URLParams
	ctx_                              context.Context
}

// StoreInline: Stores relatively small entities in the datastore.
func (s *Service) StoreInline(handlersendpointsv2storagerequest *HandlersEndpointsV2StorageRequest) *StoreInlineCall {
	c := &StoreInlineCall{s: s, urlParams_: make(gensupport.URLParams)}
	c.handlersendpointsv2storagerequest = handlersendpointsv2storagerequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *StoreInlineCall) Fields(s ...googleapi.Field) *StoreInlineCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *StoreInlineCall) Context(ctx context.Context) *StoreInlineCall {
	c.ctx_ = ctx
	return c
}

func (c *StoreInlineCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.handlersendpointsv2storagerequest)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "store_inline")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "isolateservice.store_inline" call.
// Exactly one of *HandlersEndpointsV2PushPing or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *HandlersEndpointsV2PushPing.ServerResponse.Header or (if a response
// was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *StoreInlineCall) Do() (*HandlersEndpointsV2PushPing, error) {
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
	ret := &HandlersEndpointsV2PushPing{
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
	//   "description": "Stores relatively small entities in the datastore.",
	//   "httpMethod": "POST",
	//   "id": "isolateservice.store_inline",
	//   "path": "store_inline",
	//   "request": {
	//     "$ref": "HandlersEndpointsV2StorageRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "HandlersEndpointsV2PushPing"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}
