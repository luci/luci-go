// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package config provides access to the Configuration Service.
//
// Usage example:
//
//   import "google.golang.org/api/config/v1"
//   ...
//   configService, err := config.New(oauthHttpClient)
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
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
var _ = context.Background

const apiId = "config:v1"
const apiName = "config"
const apiVersion = "v1"
const basePath = "https://luci-config.appspot.com/_ah/api/config/v1/"

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

type LuciConfigGetConfigByHashResponseMessage struct {
	Content string `json:"content,omitempty"`
}

type LuciConfigGetConfigMultiResponseMessage struct {
	Configs []*LuciConfigGetConfigMultiResponseMessageConfigEntry `json:"configs,omitempty"`
}

type LuciConfigGetConfigMultiResponseMessageConfigEntry struct {
	ConfigSet string `json:"config_set,omitempty"`

	Content string `json:"content,omitempty"`

	ContentHash string `json:"content_hash,omitempty"`

	Revision string `json:"revision,omitempty"`
}

type LuciConfigGetConfigResponseMessage struct {
	Content string `json:"content,omitempty"`

	ContentHash string `json:"content_hash,omitempty"`

	Revision string `json:"revision,omitempty"`
}

type LuciConfigGetMappingResponseMessage struct {
	Mappings []*LuciConfigGetMappingResponseMessageMapping `json:"mappings,omitempty"`
}

type LuciConfigGetMappingResponseMessageMapping struct {
	ConfigSet string `json:"config_set,omitempty"`

	Location string `json:"location,omitempty"`
}

type LuciConfigGetProjectsResponseMessage struct {
	Projects []*LuciConfigProject `json:"projects,omitempty"`
}

type LuciConfigGetRefsResponseMessage struct {
	Refs []*LuciConfigGetRefsResponseMessageRef `json:"refs,omitempty"`
}

type LuciConfigGetRefsResponseMessageRef struct {
	Name string `json:"name,omitempty"`
}

type LuciConfigProject struct {
	Id string `json:"id,omitempty"`

	Name string `json:"name,omitempty"`

	// Possible values:
	//   "GITILES"
	RepoType string `json:"repo_type,omitempty"`

	RepoUrl string `json:"repo_url,omitempty"`
}

// method id "config.get_config":

type GetConfigCall struct {
	s         *Service
	configSet string
	path      string
	opt_      map[string]interface{}
}

// GetConfig: Gets a config file.
func (s *Service) GetConfig(configSet string, path string) *GetConfigCall {
	c := &GetConfigCall{s: s, opt_: make(map[string]interface{})}
	c.configSet = configSet
	c.path = path
	return c
}

// HashOnly sets the optional parameter "hash_only":
func (c *GetConfigCall) HashOnly(hashOnly bool) *GetConfigCall {
	c.opt_["hash_only"] = hashOnly
	return c
}

// Revision sets the optional parameter "revision":
func (c *GetConfigCall) Revision(revision string) *GetConfigCall {
	c.opt_["revision"] = revision
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *GetConfigCall) Fields(s ...googleapi.Field) *GetConfigCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

func (c *GetConfigCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["hash_only"]; ok {
		params.Set("hash_only", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["revision"]; ok {
		params.Set("revision", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "config_sets/{config_set}/config/{path}")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"config_set": c.configSet,
		"path":       c.path,
	})
	req.Header.Set("User-Agent", c.s.userAgent())
	return c.s.client.Do(req)
}

func (c *GetConfigCall) Do() (*LuciConfigGetConfigResponseMessage, error) {
	res, err := c.doRequest("json")
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	var ret *LuciConfigGetConfigResponseMessage
	if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Gets a config file.",
	//   "httpMethod": "GET",
	//   "id": "config.get_config",
	//   "parameterOrder": [
	//     "config_set",
	//     "path"
	//   ],
	//   "parameters": {
	//     "config_set": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     },
	//     "hash_only": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "path": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     },
	//     "revision": {
	//       "location": "query",
	//       "type": "string"
	//     }
	//   },
	//   "path": "config_sets/{config_set}/config/{path}",
	//   "response": {
	//     "$ref": "LuciConfigGetConfigResponseMessage"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "config.get_config_by_hash":

type GetConfigByHashCall struct {
	s           *Service
	contentHash string
	opt_        map[string]interface{}
}

// GetConfigByHash: Gets a config file by its hash.
func (s *Service) GetConfigByHash(contentHash string) *GetConfigByHashCall {
	c := &GetConfigByHashCall{s: s, opt_: make(map[string]interface{})}
	c.contentHash = contentHash
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *GetConfigByHashCall) Fields(s ...googleapi.Field) *GetConfigByHashCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

func (c *GetConfigByHashCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "config/{content_hash}")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"content_hash": c.contentHash,
	})
	req.Header.Set("User-Agent", c.s.userAgent())
	return c.s.client.Do(req)
}

func (c *GetConfigByHashCall) Do() (*LuciConfigGetConfigByHashResponseMessage, error) {
	res, err := c.doRequest("json")
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	var ret *LuciConfigGetConfigByHashResponseMessage
	if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Gets a config file by its hash.",
	//   "httpMethod": "GET",
	//   "id": "config.get_config_by_hash",
	//   "parameterOrder": [
	//     "content_hash"
	//   ],
	//   "parameters": {
	//     "content_hash": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "config/{content_hash}",
	//   "response": {
	//     "$ref": "LuciConfigGetConfigByHashResponseMessage"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "config.get_mapping":

type GetMappingCall struct {
	s    *Service
	opt_ map[string]interface{}
}

// GetMapping: Returns config-set mapping, one or all.
func (s *Service) GetMapping() *GetMappingCall {
	c := &GetMappingCall{s: s, opt_: make(map[string]interface{})}
	return c
}

// ConfigSet sets the optional parameter "config_set":
func (c *GetMappingCall) ConfigSet(configSet string) *GetMappingCall {
	c.opt_["config_set"] = configSet
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *GetMappingCall) Fields(s ...googleapi.Field) *GetMappingCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

func (c *GetMappingCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["config_set"]; ok {
		params.Set("config_set", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "mapping")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("User-Agent", c.s.userAgent())
	return c.s.client.Do(req)
}

func (c *GetMappingCall) Do() (*LuciConfigGetMappingResponseMessage, error) {
	res, err := c.doRequest("json")
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	var ret *LuciConfigGetMappingResponseMessage
	if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns config-set mapping, one or all.",
	//   "httpMethod": "GET",
	//   "id": "config.get_mapping",
	//   "parameters": {
	//     "config_set": {
	//       "location": "query",
	//       "type": "string"
	//     }
	//   },
	//   "path": "mapping",
	//   "response": {
	//     "$ref": "LuciConfigGetMappingResponseMessage"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "config.get_project_configs":

type GetProjectConfigsCall struct {
	s    *Service
	path string
	opt_ map[string]interface{}
}

// GetProjectConfigs: Gets configs in all project config sets.
func (s *Service) GetProjectConfigs(path string) *GetProjectConfigsCall {
	c := &GetProjectConfigsCall{s: s, opt_: make(map[string]interface{})}
	c.path = path
	return c
}

// HashesOnly sets the optional parameter "hashes_only":
func (c *GetProjectConfigsCall) HashesOnly(hashesOnly bool) *GetProjectConfigsCall {
	c.opt_["hashes_only"] = hashesOnly
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *GetProjectConfigsCall) Fields(s ...googleapi.Field) *GetProjectConfigsCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

func (c *GetProjectConfigsCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["hashes_only"]; ok {
		params.Set("hashes_only", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "configs/projects/{path}")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"path": c.path,
	})
	req.Header.Set("User-Agent", c.s.userAgent())
	return c.s.client.Do(req)
}

func (c *GetProjectConfigsCall) Do() (*LuciConfigGetConfigMultiResponseMessage, error) {
	res, err := c.doRequest("json")
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	var ret *LuciConfigGetConfigMultiResponseMessage
	if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Gets configs in all project config sets.",
	//   "httpMethod": "GET",
	//   "id": "config.get_project_configs",
	//   "parameterOrder": [
	//     "path"
	//   ],
	//   "parameters": {
	//     "hashes_only": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "path": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "configs/projects/{path}",
	//   "response": {
	//     "$ref": "LuciConfigGetConfigMultiResponseMessage"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "config.get_projects":

type GetProjectsCall struct {
	s    *Service
	opt_ map[string]interface{}
}

// GetProjects: Gets list of registered projects. The project list is
// stored in services/luci-config:projects.cfg.
func (s *Service) GetProjects() *GetProjectsCall {
	c := &GetProjectsCall{s: s, opt_: make(map[string]interface{})}
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *GetProjectsCall) Fields(s ...googleapi.Field) *GetProjectsCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

func (c *GetProjectsCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "projects")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.SetOpaque(req.URL)
	req.Header.Set("User-Agent", c.s.userAgent())
	return c.s.client.Do(req)
}

func (c *GetProjectsCall) Do() (*LuciConfigGetProjectsResponseMessage, error) {
	res, err := c.doRequest("json")
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	var ret *LuciConfigGetProjectsResponseMessage
	if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Gets list of registered projects. The project list is stored in services/luci-config:projects.cfg.",
	//   "httpMethod": "GET",
	//   "id": "config.get_projects",
	//   "path": "projects",
	//   "response": {
	//     "$ref": "LuciConfigGetProjectsResponseMessage"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "config.get_ref_configs":

type GetRefConfigsCall struct {
	s    *Service
	path string
	opt_ map[string]interface{}
}

// GetRefConfigs: Gets configs in all ref config sets.
func (s *Service) GetRefConfigs(path string) *GetRefConfigsCall {
	c := &GetRefConfigsCall{s: s, opt_: make(map[string]interface{})}
	c.path = path
	return c
}

// HashesOnly sets the optional parameter "hashes_only":
func (c *GetRefConfigsCall) HashesOnly(hashesOnly bool) *GetRefConfigsCall {
	c.opt_["hashes_only"] = hashesOnly
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *GetRefConfigsCall) Fields(s ...googleapi.Field) *GetRefConfigsCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

func (c *GetRefConfigsCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["hashes_only"]; ok {
		params.Set("hashes_only", fmt.Sprintf("%v", v))
	}
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "configs/refs/{path}")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"path": c.path,
	})
	req.Header.Set("User-Agent", c.s.userAgent())
	return c.s.client.Do(req)
}

func (c *GetRefConfigsCall) Do() (*LuciConfigGetConfigMultiResponseMessage, error) {
	res, err := c.doRequest("json")
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	var ret *LuciConfigGetConfigMultiResponseMessage
	if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Gets configs in all ref config sets.",
	//   "httpMethod": "GET",
	//   "id": "config.get_ref_configs",
	//   "parameterOrder": [
	//     "path"
	//   ],
	//   "parameters": {
	//     "hashes_only": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "path": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "configs/refs/{path}",
	//   "response": {
	//     "$ref": "LuciConfigGetConfigMultiResponseMessage"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "config.get_refs":

type GetRefsCall struct {
	s         *Service
	projectId string
	opt_      map[string]interface{}
}

// GetRefs: Gets list of refs of a project.
func (s *Service) GetRefs(projectId string) *GetRefsCall {
	c := &GetRefsCall{s: s, opt_: make(map[string]interface{})}
	c.projectId = projectId
	return c
}

// Fields allows partial responses to be retrieved.
// See https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *GetRefsCall) Fields(s ...googleapi.Field) *GetRefsCall {
	c.opt_["fields"] = googleapi.CombineFields(s)
	return c
}

func (c *GetRefsCall) doRequest(alt string) (*http.Response, error) {
	var body io.Reader = nil
	params := make(url.Values)
	params.Set("alt", alt)
	if v, ok := c.opt_["fields"]; ok {
		params.Set("fields", fmt.Sprintf("%v", v))
	}
	urls := googleapi.ResolveRelative(c.s.BasePath, "projects/{project_id}/refs")
	urls += "?" + params.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	googleapi.Expand(req.URL, map[string]string{
		"project_id": c.projectId,
	})
	req.Header.Set("User-Agent", c.s.userAgent())
	return c.s.client.Do(req)
}

func (c *GetRefsCall) Do() (*LuciConfigGetRefsResponseMessage, error) {
	res, err := c.doRequest("json")
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	var ret *LuciConfigGetRefsResponseMessage
	if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Gets list of refs of a project.",
	//   "httpMethod": "GET",
	//   "id": "config.get_refs",
	//   "parameterOrder": [
	//     "project_id"
	//   ],
	//   "parameters": {
	//     "project_id": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "projects/{project_id}/refs",
	//   "response": {
	//     "$ref": "LuciConfigGetRefsResponseMessage"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}
