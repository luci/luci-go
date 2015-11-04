// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package epfrontend

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
)

var (
	// Capture {stuff}.
	reParam = regexp.MustCompile(`\{([^\}]+)\}`)
)

func getHostURL(r *http.Request) *url.URL {
	u := url.URL{
		Scheme: "http",
		Host:   r.Host,
	}
	if r.TLS != nil {
		u.Scheme = "https"
	}
	return &u
}

func (s *Server) handleDirectoryList(r *http.Request) (interface{}, error) {
	u := getHostURL(r)
	u.Path = r.URL.Path
	return s.directoryList(u), nil
}

// directoryList converts a list of backend APIs into an API directory.
//
// The conversion isn't perfect or spec-confirming. It makes a set of minimal
// mutations to be compatible with the `google-api-go-generator` tool.
//
// Each directory item's  DiscoveryLink is generated from the item's index.
// For example, directory item #0 is hosted at "./apis/0/rest".
func (s *Server) directoryList(root *url.URL) *directoryList {
	serviceNames := make([]string, 0, len(s.services))
	for k := range s.services {
		serviceNames = append(serviceNames, k)
	}
	sort.Strings(serviceNames)

	d := directoryList{
		Kind:             "discovery#directoryList",
		DiscoveryVersion: "v1",
		Items:            make([]*directoryItem, len(serviceNames)),
	}

	for i, name := range serviceNames {
		api := s.services[name]
		di := directoryItem{
			Kind:             "discovery#directoryItem",
			ID:               apiID(api),
			Name:             api.Name,
			Version:          api.Version,
			Description:      api.Desc,
			DiscoveryRestURL: safeURLPathJoin(root.String(), api.Name, api.Version, "rest"),
			DiscoveryLink:    safeURLPathJoin(".", "apis", api.Name, api.Version, "rest"),
			RootURL:          root.String(),
			Preferred:        false,
		}
		d.Items[i] = &di
	}

	return &d
}

func apiID(a *endpoints.APIDescriptor) string {
	return fmt.Sprintf("%s:%s", a.Name, a.Version)
}

func (s *Server) handleRestDescription(r *http.Request, a *endpoints.APIDescriptor) (interface{}, error) {
	u := getHostURL(r)
	u.Path = s.root
	d, err := buildRestDescription(u, a)
	return d, err
}

// buildRestDescription returns a single directory item's REST description
// structure.
//
// This is a JSON-compatible element that describes the APIs exported by a
// singleAPI.
func buildRestDescription(root *url.URL, a *endpoints.APIDescriptor) (*restDescription, error) {
	servicePath := safeURLPathJoin(a.Name, a.Version)
	rdesc := restDescription{
		Kind:             "discovery#restDescription",
		DiscoveryVersion: "v1",

		Protocol:       "rest",
		ID:             apiID(a),
		Name:           a.Name,
		Version:        a.Version,
		Description:    a.Desc,
		RootURL:        root.String(),
		BasePath:       safeURLPathJoin("", root.Path, servicePath, ""),
		BaseURL:        safeURLPathJoin(root.String(), servicePath, ""),
		ServicePath:    safeURLPathJoin(servicePath, ""),
		DefaultVersion: a.Default,
	}
	if len(a.Descriptor.Schemas) > 0 {
		rdesc.Schemas = make(map[string]*endpoints.APISchemaDescriptor, len(a.Descriptor.Schemas))
	}
	if len(a.Methods) > 0 {
		rdesc.Methods = make(map[string]*restMethod, len(a.Methods))
	}

	for name, schema := range a.Descriptor.Schemas {
		schema := *schema
		rdesc.Schemas[name] = &schema
	}

	for id, desc := range a.Methods {
		m := restMethod{
			ID:          id,
			Path:        desc.Path,
			HTTPMethod:  desc.HTTPMethod,
			Scopes:      desc.Scopes,
			Description: desc.Desc,
		}
		m.ParameterOrder = parseParameterOrderFromPath(m.Path)
		if nParams := len(desc.Request.Params) + len(desc.Response.Params); nParams > 0 {
			m.Parameters = make(map[string]*restMethodParameter, nParams)
		}

		err := error(nil)
		rosyMethod := a.Descriptor.Methods[desc.RosyMethod]
		if rosyMethod == nil {
			return nil, fmt.Errorf("no descriptor for RosyMethod %q", desc.RosyMethod)
		}
		m.Request, err = m.buildMethodParamRef(&desc.Request, rosyMethod.Request, true)
		if err != nil {
			return nil, fmt.Errorf("failed to build method [%s] request: %s", m.ID, err)
		}

		m.Response, err = m.buildMethodParamRef(&desc.Response, rosyMethod.Response, false)
		if err != nil {
			return nil, fmt.Errorf("failed to build method [%s] response: %s", m.ID, err)
		}

		rdesc.Methods[strings.TrimPrefix(m.ID, fmt.Sprintf("%s.", a.Name))] = &m
	}

	return &rdesc, nil
}

func (m *restMethod) buildMethodParamRef(desc *endpoints.APIReqRespDescriptor, ref *endpoints.APISchemaRef, canPath bool) (
	*restMethodParameterRef, error) {
	pathParams := map[string]struct{}{}
	if canPath {
		for _, p := range m.ParameterOrder {
			pathParams[p] = struct{}{}
		}
	}

	for name, param := range desc.Params {
		rmp := restMethodParameter{
			Required: param.Required, // All path parameters are required.
		}
		rmp.Type, rmp.Format = convertBackendType(param.Type)
		_, inPath := pathParams[name]

		if m.HTTPMethod == "GET" && !inPath {
			rmp.Location = "query"
		} else {
			rmp.Location = "path"
			rmp.Required = true
		}
		m.Parameters[name] = &rmp
	}

	if desc.Body == "empty" {
		return nil, nil
	}

	paramRef := restMethodParameterRef{
		ParameterName: desc.BodyName,
	}
	if ref != nil {
		paramRef.Ref = ref.Ref
	}

	return &paramRef, nil
}

func convertBackendType(t string) (string, string) {
	switch t {
	case "int32":
		return "integer", "int32"
	case "int64":
		return "string", "int64"
	case "uint32":
		return "integer", "uint32"
	case "uint64":
		return "string", "uint64"
	case "float":
		return "number", "float"
	case "double":
		return "number", "double"
	case "boolean":
		return "boolean", ""
	case "string":
		return "string", ""
	}

	return "", ""
}

func parseParameterOrderFromPath(path string) []string {
	matches := reParam.FindAllStringSubmatch(path, -1)
	if len(matches) == 0 {
		return nil
	}

	po := make([]string, 0, len(matches))
	for _, match := range matches {
		po = append(po, match[1])
	}
	return po
}

// directoryList is a Google Cloud Endpoints frontend directory list structure.
//
// This is the first-level directory structure, which exports a series of API
// items.
type directoryList struct {
	Kind             string           `json:"kind,omitempty"`
	DiscoveryVersion string           `json:"discoveryVersion,omitempty"`
	Items            []*directoryItem `json:"items,omitempty"`
}

// directoryItem is a single API's directoryList entry.
//
// This is a This is the second-level directory structure which exports a single
// API's methods.
//
// The directoryItem exports a REST API (restDescription) at its relative
// DiscoveryLink.
type directoryItem struct {
	Kind             string `json:"kind,omitempty"`
	ID               string `json:"id,omitempty"`
	Name             string `json:"name,omitempty"`
	Version          string `json:"version,omitempty"`
	Title            string `json:"title,omitempty"`
	Description      string `json:"description,omitempty"`
	DiscoveryRestURL string `json:"discoveryRestUrl,omitempty"`
	DiscoveryLink    string `json:"discoveryLink,omitempty"`
	RootURL          string `json:"rootUrl,omitempty"`
	Preferred        bool   `json:"preferred,omitempty"`
}

// restDescription is a hosted JSON at a rest endpoint. It is translated from a
// singleAPI into a form served by the frontend.
type restDescription struct {
	Kind             string `json:"kind,omitempty"`
	DiscoveryVersion string `json:"discoveryVersion,omitempty"`

	Protocol       string `json:"protocol,omitempty"`
	ID             string `json:"id,omitempty"`
	Name           string `json:"name,omitempty"`
	Version        string `json:"version,omitempty"`
	Description    string `json:"description,omitempty"`
	BaseURL        string `json:"baseUrl,omitempty"`
	BasePath       string `json:"basePath,omitempty"`
	RootURL        string `json:"rootUrl,omitempty"`
	ServicePath    string `json:"servicePath,omitempty"`
	Root           string `json:"root,omitempty"`
	DefaultVersion bool   `json:"defaultVersion,omitempty"`

	Schemas map[string]*endpoints.APISchemaDescriptor `json:"schemas,omitempty"`
	Methods map[string]*restMethod                    `json:"methods,omitempty"`
}

type restMethod struct {
	ID             string                          `json:"id,omitempty"`
	Path           string                          `json:"path,omitempty"`
	HTTPMethod     string                          `json:"httpMethod,omitempty"`
	Description    string                          `json:"description,omitempty"`
	Parameters     map[string]*restMethodParameter `json:"parameters,omitempty"`
	ParameterOrder []string                        `json:"parameterOrder,omitempty"`
	Request        *restMethodParameterRef         `json:"request,omitempty"`
	Response       *restMethodParameterRef         `json:"response,omitempty"`
	Scopes         []string                        `json:"scopes,omitempty"`
}

type restMethodParameter struct {
	Type     string `json:"type,omitempty"`
	Format   string `json:"format,omitempty"`
	Location string `json:"location,omitempty"`
	Required bool   `json:"required,omitempty"`
}

type restMethodParameterRef struct {
	Ref           string `json:"$ref,omitempty"`
	ParameterName string `json:"parameterName,omitempty"`
}
