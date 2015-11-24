// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dm provides access to the .
//
// Usage example:
//
//   import "github.com/luci/luci-go/common/api/dungeon_master/dm/v1"
//   ...
//   dmService, err := dm.New(oauthHttpClient)
package dm

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

const apiId = "dm:v1"
const apiName = "dm"
const apiVersion = "v1"
const basePath = "https://luci-dm.appspot.com/_ah/api/dm/v1/"

func New(client *http.Client) (*Service, error) {
	if client == nil {
		return nil, errors.New("client is nil")
	}
	s := &Service{client: client, BasePath: basePath}
	s.Executions = NewExecutionsService(s)
	s.Quests = NewQuestsService(s)
	return s, nil
}

type Service struct {
	client    *http.Client
	BasePath  string // API endpoint base URL
	UserAgent string // optional additional User-Agent fragment

	Executions *ExecutionsService

	Quests *QuestsService
}

func (s *Service) userAgent() string {
	if s.UserAgent == "" {
		return googleapi.UserAgent
	}
	return googleapi.UserAgent + " " + s.UserAgent
}

func NewExecutionsService(s *Service) *ExecutionsService {
	rs := &ExecutionsService{s: s}
	return rs
}

type ExecutionsService struct {
	s *Service
}

func NewQuestsService(s *Service) *QuestsService {
	rs := &QuestsService{s: s}
	rs.Attempts = NewQuestsAttemptsService(s)
	return rs
}

type QuestsService struct {
	s *Service

	Attempts *QuestsAttemptsService
}

func NewQuestsAttemptsService(s *Service) *QuestsAttemptsService {
	rs := &QuestsAttemptsService{s: s}
	return rs
}

type QuestsAttemptsService struct {
	s *Service
}

type AddDepsReq struct {
	AttemptNum int64 `json:"AttemptNum,omitempty"`

	ExecutionKey string `json:"ExecutionKey,omitempty"`

	QuestID string `json:"QuestID,omitempty"`

	To []*AttemptID `json:"To,omitempty"`

	// ForceSendFields is a list of field names (e.g. "AttemptNum") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *AddDepsReq) MarshalJSON() ([]byte, error) {
	type noMethod AddDepsReq
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type AddDepsRsp struct {
	ShouldHalt bool `json:"ShouldHalt,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "ShouldHalt") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *AddDepsRsp) MarshalJSON() ([]byte, error) {
	type noMethod AddDepsRsp
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type Attempt struct {
	// Expiration: The time at which this result will become Expired. Only
	// set for attempts in the Finished or Expired states
	Expiration string `json:"Expiration,omitempty"`

	ID *AttemptID `json:"ID,omitempty"`

	// NumExecutions: The number of executions this Attempt has had.
	NumExecutions int64 `json:"NumExecutions,omitempty"`

	// NumWaitingDeps: The number of dependencies that this Attempt is
	// blocked on. Only valid for attempts in the AddingDeps or Blocked
	// states
	NumWaitingDeps int64 `json:"NumWaitingDeps,omitempty"`

	// State: The current state of this Attempt
	State int64 `json:"State,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Expiration") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *Attempt) MarshalJSON() ([]byte, error) {
	type noMethod Attempt
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type AttemptID struct {
	AttemptNum int64 `json:"AttemptNum,omitempty"`

	QuestID string `json:"QuestID,omitempty"`

	// ForceSendFields is a list of field names (e.g. "AttemptNum") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *AttemptID) MarshalJSON() ([]byte, error) {
	type noMethod AttemptID
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type AttemptResult struct {
	// Data: The JSON result for this Attempt
	Data string `json:"Data,omitempty"`

	ID *AttemptID `json:"ID,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Data") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *AttemptResult) MarshalJSON() ([]byte, error) {
	type noMethod AttemptResult
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type ClaimExecutionRsp struct {
	Attempt *Attempt `json:"Attempt,omitempty"`

	Execution *ExecutionInfo `json:"Execution,omitempty"`

	NoneAvailable bool `json:"NoneAvailable,omitempty"`

	Quest *Quest `json:"Quest,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Attempt") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *ClaimExecutionRsp) MarshalJSON() ([]byte, error) {
	type noMethod ClaimExecutionRsp
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type Data struct {
	AttemptResults []*AttemptResult `json:"AttemptResults,omitempty"`

	Attempts []*Attempt `json:"Attempts,omitempty"`

	BackDeps []*DepsFromAttempt `json:"BackDeps,omitempty"`

	Distributors []*Distributor `json:"Distributors,omitempty"`

	Executions []*ExecutionsForAttempt `json:"Executions,omitempty"`

	FwdDeps []*DepsFromAttempt `json:"FwdDeps,omitempty"`

	HadErrors bool `json:"HadErrors,omitempty"`

	// More: Indicates that the query has more results.
	More bool `json:"More,omitempty"`

	Quests []*Quest `json:"Quests,omitempty"`

	Timeout bool `json:"Timeout,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "AttemptResults") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *Data) MarshalJSON() ([]byte, error) {
	type noMethod Data
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type DepsFromAttempt struct {
	// From: The Quest that this dependency points from
	From *AttemptID `json:"From,omitempty"`

	To []*QuestAttempts `json:"To,omitempty"`

	// ForceSendFields is a list of field names (e.g. "From") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *DepsFromAttempt) MarshalJSON() ([]byte, error) {
	type noMethod DepsFromAttempt
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type Distributor struct {
	Name string `json:"Name,omitempty"`

	URL string `json:"URL,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Name") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *Distributor) MarshalJSON() ([]byte, error) {
	type noMethod Distributor
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type EnsureAttemptReq struct {
	AttemptNum int64 `json:"AttemptNum,omitempty"`

	QuestID string `json:"QuestID,omitempty"`

	// ForceSendFields is a list of field names (e.g. "AttemptNum") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *EnsureAttemptReq) MarshalJSON() ([]byte, error) {
	type noMethod EnsureAttemptReq
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type EnsureQuestsReq struct {
	QuestDescriptors []*QuestDescriptor `json:"QuestDescriptors,omitempty"`

	// ForceSendFields is a list of field names (e.g. "QuestDescriptors") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *EnsureQuestsReq) MarshalJSON() ([]byte, error) {
	type noMethod EnsureQuestsReq
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type EnsureQuestsRsp struct {
	QuestIDs []string `json:"QuestIDs,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "QuestIDs") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *EnsureQuestsRsp) MarshalJSON() ([]byte, error) {
	type noMethod EnsureQuestsRsp
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type ExecutionInfo struct {
	// DistributorToken: The distributor Token that this execution has.
	DistributorToken string `json:"DistributorToken,omitempty"`

	// ExecutionID: The Execution ID
	ExecutionID int64 `json:"ExecutionID,omitempty"`

	// ForceSendFields is a list of field names (e.g. "DistributorToken") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *ExecutionInfo) MarshalJSON() ([]byte, error) {
	type noMethod ExecutionInfo
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type ExecutionsForAttempt struct {
	Attempt *AttemptID `json:"Attempt,omitempty"`

	Executions []*ExecutionInfo `json:"Executions,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Attempt") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *ExecutionsForAttempt) MarshalJSON() ([]byte, error) {
	type noMethod ExecutionsForAttempt
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type FinishAttemptReq struct {
	AttemptNum int64 `json:"AttemptNum,omitempty"`

	ExecutionKey string `json:"ExecutionKey,omitempty"`

	QuestID string `json:"QuestID,omitempty"`

	Result string `json:"Result,omitempty"`

	ResultExpiration string `json:"ResultExpiration,omitempty"`

	// ForceSendFields is a list of field names (e.g. "AttemptNum") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *FinishAttemptReq) MarshalJSON() ([]byte, error) {
	type noMethod FinishAttemptReq
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type Quest struct {
	// Created: The time that this quest was created
	Created string `json:"Created,omitempty"`

	// Distributor: The Distributor to use for this Quest
	Distributor string `json:"Distributor,omitempty"`

	// ID: The Quest ID
	ID string `json:"ID,omitempty"`

	// Payload: The Quest JSON payload for the distributor
	Payload string `json:"Payload,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Created") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *Quest) MarshalJSON() ([]byte, error) {
	type noMethod Quest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type QuestAttempts struct {
	// Attempts: The Attempt(s) that this dependency points to
	Attempts []int64 `json:"Attempts,omitempty"`

	// QuestID: The Quest that this dependency points to
	QuestID string `json:"QuestID,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Attempts") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *QuestAttempts) MarshalJSON() ([]byte, error) {
	type noMethod QuestAttempts
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

type QuestDescriptor struct {
	Distributor string `json:"Distributor,omitempty"`

	Payload string `json:"Payload,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Distributor") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *QuestDescriptor) MarshalJSON() ([]byte, error) {
	type noMethod QuestDescriptor
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// method id "dm.executions.claim":

type ExecutionsClaimCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
}

// Claim: TEMP: Claim an execution id/key
func (r *ExecutionsService) Claim() *ExecutionsClaimCall {
	c := &ExecutionsClaimCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ExecutionsClaimCall) Fields(s ...googleapi.Field) *ExecutionsClaimCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ExecutionsClaimCall) IfNoneMatch(entityTag string) *ExecutionsClaimCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ExecutionsClaimCall) Context(ctx context.Context) *ExecutionsClaimCall {
	c.ctx_ = ctx
	return c
}

func (c *ExecutionsClaimCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "executions/claim")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		req.Header.Set("If-None-Match", c.ifNoneMatch_)
	}
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "dm.executions.claim" call.
// Exactly one of *ClaimExecutionRsp or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *ClaimExecutionRsp.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ExecutionsClaimCall) Do() (*ClaimExecutionRsp, error) {
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
	ret := &ClaimExecutionRsp{
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
	//   "description": "TEMP: Claim an execution id/key",
	//   "httpMethod": "GET",
	//   "id": "dm.executions.claim",
	//   "path": "executions/claim",
	//   "response": {
	//     "$ref": "ClaimExecutionRsp"
	//   }
	// }

}

// method id "dm.quests.insert":

type QuestsInsertCall struct {
	s               *Service
	ensurequestsreq *EnsureQuestsReq
	urlParams_      gensupport.URLParams
	ctx_            context.Context
}

// Insert: Ensures the existence of one or more Quests
func (r *QuestsService) Insert(ensurequestsreq *EnsureQuestsReq) *QuestsInsertCall {
	c := &QuestsInsertCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.ensurequestsreq = ensurequestsreq
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *QuestsInsertCall) Fields(s ...googleapi.Field) *QuestsInsertCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *QuestsInsertCall) Context(ctx context.Context) *QuestsInsertCall {
	c.ctx_ = ctx
	return c
}

func (c *QuestsInsertCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.ensurequestsreq)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "quests")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("PUT", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "dm.quests.insert" call.
// Exactly one of *EnsureQuestsRsp or error will be non-nil. Any non-2xx
// status code is an error. Response headers are in either
// *EnsureQuestsRsp.ServerResponse.Header or (if a response was returned
// at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *QuestsInsertCall) Do() (*EnsureQuestsRsp, error) {
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
	ret := &EnsureQuestsRsp{
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
	//   "description": "Ensures the existence of one or more Quests",
	//   "httpMethod": "PUT",
	//   "id": "dm.quests.insert",
	//   "path": "quests",
	//   "request": {
	//     "$ref": "EnsureQuestsReq",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "EnsureQuestsRsp"
	//   }
	// }

}

// method id "dm.quests.list":

type QuestsListCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
}

// List: Recursively view all quests
func (r *QuestsService) List() *QuestsListCall {
	c := &QuestsListCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// From sets the optional parameter "From":
func (c *QuestsListCall) From(From string) *QuestsListCall {
	c.urlParams_.Set("From", From)
	return c
}

// To sets the optional parameter "To":
func (c *QuestsListCall) To(To string) *QuestsListCall {
	c.urlParams_.Set("To", To)
	return c
}

// WithAttempts sets the optional parameter "WithAttempts":
func (c *QuestsListCall) WithAttempts(WithAttempts bool) *QuestsListCall {
	c.urlParams_.Set("WithAttempts", fmt.Sprint(WithAttempts))
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *QuestsListCall) Fields(s ...googleapi.Field) *QuestsListCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *QuestsListCall) IfNoneMatch(entityTag string) *QuestsListCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *QuestsListCall) Context(ctx context.Context) *QuestsListCall {
	c.ctx_ = ctx
	return c
}

func (c *QuestsListCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "quests")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		req.Header.Set("If-None-Match", c.ifNoneMatch_)
	}
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "dm.quests.list" call.
// Exactly one of *Data or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *Data.ServerResponse.Header or (if a response was returned at all) in
// error.(*googleapi.Error).Header. Use googleapi.IsNotModified to check
// whether the returned error was because http.StatusNotModified was
// returned.
func (c *QuestsListCall) Do() (*Data, error) {
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
	ret := &Data{
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
	//   "description": "Recursively view all quests",
	//   "httpMethod": "GET",
	//   "id": "dm.quests.list",
	//   "parameters": {
	//     "From": {
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "To": {
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "WithAttempts": {
	//       "location": "query",
	//       "type": "boolean"
	//     }
	//   },
	//   "path": "quests",
	//   "response": {
	//     "$ref": "Data"
	//   }
	// }

}

// method id "dm.quests.attempts.dependencies":

type QuestsAttemptsDependenciesCall struct {
	s          *Service
	QuestID    string
	AttemptNum int64
	adddepsreq *AddDepsReq
	urlParams_ gensupport.URLParams
	ctx_       context.Context
}

// Dependencies: Allows an Execution to add additional dependencies
func (r *QuestsAttemptsService) Dependencies(QuestID string, AttemptNum int64, adddepsreq *AddDepsReq) *QuestsAttemptsDependenciesCall {
	c := &QuestsAttemptsDependenciesCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.QuestID = QuestID
	c.AttemptNum = AttemptNum
	c.adddepsreq = adddepsreq
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *QuestsAttemptsDependenciesCall) Fields(s ...googleapi.Field) *QuestsAttemptsDependenciesCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *QuestsAttemptsDependenciesCall) Context(ctx context.Context) *QuestsAttemptsDependenciesCall {
	c.ctx_ = ctx
	return c
}

func (c *QuestsAttemptsDependenciesCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.adddepsreq)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "quests/{QuestID}/attempts/{AttemptNum}/dependencies")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("PUT", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"QuestID":    c.QuestID,
		"AttemptNum": strconv.FormatInt(c.AttemptNum, 10),
	})
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "dm.quests.attempts.dependencies" call.
// Exactly one of *AddDepsRsp or error will be non-nil. Any non-2xx
// status code is an error. Response headers are in either
// *AddDepsRsp.ServerResponse.Header or (if a response was returned at
// all) in error.(*googleapi.Error).Header. Use googleapi.IsNotModified
// to check whether the returned error was because
// http.StatusNotModified was returned.
func (c *QuestsAttemptsDependenciesCall) Do() (*AddDepsRsp, error) {
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
	ret := &AddDepsRsp{
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
	//   "description": "Allows an Execution to add additional dependencies",
	//   "httpMethod": "PUT",
	//   "id": "dm.quests.attempts.dependencies",
	//   "parameterOrder": [
	//     "QuestID",
	//     "AttemptNum"
	//   ],
	//   "parameters": {
	//     "AttemptNum": {
	//       "format": "uint32",
	//       "location": "path",
	//       "required": true,
	//       "type": "integer"
	//     },
	//     "QuestID": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "quests/{QuestID}/attempts/{AttemptNum}/dependencies",
	//   "request": {
	//     "$ref": "AddDepsReq",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "AddDepsRsp"
	//   }
	// }

}

// method id "dm.quests.attempts.finish":

type QuestsAttemptsFinishCall struct {
	s                *Service
	QuestID          string
	AttemptNum       int64
	finishattemptreq *FinishAttemptReq
	urlParams_       gensupport.URLParams
	ctx_             context.Context
}

// Finish: Sets the result of an attempt
func (r *QuestsAttemptsService) Finish(QuestID string, AttemptNum int64, finishattemptreq *FinishAttemptReq) *QuestsAttemptsFinishCall {
	c := &QuestsAttemptsFinishCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.QuestID = QuestID
	c.AttemptNum = AttemptNum
	c.finishattemptreq = finishattemptreq
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *QuestsAttemptsFinishCall) Fields(s ...googleapi.Field) *QuestsAttemptsFinishCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *QuestsAttemptsFinishCall) Context(ctx context.Context) *QuestsAttemptsFinishCall {
	c.ctx_ = ctx
	return c
}

func (c *QuestsAttemptsFinishCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.finishattemptreq)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "quests/{QuestID}/attempts/{AttemptNum}/result")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"QuestID":    c.QuestID,
		"AttemptNum": strconv.FormatInt(c.AttemptNum, 10),
	})
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "dm.quests.attempts.finish" call.
func (c *QuestsAttemptsFinishCall) Do() error {
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
	//   "description": "Sets the result of an attempt",
	//   "httpMethod": "POST",
	//   "id": "dm.quests.attempts.finish",
	//   "parameterOrder": [
	//     "QuestID",
	//     "AttemptNum"
	//   ],
	//   "parameters": {
	//     "AttemptNum": {
	//       "format": "uint32",
	//       "location": "path",
	//       "required": true,
	//       "type": "integer"
	//     },
	//     "QuestID": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "quests/{QuestID}/attempts/{AttemptNum}/result",
	//   "request": {
	//     "$ref": "FinishAttemptReq",
	//     "parameterName": "resource"
	//   }
	// }

}

// method id "dm.quests.attempts.get":

type QuestsAttemptsGetCall struct {
	s            *Service
	QuestID      string
	AttemptNum   int64
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
}

// Get: Get the status and dependencies of an Attempt
func (r *QuestsAttemptsService) Get(QuestID string, AttemptNum int64) *QuestsAttemptsGetCall {
	c := &QuestsAttemptsGetCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.QuestID = QuestID
	c.AttemptNum = AttemptNum
	return c
}

// OptionsBackDeps sets the optional parameter "Options.BackDeps":
func (c *QuestsAttemptsGetCall) OptionsBackDeps(OptionsBackDeps bool) *QuestsAttemptsGetCall {
	c.urlParams_.Set("Options.BackDeps", fmt.Sprint(OptionsBackDeps))
	return c
}

// OptionsDFS sets the optional parameter "Options.DFS":
func (c *QuestsAttemptsGetCall) OptionsDFS(OptionsDFS bool) *QuestsAttemptsGetCall {
	c.urlParams_.Set("Options.DFS", fmt.Sprint(OptionsDFS))
	return c
}

// OptionsFwdDeps sets the optional parameter "Options.FwdDeps":
func (c *QuestsAttemptsGetCall) OptionsFwdDeps(OptionsFwdDeps bool) *QuestsAttemptsGetCall {
	c.urlParams_.Set("Options.FwdDeps", fmt.Sprint(OptionsFwdDeps))
	return c
}

// OptionsMaxDepth sets the optional parameter "Options.MaxDepth":
func (c *QuestsAttemptsGetCall) OptionsMaxDepth(OptionsMaxDepth int64) *QuestsAttemptsGetCall {
	c.urlParams_.Set("Options.MaxDepth", fmt.Sprint(OptionsMaxDepth))
	return c
}

// OptionsResult sets the optional parameter "Options.Result":
func (c *QuestsAttemptsGetCall) OptionsResult(OptionsResult bool) *QuestsAttemptsGetCall {
	c.urlParams_.Set("Options.Result", fmt.Sprint(OptionsResult))
	return c
}

// TimeoutMs sets the optional parameter "TimeoutMs":
func (c *QuestsAttemptsGetCall) TimeoutMs(TimeoutMs int64) *QuestsAttemptsGetCall {
	c.urlParams_.Set("TimeoutMs", fmt.Sprint(TimeoutMs))
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *QuestsAttemptsGetCall) Fields(s ...googleapi.Field) *QuestsAttemptsGetCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *QuestsAttemptsGetCall) IfNoneMatch(entityTag string) *QuestsAttemptsGetCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *QuestsAttemptsGetCall) Context(ctx context.Context) *QuestsAttemptsGetCall {
	c.ctx_ = ctx
	return c
}

func (c *QuestsAttemptsGetCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "quests/{QuestID}/attempts/{AttemptNum}")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"QuestID":    c.QuestID,
		"AttemptNum": strconv.FormatInt(c.AttemptNum, 10),
	})
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		req.Header.Set("If-None-Match", c.ifNoneMatch_)
	}
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "dm.quests.attempts.get" call.
// Exactly one of *Data or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *Data.ServerResponse.Header or (if a response was returned at all) in
// error.(*googleapi.Error).Header. Use googleapi.IsNotModified to check
// whether the returned error was because http.StatusNotModified was
// returned.
func (c *QuestsAttemptsGetCall) Do() (*Data, error) {
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
	ret := &Data{
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
	//   "description": "Get the status and dependencies of an Attempt",
	//   "httpMethod": "GET",
	//   "id": "dm.quests.attempts.get",
	//   "parameterOrder": [
	//     "QuestID",
	//     "AttemptNum"
	//   ],
	//   "parameters": {
	//     "AttemptNum": {
	//       "format": "uint32",
	//       "location": "path",
	//       "required": true,
	//       "type": "integer"
	//     },
	//     "Options.BackDeps": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "Options.DFS": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "Options.FwdDeps": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "Options.MaxDepth": {
	//       "format": "uint32",
	//       "location": "query",
	//       "type": "integer"
	//     },
	//     "Options.Result": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "QuestID": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     },
	//     "TimeoutMs": {
	//       "format": "uint32",
	//       "location": "query",
	//       "type": "integer"
	//     }
	//   },
	//   "path": "quests/{QuestID}/attempts/{AttemptNum}",
	//   "response": {
	//     "$ref": "Data"
	//   }
	// }

}

// method id "dm.quests.attempts.insert":

type QuestsAttemptsInsertCall struct {
	s                *Service
	QuestID          string
	AttemptNum       int64
	ensureattemptreq *EnsureAttemptReq
	urlParams_       gensupport.URLParams
	ctx_             context.Context
}

// Insert: Ensures the existence of an attempt
func (r *QuestsAttemptsService) Insert(QuestID string, AttemptNum int64, ensureattemptreq *EnsureAttemptReq) *QuestsAttemptsInsertCall {
	c := &QuestsAttemptsInsertCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.QuestID = QuestID
	c.AttemptNum = AttemptNum
	c.ensureattemptreq = ensureattemptreq
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *QuestsAttemptsInsertCall) Fields(s ...googleapi.Field) *QuestsAttemptsInsertCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *QuestsAttemptsInsertCall) Context(ctx context.Context) *QuestsAttemptsInsertCall {
	c.ctx_ = ctx
	return c
}

func (c *QuestsAttemptsInsertCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.ensureattemptreq)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "quests/{QuestID}/attempts/{AttemptNum}")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("PUT", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"QuestID":    c.QuestID,
		"AttemptNum": strconv.FormatInt(c.AttemptNum, 10),
	})
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "dm.quests.attempts.insert" call.
func (c *QuestsAttemptsInsertCall) Do() error {
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
	//   "description": "Ensures the existence of an attempt",
	//   "httpMethod": "PUT",
	//   "id": "dm.quests.attempts.insert",
	//   "parameterOrder": [
	//     "QuestID",
	//     "AttemptNum"
	//   ],
	//   "parameters": {
	//     "AttemptNum": {
	//       "format": "uint32",
	//       "location": "path",
	//       "required": true,
	//       "type": "integer"
	//     },
	//     "QuestID": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "quests/{QuestID}/attempts/{AttemptNum}",
	//   "request": {
	//     "$ref": "EnsureAttemptReq",
	//     "parameterName": "resource"
	//   }
	// }

}

// method id "dm.quests.attempts.list":

type QuestsAttemptsListCall struct {
	s            *Service
	QuestID      string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
}

// List: Lists the attempts of a Quest
func (r *QuestsAttemptsService) List(QuestID string) *QuestsAttemptsListCall {
	c := &QuestsAttemptsListCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.QuestID = QuestID
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *QuestsAttemptsListCall) Fields(s ...googleapi.Field) *QuestsAttemptsListCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *QuestsAttemptsListCall) IfNoneMatch(entityTag string) *QuestsAttemptsListCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *QuestsAttemptsListCall) Context(ctx context.Context) *QuestsAttemptsListCall {
	c.ctx_ = ctx
	return c
}

func (c *QuestsAttemptsListCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "quests/{QuestID}/attempts")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"QuestID": c.QuestID,
	})
	req.Header.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		req.Header.Set("If-None-Match", c.ifNoneMatch_)
	}
	if c.ctx_ != nil {
		return ctxhttp.Do(c.ctx_, c.s.client, req)
	}
	return c.s.client.Do(req)
}

// Do executes the "dm.quests.attempts.list" call.
// Exactly one of *Data or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *Data.ServerResponse.Header or (if a response was returned at all) in
// error.(*googleapi.Error).Header. Use googleapi.IsNotModified to check
// whether the returned error was because http.StatusNotModified was
// returned.
func (c *QuestsAttemptsListCall) Do() (*Data, error) {
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
	ret := &Data{
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
	//   "description": "Lists the attempts of a Quest",
	//   "httpMethod": "GET",
	//   "id": "dm.quests.attempts.list",
	//   "parameterOrder": [
	//     "QuestID"
	//   ],
	//   "parameters": {
	//     "QuestID": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "quests/{QuestID}/attempts",
	//   "response": {
	//     "$ref": "Data"
	//   }
	// }

}
