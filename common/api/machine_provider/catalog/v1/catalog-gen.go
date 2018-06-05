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

// Package catalog provides access to the .
//
// Usage example:
//
//   import "go.chromium.org/luci/common/api/machine_provider/catalog/v1"
//   ...
//   catalogService, err := catalog.New(oauthHttpClient)
package catalog // import "go.chromium.org/luci/common/api/machine_provider/catalog/v1"

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

const apiId = "catalog:v1"
const apiName = "catalog"
const apiVersion = "v1"
const basePath = "http://localhost:8080/_ah/api/catalog/v1"

// OAuth2 scopes used by this API.
const (
	// https://www.googleapis.com/auth/userinfo.email
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

// ComponentsMachineProviderDimensionsDimensions: Represents the
// dimensions of a machine.
type ComponentsMachineProviderDimensionsDimensions struct {
	// Possible values:
	//   "DUMMY"
	//   "GCE"
	//   "VSPHERE"
	Backend string `json:"backend,omitempty"`

	DiskGb int64 `json:"disk_gb,omitempty,string"`

	Hostname string `json:"hostname,omitempty"`

	// Possible values:
	//   "DEBIAN"
	//   "UBUNTU"
	LinuxFlavor string `json:"linux_flavor,omitempty"`

	MemoryGb float64 `json:"memory_gb,omitempty"`

	NumCpus int64 `json:"num_cpus,omitempty,string"`

	// Possible values:
	//   "LINUX"
	//   "OSX"
	//   "WINDOWS"
	OsFamily string `json:"os_family,omitempty"`

	OsVersion string `json:"os_version,omitempty"`

	Project string `json:"project,omitempty"`

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

func (s *ComponentsMachineProviderDimensionsDimensions) MarshalJSON() ([]byte, error) {
	type NoMethod ComponentsMachineProviderDimensionsDimensions
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

func (s *ComponentsMachineProviderDimensionsDimensions) UnmarshalJSON(data []byte) error {
	type NoMethod ComponentsMachineProviderDimensionsDimensions
	var s1 struct {
		MemoryGb gensupport.JSONFloat64 `json:"memory_gb"`
		*NoMethod
	}
	s1.NoMethod = (*NoMethod)(s)
	if err := json.Unmarshal(data, &s1); err != nil {
		return err
	}
	s.MemoryGb = float64(s1.MemoryGb)
	return nil
}

// ComponentsMachineProviderPoliciesKeyValuePair: Represents a key-value
// pair.
type ComponentsMachineProviderPoliciesKeyValuePair struct {
	Key string `json:"key,omitempty"`

	Value string `json:"value,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Key") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Key") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ComponentsMachineProviderPoliciesKeyValuePair) MarshalJSON() ([]byte, error) {
	type NoMethod ComponentsMachineProviderPoliciesKeyValuePair
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ComponentsMachineProviderPoliciesPolicies: Represents the policies
// for a machine. There are two Pub/Sub channels of communication for
// each machine. One is the backend-level channel which the Machine
// Provider will use to tell the backend that the machine has been
// leased, or that the machine needs to be reclaimed. The other is the
// channel between the Machine Provider and the machine itself. The
// machine should listen for instructions from the Machine Provider on
// this channel. Since the machine itself is what's being leased out to
// untrusted users, we will assign this Cloud Pub/Sub topic and give it
// restricted permissions which only allow it to subscribe to the one
// topic. On the other hand, the backend is trusted so we allow it to
// choose its own topic. When a backend adds a machine to the Catalog,
// it should provide the Pub/Sub topic and project to communicate on
// regarding the machine, as well as the service account on the machine
// itself which will be used to authenticate pull requests on the
// subscription created by the Machine Provider for the machine.
type ComponentsMachineProviderPoliciesPolicies struct {
	// BackendAttributes: Represents a key-value pair.
	BackendAttributes []*ComponentsMachineProviderPoliciesKeyValuePair `json:"backend_attributes,omitempty"`

	BackendProject string `json:"backend_project,omitempty"`

	BackendTopic string `json:"backend_topic,omitempty"`

	MachineServiceAccount string `json:"machine_service_account,omitempty"`

	// Possible values:
	//   "DELETE"
	//   "MAKE_AVAILABLE" (default)
	//   "RECLAIM"
	OnReclamation string `json:"on_reclamation,omitempty"`

	// ForceSendFields is a list of field names (e.g. "BackendAttributes")
	// to unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "BackendAttributes") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *ComponentsMachineProviderPoliciesPolicies) MarshalJSON() ([]byte, error) {
	type NoMethod ComponentsMachineProviderPoliciesPolicies
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ComponentsMachineProviderRpcMessagesCatalogBatchManipulationResponse:
// Represents a response to a batched catalog manipulation request.
type ComponentsMachineProviderRpcMessagesCatalogBatchManipulationResponse struct {
	// Responses: Represents a response to a catalog manipulation request.
	Responses []*ComponentsMachineProviderRpcMessagesCatalogManipulationResponse `json:"responses,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Responses") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Responses") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ComponentsMachineProviderRpcMessagesCatalogBatchManipulationResponse) MarshalJSON() ([]byte, error) {
	type NoMethod ComponentsMachineProviderRpcMessagesCatalogBatchManipulationResponse
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ComponentsMachineProviderRpcMessagesCatalogMachineAdditionRequest:
// Represents a request to add a machine to the catalog.
// dimensions.backend must be specified. dimensions.hostname must be
// unique per backend.
type ComponentsMachineProviderRpcMessagesCatalogMachineAdditionRequest struct {
	// Dimensions: Represents the dimensions of a machine.
	Dimensions *ComponentsMachineProviderDimensionsDimensions `json:"dimensions,omitempty"`

	// Policies: Represents the policies for a machine. There are two
	// Pub/Sub channels of communication for each machine. One is the
	// backend-level channel which the Machine Provider will use to tell the
	// backend that the machine has been leased, or that the machine needs
	// to be reclaimed. The other is the channel between the Machine
	// Provider and the machine itself. The machine should listen for
	// instructions from the Machine Provider on this channel. Since the
	// machine itself is what's being leased out to untrusted users, we will
	// assign this Cloud Pub/Sub topic and give it restricted permissions
	// which only allow it to subscribe to the one topic. On the other hand,
	// the backend is trusted so we allow it to choose its own topic. When a
	// backend adds a machine to the Catalog, it should provide the Pub/Sub
	// topic and project to communicate on regarding the machine, as well as
	// the service account on the machine itself which will be used to
	// authenticate pull requests on the subscription created by the Machine
	// Provider for the machine.
	Policies *ComponentsMachineProviderPoliciesPolicies `json:"policies,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Dimensions") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Dimensions") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ComponentsMachineProviderRpcMessagesCatalogMachineAdditionRequest) MarshalJSON() ([]byte, error) {
	type NoMethod ComponentsMachineProviderRpcMessagesCatalogMachineAdditionRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ComponentsMachineProviderRpcMessagesCatalogMachineBatchAdditionRequest
// : Represents a batched set of CatalogMachineAdditionRequests.
// dimensions.backend must be specified in each
// CatalogMachineAdditionRequest. dimensions.hostname must be unique per
// backend.
type ComponentsMachineProviderRpcMessagesCatalogMachineBatchAdditionRequest struct {
	// Requests: Represents a request to add a machine to the catalog.
	// dimensions.backend must be specified. dimensions.hostname must be
	// unique per backend.
	Requests []*ComponentsMachineProviderRpcMessagesCatalogMachineAdditionRequest `json:"requests,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Requests") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Requests") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ComponentsMachineProviderRpcMessagesCatalogMachineBatchAdditionRequest) MarshalJSON() ([]byte, error) {
	type NoMethod ComponentsMachineProviderRpcMessagesCatalogMachineBatchAdditionRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ComponentsMachineProviderRpcMessagesCatalogMachineDeletionRequest:
// Represents a request to delete a machine in the catalog.
type ComponentsMachineProviderRpcMessagesCatalogMachineDeletionRequest struct {
	// Dimensions: Represents the dimensions of a machine.
	Dimensions *ComponentsMachineProviderDimensionsDimensions `json:"dimensions,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Dimensions") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Dimensions") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ComponentsMachineProviderRpcMessagesCatalogMachineDeletionRequest) MarshalJSON() ([]byte, error) {
	type NoMethod ComponentsMachineProviderRpcMessagesCatalogMachineDeletionRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalRequest:
// Represents a request to retrieve a machine from the catalog.
type ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalRequest struct {
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

func (s *ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalRequest) MarshalJSON() ([]byte, error) {
	type NoMethod ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalResponse:
// Represents a response to a catalog machine retrieval request.
type ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalResponse struct {
	// Dimensions: Represents the dimensions of a machine.
	Dimensions *ComponentsMachineProviderDimensionsDimensions `json:"dimensions,omitempty"`

	LeaseExpirationTs int64 `json:"lease_expiration_ts,omitempty,string"`

	// Policies: Represents the policies for a machine. There are two
	// Pub/Sub channels of communication for each machine. One is the
	// backend-level channel which the Machine Provider will use to tell the
	// backend that the machine has been leased, or that the machine needs
	// to be reclaimed. The other is the channel between the Machine
	// Provider and the machine itself. The machine should listen for
	// instructions from the Machine Provider on this channel. Since the
	// machine itself is what's being leased out to untrusted users, we will
	// assign this Cloud Pub/Sub topic and give it restricted permissions
	// which only allow it to subscribe to the one topic. On the other hand,
	// the backend is trusted so we allow it to choose its own topic. When a
	// backend adds a machine to the Catalog, it should provide the Pub/Sub
	// topic and project to communicate on regarding the machine, as well as
	// the service account on the machine itself which will be used to
	// authenticate pull requests on the subscription created by the Machine
	// Provider for the machine.
	Policies *ComponentsMachineProviderPoliciesPolicies `json:"policies,omitempty"`

	PubsubSubscription string `json:"pubsub_subscription,omitempty"`

	PubsubSubscriptionProject string `json:"pubsub_subscription_project,omitempty"`

	PubsubTopic string `json:"pubsub_topic,omitempty"`

	PubsubTopicProject string `json:"pubsub_topic_project,omitempty"`

	State string `json:"state,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Dimensions") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Dimensions") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalResponse) MarshalJSON() ([]byte, error) {
	type NoMethod ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalResponse
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ComponentsMachineProviderRpcMessagesCatalogManipulationResponse:
// Represents a response to a catalog manipulation request.
type ComponentsMachineProviderRpcMessagesCatalogManipulationResponse struct {
	// Possible values:
	//   "ENTRY_NOT_FOUND"
	//   "HOSTNAME_REUSE"
	//   "INVALID_PROJECT"
	//   "INVALID_TOPIC"
	//   "LEASED"
	//   "MISMATCHED_BACKEND"
	//   "UNSPECIFIED_BACKEND"
	//   "UNSPECIFIED_HOSTNAME"
	//   "UNSPECIFIED_TOPIC"
	Error string `json:"error,omitempty"`

	// MachineAdditionRequest: Represents a request to add a machine to the
	// catalog. dimensions.backend must be specified. dimensions.hostname
	// must be unique per backend.
	MachineAdditionRequest *ComponentsMachineProviderRpcMessagesCatalogMachineAdditionRequest `json:"machine_addition_request,omitempty"`

	// MachineDeletionRequest: Represents a request to delete a machine in
	// the catalog.
	MachineDeletionRequest *ComponentsMachineProviderRpcMessagesCatalogMachineDeletionRequest `json:"machine_deletion_request,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Error") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Error") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ComponentsMachineProviderRpcMessagesCatalogManipulationResponse) MarshalJSON() ([]byte, error) {
	type NoMethod ComponentsMachineProviderRpcMessagesCatalogManipulationResponse
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// method id "catalog.add_machine":

type AddMachineCall struct {
	s                                                                 *Service
	componentsmachineproviderrpcmessagescatalogmachineadditionrequest *ComponentsMachineProviderRpcMessagesCatalogMachineAdditionRequest
	urlParams_                                                        gensupport.URLParams
	ctx_                                                              context.Context
	header_                                                           http.Header
}

// AddMachine: Handles an incoming CatalogMachineAdditionRequest.
func (s *Service) AddMachine(componentsmachineproviderrpcmessagescatalogmachineadditionrequest *ComponentsMachineProviderRpcMessagesCatalogMachineAdditionRequest) *AddMachineCall {
	c := &AddMachineCall{s: s, urlParams_: make(gensupport.URLParams)}
	c.componentsmachineproviderrpcmessagescatalogmachineadditionrequest = componentsmachineproviderrpcmessagescatalogmachineadditionrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *AddMachineCall) Fields(s ...googleapi.Field) *AddMachineCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *AddMachineCall) Context(ctx context.Context) *AddMachineCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *AddMachineCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *AddMachineCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.componentsmachineproviderrpcmessagescatalogmachineadditionrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "add_machine")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "catalog.add_machine" call.
// Exactly one of
// *ComponentsMachineProviderRpcMessagesCatalogManipulationResponse or
// error will be non-nil. Any non-2xx status code is an error. Response
// headers are in either
// *ComponentsMachineProviderRpcMessagesCatalogManipulationResponse.Serve
// rResponse.Header or (if a response was returned at all) in
// error.(*googleapi.Error).Header. Use googleapi.IsNotModified to check
// whether the returned error was because http.StatusNotModified was
// returned.
func (c *AddMachineCall) Do(opts ...googleapi.CallOption) (*ComponentsMachineProviderRpcMessagesCatalogManipulationResponse, error) {
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
	ret := &ComponentsMachineProviderRpcMessagesCatalogManipulationResponse{
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
	//   "description": "Handles an incoming CatalogMachineAdditionRequest.",
	//   "httpMethod": "POST",
	//   "id": "catalog.add_machine",
	//   "path": "add_machine",
	//   "request": {
	//     "$ref": "ComponentsMachineProviderRpcMessagesCatalogMachineAdditionRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "ComponentsMachineProviderRpcMessagesCatalogManipulationResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "catalog.add_machines":

type AddMachinesCall struct {
	s                                                                      *Service
	componentsmachineproviderrpcmessagescatalogmachinebatchadditionrequest *ComponentsMachineProviderRpcMessagesCatalogMachineBatchAdditionRequest
	urlParams_                                                             gensupport.URLParams
	ctx_                                                                   context.Context
	header_                                                                http.Header
}

// AddMachines: Handles an incoming CatalogMachineBatchAdditionRequest.
// Batches are intended to save on RPCs only. The batched requests will
// not execute transactionally.
func (s *Service) AddMachines(componentsmachineproviderrpcmessagescatalogmachinebatchadditionrequest *ComponentsMachineProviderRpcMessagesCatalogMachineBatchAdditionRequest) *AddMachinesCall {
	c := &AddMachinesCall{s: s, urlParams_: make(gensupport.URLParams)}
	c.componentsmachineproviderrpcmessagescatalogmachinebatchadditionrequest = componentsmachineproviderrpcmessagescatalogmachinebatchadditionrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *AddMachinesCall) Fields(s ...googleapi.Field) *AddMachinesCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *AddMachinesCall) Context(ctx context.Context) *AddMachinesCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *AddMachinesCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *AddMachinesCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.componentsmachineproviderrpcmessagescatalogmachinebatchadditionrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "add_machines")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "catalog.add_machines" call.
// Exactly one of
// *ComponentsMachineProviderRpcMessagesCatalogBatchManipulationResponse
// or error will be non-nil. Any non-2xx status code is an error.
// Response headers are in either
// *ComponentsMachineProviderRpcMessagesCatalogBatchManipulationResponse.
// ServerResponse.Header or (if a response was returned at all) in
// error.(*googleapi.Error).Header. Use googleapi.IsNotModified to check
// whether the returned error was because http.StatusNotModified was
// returned.
func (c *AddMachinesCall) Do(opts ...googleapi.CallOption) (*ComponentsMachineProviderRpcMessagesCatalogBatchManipulationResponse, error) {
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
	ret := &ComponentsMachineProviderRpcMessagesCatalogBatchManipulationResponse{
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
	//   "description": "Handles an incoming CatalogMachineBatchAdditionRequest. Batches are intended to save on RPCs only. The batched requests will not execute transactionally.",
	//   "httpMethod": "POST",
	//   "id": "catalog.add_machines",
	//   "path": "add_machines",
	//   "request": {
	//     "$ref": "ComponentsMachineProviderRpcMessagesCatalogMachineBatchAdditionRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "ComponentsMachineProviderRpcMessagesCatalogBatchManipulationResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "catalog.delete_machine":

type DeleteMachineCall struct {
	s                                                                 *Service
	componentsmachineproviderrpcmessagescatalogmachinedeletionrequest *ComponentsMachineProviderRpcMessagesCatalogMachineDeletionRequest
	urlParams_                                                        gensupport.URLParams
	ctx_                                                              context.Context
	header_                                                           http.Header
}

// DeleteMachine: Handles an incoming CatalogMachineDeletionRequest.
func (s *Service) DeleteMachine(componentsmachineproviderrpcmessagescatalogmachinedeletionrequest *ComponentsMachineProviderRpcMessagesCatalogMachineDeletionRequest) *DeleteMachineCall {
	c := &DeleteMachineCall{s: s, urlParams_: make(gensupport.URLParams)}
	c.componentsmachineproviderrpcmessagescatalogmachinedeletionrequest = componentsmachineproviderrpcmessagescatalogmachinedeletionrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *DeleteMachineCall) Fields(s ...googleapi.Field) *DeleteMachineCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *DeleteMachineCall) Context(ctx context.Context) *DeleteMachineCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *DeleteMachineCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *DeleteMachineCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.componentsmachineproviderrpcmessagescatalogmachinedeletionrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "delete_machine")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "catalog.delete_machine" call.
// Exactly one of
// *ComponentsMachineProviderRpcMessagesCatalogManipulationResponse or
// error will be non-nil. Any non-2xx status code is an error. Response
// headers are in either
// *ComponentsMachineProviderRpcMessagesCatalogManipulationResponse.Serve
// rResponse.Header or (if a response was returned at all) in
// error.(*googleapi.Error).Header. Use googleapi.IsNotModified to check
// whether the returned error was because http.StatusNotModified was
// returned.
func (c *DeleteMachineCall) Do(opts ...googleapi.CallOption) (*ComponentsMachineProviderRpcMessagesCatalogManipulationResponse, error) {
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
	ret := &ComponentsMachineProviderRpcMessagesCatalogManipulationResponse{
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
	//   "description": "Handles an incoming CatalogMachineDeletionRequest.",
	//   "httpMethod": "POST",
	//   "id": "catalog.delete_machine",
	//   "path": "delete_machine",
	//   "request": {
	//     "$ref": "ComponentsMachineProviderRpcMessagesCatalogMachineDeletionRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "ComponentsMachineProviderRpcMessagesCatalogManipulationResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "catalog.get":

type GetCall struct {
	s                                                                  *Service
	componentsmachineproviderrpcmessagescatalogmachineretrievalrequest *ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalRequest
	urlParams_                                                         gensupport.URLParams
	ctx_                                                               context.Context
	header_                                                            http.Header
}

// Get: Handles an incoming CatalogMachineRetrievalRequest.
func (s *Service) Get(componentsmachineproviderrpcmessagescatalogmachineretrievalrequest *ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalRequest) *GetCall {
	c := &GetCall{s: s, urlParams_: make(gensupport.URLParams)}
	c.componentsmachineproviderrpcmessagescatalogmachineretrievalrequest = componentsmachineproviderrpcmessagescatalogmachineretrievalrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *GetCall) Fields(s ...googleapi.Field) *GetCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *GetCall) Context(ctx context.Context) *GetCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *GetCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *GetCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.componentsmachineproviderrpcmessagescatalogmachineretrievalrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "get")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "catalog.get" call.
// Exactly one of
// *ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalResponse
// or error will be non-nil. Any non-2xx status code is an error.
// Response headers are in either
// *ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalResponse.S
// erverResponse.Header or (if a response was returned at all) in
// error.(*googleapi.Error).Header. Use googleapi.IsNotModified to check
// whether the returned error was because http.StatusNotModified was
// returned.
func (c *GetCall) Do(opts ...googleapi.CallOption) (*ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalResponse, error) {
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
	ret := &ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalResponse{
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
	//   "description": "Handles an incoming CatalogMachineRetrievalRequest.",
	//   "httpMethod": "POST",
	//   "id": "catalog.get",
	//   "path": "get",
	//   "request": {
	//     "$ref": "ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "ComponentsMachineProviderRpcMessagesCatalogMachineRetrievalResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}
