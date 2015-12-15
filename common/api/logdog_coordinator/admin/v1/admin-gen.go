// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package admin provides access to the .
//
// Usage example:
//
//   import "github.com/luci/luci-go/common/api/logdog_coordinator/admin/v1"
//   ...
//   adminService, err := admin.New(oauthHttpClient)
package admin // import "github.com/luci/luci-go/common/api/logdog_coordinator/admin/v1"

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

const apiId = "admin:v1"
const apiName = "admin"
const apiVersion = "v1"
const basePath = "http://localhost:8080/api/admin/v1/"

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

type SetConfigRequest struct {
	ConfigPath string `json:"configPath,omitempty"`

	ConfigService string `json:"configService,omitempty"`

	ConfigSet string `json:"configSet,omitempty"`

	// ForceSendFields is a list of field names (e.g. "ConfigPath") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`
}

func (s *SetConfigRequest) MarshalJSON() ([]byte, error) {
	type noMethod SetConfigRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields)
}

// method id "admin.setConfig":

type SetConfigCall struct {
	s                *Service
	setconfigrequest *SetConfigRequest
	opt_             map[string]interface{}
	ctx_             context.Context
}

// SetConfig: Set the instance's global configuration parameters.
func (s *Service) SetConfig(setconfigrequest *SetConfigRequest) *SetConfigCall {
	c := &SetConfigCall{s: s, opt_: make(map[string]interface{})}
	c.setconfigrequest = setconfigrequest
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *SetConfigCall) Fields(s ...googleapi.Field) *SetConfigCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

// Context sets the context to be used in this call's Do method.
// Any pending HTTP request will be aborted if the provided context
// is canceled.
func (c *SetConfigCall) Context(ctx context.Context) *SetConfigCall {
	c.ctx_ = ctx
	return c
}

func (c *SetConfigCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.setconfigrequest)
	if err != nil {
		return nil, err
	}
	ctype := "application/json"
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "setconfig")
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

// Do executes the "admin.setConfig" call.
func (c *SetConfigCall) Do() error {
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
	//   "description": "Set the instance's global configuration parameters.",
	//   "httpMethod": "POST",
	//   "id": "admin.setConfig",
	//   "path": "setconfig",
	//   "request": {
	//     "$ref": "SetConfigRequest",
	//     "parameterName": "resource"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}
