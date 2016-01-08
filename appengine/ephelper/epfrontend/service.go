// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package epfrontend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/paniccatcher"
)

const (
	// DefaultServerRoot is the default Server root value.
	DefaultServerRoot = "/_ah/api/"
)

var (
	errMethodNotAllowed = endpoints.NewAPIError(http.StatusText(http.StatusMethodNotAllowed),
		"", http.StatusMethodNotAllowed)
)

type handlerFuncWithError func(http.ResponseWriter, *http.Request) error

// endpoint is a resolved backend endpoint.
type endpoint struct {
	// desc is the resolved service descriptor.
	desc *endpoints.APIDescriptor
	// method is the resolved method within the service.
	method *endpoints.APIMethod
	// backendPath is the backend path of this method.
	backendPath string
}

type endpointNode struct {
	// parent points to this endpointNode's parent node. If nil, this is the
	// root endpoint node.
	parent *endpointNode

	// component is the path component's value.
	//
	// If this is an empty string, this is a query parameter component.
	component string
	// paramName is the query parameter name for this node. It is valid if
	// component is "".
	paramName string
	// ep is the endpoint attached to this node. It can be nil if no endpoint is
	// attached.
	ep *endpoint

	children map[string]*endpointNode
}

func (n *endpointNode) addChild(components []string, ep *endpoint) error {
	comp := components[0]
	paramName := ""

	if strings.HasPrefix(comp, "{") && strings.HasSuffix(comp, "}") {
		// Replace query parameter with empty string.
		paramName = comp[1 : len(comp)-1]
		comp = ""
	}

	child := n.children[comp]
	if child == nil {
		if n.children == nil {
			n.children = map[string]*endpointNode{}
		}

		child = &endpointNode{
			parent:    n,
			component: comp,
			paramName: paramName,
		}
		n.children[child.component] = child
	}

	if len(components) > 1 {
		return child.addChild(components[1:], ep)
	}

	child.ep = ep
	return nil
}

func (n *endpointNode) lookup(components ...string) *endpointNode {
	if len(components) == 0 {
		if n.ep == nil {
			// Not a leaf node!
			return nil
		}
		return n
	}

	// Is this node parent to a query parameter?
	if child := n.children[""]; child != nil {
		// Assume that this component is a query parameter. Can we resolve against
		// the remainder of the tree?
		if cn := child.lookup(components[1:]...); cn != nil {
			return cn
		}
	}

	// Use this node as a direct value.
	if child := n.children[components[0]]; child != nil {
		return child.lookup(components[1:]...)
	}
	return nil
}

// serviceCall is a stateful structure that will be populated with the service
// call's parameters as they are resolved.
type serviceCall struct {
	// path is the initial service call path.
	path string

	// ep is the resolved endpoint description for this call.
	ep *endpoint
	// params is the map of key/value query parameters populated during
	// resolution.
	params map[string]interface{}
}

func (c *serviceCall) addParam(k string, v interface{}) {
	if c.params == nil {
		c.params = map[string]interface{}{}
	}
	c.params[k] = v
}

// Server is an HTTP handler
type Server struct {
	// Logger is a function that is called to obtain a logger for the current
	// http.Request. Logger will be called with a nil http.Request when a logger
	// is needed outside of the scope of request handling.
	//
	// If Logger is nil, or if Logger returns nil, a logging.Null logger will be
	// used.
	Logger func(*http.Request) logging.Logger

	// root is the root path of this Server.
	root string
	// backend is the endpoints Server instance to bind to.
	backend http.Handler
	// logger is the resolved no-context logger.
	logger logging.Logger
	// endpoints is a map of method to endpoint path component tree.
	endpoints map[string]*endpointNode
	// services is a map of registered services.
	services map[string]*endpoints.APIDescriptor
	// paths is a map of direct paths and their handler functions.
	paths map[string]handlerFuncWithError
}

// New instantiates a new Server instance.
//
// If root is the empty string, DefaultServerRoot will be used.
// If backend is nil, endpoints.DefaultServer will be used.
func New(root string, backend http.Handler) *Server {
	if root == "" {
		root = DefaultServerRoot
	}
	if backend == nil {
		backend = endpoints.DefaultServer
	}

	s := &Server{
		root:    root,
		backend: backend,
	}
	s.logger = s.getLogger(nil)

	s.registerPath(safeURLPathJoin("discovery", "v1", "apis"), s.serveGetJSON(s.handleDirectoryList))
	s.registerPath(safeURLPathJoin("explorer"), func(w http.ResponseWriter, r *http.Request) error {
		http.Redirect(w, r, fmt.Sprintf("http://apis-explorer.appspot.com/apis-explorer/?base=%s",
			safeURLPathJoin(getHostURL(r).String(), s.root)), http.StatusFound)
		return nil
	})
	return s
}

// RegisterService registers an endpoints service's descriptor with the Server.
func (s *Server) RegisterService(svc *endpoints.RPCService) error {
	desc := endpoints.APIDescriptor{}
	if err := svc.APIDescriptor(&desc, "epfrontend"); err != nil {
		return err
	}
	if _, ok := s.services[desc.Name]; ok {
		return fmt.Errorf("service [%s] already registered", desc.Name)
	}
	if s.services == nil {
		s.services = map[string]*endpoints.APIDescriptor{}
	}
	s.services[desc.Name] = &desc

	// Traverse the method map.
	apibase := []string{desc.Name, desc.Version}
	for _, m := range desc.Methods {
		// Build our API path: service/version / method/path/components
		parts := strings.Split(m.Path, "/")
		apipath := make([]string, 0, len(apibase)+len(parts))
		apipath = append(apipath, apibase...)
		apipath = append(apipath, parts...)

		ep := endpoint{
			desc:        &desc,
			method:      m,
			backendPath: m.RosyMethod,
		}
		if err := s.registerEndpoint(m.HTTPMethod, apipath, &ep); err != nil {
			return err
		}
	}

	s.registerPath(safeURLPathJoin("discovery", "v1", "apis", desc.Name, desc.Version, "rest"),
		s.serveGetJSON(func(r *http.Request) (interface{}, error) {
			return s.handleRestDescription(r, &desc)
		}))
	return nil
}

func (s *Server) registerEndpoint(method string, apipath []string, ep *endpoint) error {
	s.logger.Debugf("Registering %s %s => %#v", method, apipath, ep)
	if s.endpoints == nil {
		s.endpoints = map[string]*endpointNode{}
	}

	methodNode := s.endpoints[method]
	if methodNode == nil {
		methodNode = &endpointNode{}
		s.endpoints[method] = methodNode
	}

	if n := methodNode.lookup(apipath...); n != nil {
		return fmt.Errorf("%s method [%s] is already registered", method, apipath)
	}
	return methodNode.addChild(apipath, ep)
}

func (s *Server) registerPath(path string, handler handlerFuncWithError) {
	if s.paths == nil {
		s.paths = map[string]handlerFuncWithError{}
	}
	s.paths[path] = handler
}

// HandleHTTP adds Server s to specified http.ServeMux.
// If no mux is provided, http.DefaultServeMux will be used.
func (s *Server) HandleHTTP(mux *http.ServeMux) {
	if mux == nil {
		mux = http.DefaultServeMux
	}
	mux.Handle(s.root, s)
}

// ServeHTTP is the Server's implementation of the http.Handler interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := requestHandler{
		Server: s,
		logger: s.getLogger(r),
	}
	h.handle(w, r)
}

// serveGetJSON returns an HTTP handler that serves the marshalled JSON form of
// the specified object.
func (s *Server) serveGetJSON(f func(*http.Request) (interface{}, error)) handlerFuncWithError {
	return func(w http.ResponseWriter, req *http.Request) error {
		if req.Method != "GET" {
			return errMethodNotAllowed
		}

		i, err := f(req)
		if err != nil {
			return err
		}

		data, err := json.MarshalIndent(i, "", " ")
		if err != nil {
			return err
		}

		w.Header().Add("Content-Type", "application/json")
		_, err = w.Write(data)
		return err
	}
}

func (s *Server) getLogger(r *http.Request) (l logging.Logger) {
	if s.Logger != nil {
		l = s.Logger(r)
	}
	if l == nil {
		l = logging.Null()
	}
	return
}

type requestHandler struct {
	*Server

	// logger shadows Server.logger and is bound to an http.Request.
	logger logging.Logger
}

func (h *requestHandler) handle(w http.ResponseWriter, r *http.Request) {
	var err error
	iw := &errorResponseWriter{ResponseWriter: w}
	defer func() {
		if err != nil {
			iw.setError(err)
		}
		iw.forwardError()
	}()

	paniccatcher.Do(func() {
		err = h.handleImpl(iw, r)
	}, func(p *paniccatcher.Panic) {
		h.logger.Errorf("Panic during endpoint handling: %s\n%s", p.Reason, p.Stack)
		err = endpoints.InternalServerError
	})
}

func (h *requestHandler) handleImpl(w http.ResponseWriter, r *http.Request) error {
	if r.Body != nil {
		defer r.Body.Close()
	}

	// Strip our root from the request path, leaving the endpoint path.
	if !strings.HasPrefix(r.URL.Path, h.root) {
		h.logger.Debugf("Path (%s) does not begin with root (%s)", r.URL.Path, h.root)
		return endpoints.NotFoundError
	}
	path := strings.TrimPrefix(r.URL.Path, h.root)
	h.logger.Debugf("Received HTTP %s request [%s]: [%s]", r.Method, r.URL.Path, path)

	// If we have an explicit path handler set up for this path, use it.
	if ph := h.paths[path]; ph != nil {
		return ph(w, r)
	}

	// Resolve path to a service call.
	call, err := h.resolveRequest(r.Method, path)
	if err != nil {
		h.logger.Warningf("Could not resolve %s request [%s] to a backend endpoint: %v",
			r.Method, path, err)
		return err
	}
	// Add query strings to call parameters.
	for k, vs := range r.URL.Query() {
		// If there are multiple values, add the last.
		if len(vs) > 0 {
			param := vs[len(vs)-1]
			v, err := convertQueryParam(k, param, call.ep.method)
			if err != nil {
				h.logger.Errorf("Could not convert query param %q (%q): %v", k, param, err)
				return endpoints.BadRequestError
			}
			call.addParam(k, v)
		}
	}
	h.logger.Debugf("Resolved %s request [%s] to endpoint [%s].", r.Method, path, call.ep.backendPath)

	// Read the body for forwarding.
	buf := bytes.Buffer{}
	if _, err := buf.ReadFrom(r.Body); err != nil {
		h.logger.Errorf("failed to read request body: %v", err)
		return endpoints.InternalServerError
	}

	// Replace the body with a JSON object, if necessary.
	if buf.Len() == 0 || len(call.params) > 0 {
		data := map[string]*jsonRWRawMessage{}
		if buf.Len() > 0 {
			if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
				h.logger.Errorf("Failed to unmarshal request JSON: %v", err)
				return endpoints.BadRequestError
			}
		}

		for k, v := range call.params {
			data[k] = &jsonRWRawMessage{
				v: v,
			}
		}

		buf.Reset()
		m := json.NewEncoder(&buf)
		if err := m.Encode(data); err != nil {
			h.logger.Errorf("Failed to re-marshal request JSON: %v", err)
			return endpoints.InternalServerError
		}
	}
	r.Body = ioutil.NopCloser(&buf)

	// Mutate our request to pretend it's a frontend-to-backend request, then
	// forward it to the backend for actual API processing!
	//
	// The actual Path prefix doesn't matter, as the service only looks at the
	// last component to derive the service call.
	r.Method = "POST"
	r.RequestURI = "/_ah/spi/" + call.ep.backendPath // Strips query parameters.
	r.URL.Path = r.RequestURI
	h.backend.ServeHTTP(w, r)
	return nil
}

func (h *requestHandler) resolveRequest(method, path string) (*serviceCall, error) {
	call := serviceCall{
		path: path,
	}

	parts := strings.Split(path, "/")

	h.logger.Debugf("Resolving %s request %s...", method, parts)
	n := h.endpoints[method]
	if n != nil {
		n = n.lookup(parts...)
	}
	if n == nil {
		return nil, endpoints.NotFoundError
	}
	ep := n.ep
	call.ep = ep

	// Build our query parameters by reverse-traversing the tree towards its root.
	//
	// The endpoints node path will have the same size as parts, since it matched.
	for len(parts) > 0 {
		if n.paramName != "" {
			param := parts[len(parts)-1]
			v, err := convertQueryParam(n.paramName, param, ep.method)
			if err != nil {
				h.logger.Errorf("Could not convert path param %q (%q): %v", n.paramName, param, err)
				return nil, endpoints.BadRequestError
			}
			call.addParam(n.paramName, v)
		}

		parts = parts[:len(parts)-1]
		n = n.parent
	}

	return &call, nil
}

func convertQueryParam(k, v string, m *endpoints.APIMethod) (interface{}, error) {
	// Find the parameter for "k".
	p := m.Request.Params[k]
	if p == nil {
		return v, nil
	}

	switch p.Type {
	case "int64", "uint64":
		// Assume all int64 are "string"-quoted integers. If there are quotes, strip
		// them. Return the result as a string.
		if len(v) >= 2 && strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
			v = v[1 : len(v)-1]
		}
		if _, err := strconv.ParseInt(v, 10, 64); err != nil {
			return nil, err
		}
		return v, nil
	case "int32", "uint32":
		return strconv.ParseInt(v, 10, 32)
	case "float", "double":
		return strconv.ParseFloat(v, 64)
	case "boolean":
		switch v {
		case "", "false":
			return false, nil
		default:
			return true, nil
		}
	case "string", "bytes":
		return v, nil

	default:
		return nil, fmt.Errorf("unknown type %q", p.Type)
	}
}
