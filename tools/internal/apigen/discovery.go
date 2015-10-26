// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apigen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	// Capture {stuff}.
	reParam = regexp.MustCompile(`\{([^\}]+)\}`)
)

type allAPIs struct {
	Items []string `json:"items"`
}

type hostedAPIs struct {
	apis []*singleAPI
}

// directoryList converts a list of backend APIs into an API directory.
//
// The conversion isn't perfect or spec-confirming. It makes a set of minimal
// mutations to be compatible with the `google-api-go-generator` tool.
//
// Each directory item's  DiscoveryLink is generated from the item's index.
// For example, directory item #0 is hosted at "./apis/0/rest".
func (h *hostedAPIs) directoryList() (*directoryList, error) {
	err := error(nil)
	d := directoryList{
		Kind:             "discovery#directoryList",
		DiscoveryVersion: "v1",
		Items:            make([]*directoryItem, len(h.apis)),
	}

	for i, api := range h.apis {
		di := directoryItem{
			Kind:             "discovery#directoryItem",
			ID:               api.ID(),
			Name:             api.Name,
			Version:          api.Version,
			Title:            api.Title,
			Description:      api.Description,
			DiscoveryRestURL: "http://localhost/bogus",
			DiscoveryLink:    fmt.Sprintf("./apis/%d/rest", i),
			RootURL:          fmt.Sprintf("http://example.com/_ah/api/%s/%s/", api.Name, api.Version),
			Preferred:        false,
		}

		di.rdesc, err = api.restDescription()
		if err != nil {
			return nil, fmt.Errorf("error on API #%d (%s): %s", i, api.ID(), err)
		}

		d.Items[i] = &di
	}

	return &d, nil
}

// translateAPIs converts a Cloud Endpoints backend API listing into an appAPIs
// array of singleAPI elements.
func translateAPIs(data []byte) (*hostedAPIs, error) {
	allAPIs := &allAPIs{}
	dec := json.NewDecoder(bytes.NewBuffer(data))
	dec.UseNumber()
	if err := dec.Decode(&allAPIs); err != nil {
		return nil, err
	}

	d := hostedAPIs{}
	for _, item := range allAPIs.Items {
		api, err := loadSingleAPI(item)
		if err != nil {
			return nil, err
		}
		d.apis = append(d.apis, api)
	}

	return &d, nil
}

// singleAPI is the input form of a decoded element in the allAPIs Items list.
// It represents a single exported backend Cloud Endpoints API.
type singleAPI struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	Description    string `json:"description"`
	Title          string `json:"title"`
	Root           string `json:"root"`
	DefaultVersion bool   `json:"defaultVersion"`

	Descriptor struct {
		Schemas map[string]*singleAPISchema `json:"schemas"`
		Methods map[string]struct {
			Request  *singleAPIMethodRef
			Response *singleAPIMethodRef
		} `json:"methods"`
	}
	Methods map[string]struct {
		Description string           `json:"description"`
		HTTPMethod  string           `json:"httpMethod"`
		Scopes      []string         `json:"scopes"`
		Path        string           `json:"path"`
		Request     *singleAPIMethod `json:"request"`
		Response    *singleAPIMethod `json:"response"`
		RosyMethod  string           `json:"rosyMethod"`
	} `json:"methods"`
}

type singleAPISchema struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Properties map[string]struct {
		Type   string              `json:"type,omitempty"`
		Format string              `json:"format,omitempty"`
		Ref    string              `json:"$ref,omitempty"`
		Items  *singleAPIMethodRef `json:"items,omitempty"`
	} `json:"properties"`
}

type singleAPIMethodRef struct {
	Ref  string `json:"$ref,omitempty"`
	Type string `json:"type,omitempty"`
}

type singleAPIMethod struct {
	Body       string `json:"body"`
	BodyName   string `json:"bodyName"`
	Parameters map[string]struct {
		Required bool   `json:"required"`
		Type     string `json:"type"`
	} `json:"parameters"`
}

// loadSingleAPI loads a singleAPI from a single backend item.
func loadSingleAPI(j string) (*singleAPI, error) {
	dec := json.NewDecoder(strings.NewReader(j))
	dec.UseNumber()

	api := singleAPI{}
	if err := dec.Decode(&api); err != nil {
		return nil, err
	}

	return &api, nil
}

func (a *singleAPI) ID() string {
	return fmt.Sprintf("%s:%s", a.Name, a.Version)
}

// restDescription returns a single directory item's REST description structure.
//
// This is a JSON-compatible element that describes the APIs exported by a
// singleAPI.
func (a *singleAPI) restDescription() (*restDescription, error) {
	rdesc := restDescription{
		Kind:             "discovery#restDescription",
		DiscoveryVersion: "v1",

		ID:             a.ID(),
		Name:           a.Name,
		Version:        a.Version,
		Description:    a.Description,
		Root:           a.Root,
		DefaultVersion: a.DefaultVersion,
		Schemas:        make(map[string]*singleAPISchema, len(a.Descriptor.Schemas)),
		Methods:        map[string]*restMethod{},
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
			Description: desc.Description,
			Parameters:  map[string]*restMethodParameter{},

			a: a,
		}
		m.ParameterOrder = parseParameterOrderFromPath(m.Path)

		err := error(nil)
		m.Request, err = m.buildMethodParamRef(desc.Request, a.Descriptor.Methods[desc.RosyMethod].Request)
		if err != nil {
			return nil, fmt.Errorf("failed to build method [%s] request: %s", m.ID, err)
		}

		m.Response, err = m.buildMethodParamRef(desc.Response, a.Descriptor.Methods[desc.RosyMethod].Response)
		if err != nil {
			return nil, fmt.Errorf("failed to build method [%s] response: %s", m.ID, err)
		}

		rdesc.Methods[strings.TrimPrefix(m.ID, fmt.Sprintf("%s.", a.Name))] = &m
	}

	return &rdesc, nil
}

func (m *restMethod) buildMethodParamRef(desc *singleAPIMethod, ref *singleAPIMethodRef) (*restMethodParameterRef, error) {
	for name, param := range desc.Parameters {
		rmp := restMethodParameter{
			Type:     param.Type,
			Required: param.Required, // All path parameters are required.
		}
		if m.HTTPMethod == "GET" {
			rmp.Location = "query"
		} else {
			rmp.Location = "path"
		}
		m.Parameters[name] = &rmp
	}

	if desc.Body == "empty" {
		return nil, nil
	}

	paramRef := restMethodParameterRef{
		Ref:           ref.Ref,
		ParameterName: desc.BodyName,
	}

	return &paramRef, nil
}

func parseParameterOrderFromPath(path string) []string {
	matches := reParam.FindAllStringSubmatch(path, -1)
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

	rdesc *restDescription
}

// restDescription is a hosted JSON at a rest endpoint. It is translated from a
// singleAPI into a form served by the frontend.
type restDescription struct {
	Kind             string `json:"kind,omitempty"`
	DiscoveryVersion string `json:"discoveryVersion,omitempty"`

	ID             string `json:"id,omitempty"`
	Name           string `json:"name,omitempty"`
	Version        string `json:"version,omitempty"`
	Description    string `json:"description,omitempty"`
	Root           string `json:"root,omitempty"`
	DefaultVersion bool   `json:"defaultVersion,omitempty"`

	Schemas map[string]*singleAPISchema `json:"schemas,omitempty"`
	Methods map[string]*restMethod      `json:"methods,omitempty"`
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

	// a is the source API for this method.
	a *singleAPI
}

type restMethodParameter struct {
	Type     string `json:"type,omitempty"`
	Location string `json:"location,omitempty"`
	Required bool   `json:"required,omitempty"`
}

type restMethodParameterRef struct {
	Ref           string `json:"$ref,omitempty"`
	ParameterName string `json:"parameterName,omitempty"`
}

// relativeLink returns the URL path of an item's DiscoveryLink.
//
// A DiscoveryLink is expressed relative to a root path. This API returns the
// full path given the root component.
//
// For example:
//   Root: path/to/apis
//   DiscoveryLink: ./apis/0/rest
//   relativeLink: path/to/apis/apis/0/rest
func (e *directoryItem) relativeLink(root string) string {
	return root + strings.TrimPrefix(e.DiscoveryLink, "./")
}
