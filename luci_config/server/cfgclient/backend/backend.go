// Copyright 2016 The LUCI Authors.
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

package backend

import (
	"net/url"

	"go.chromium.org/luci/common/config"

	"golang.org/x/net/context"
)

// FormatSpec is a specification for formatted data.
type FormatSpec struct {
	// Formatter is the supported destination Resolver format for this item.
	// Backends (notably the format.Backend) may project the Item into this
	// format.
	//
	// An empty string means the original config service format.
	Formatter string

	// Data is additional format data describing the type. It may be empty.
	Data string
}

// Unformatted retrns true if fs does not specify a format.
func (fs *FormatSpec) Unformatted() bool { return fs.Formatter == "" }

// Item is a single config item. It is used to pass configuration data
// between Backend instances.
type Item struct {
	Meta

	Content string

	// FormatSpec, if non-empty, qualifies the format of the Content.
	FormatSpec FormatSpec
}

// Params are parameters supplied to Backend methods. They are generated
// by the main interface user-facing methods (config.go)
type Params struct {
	// Authority is the authority to use in the request.
	Authority Authority
	// Content, if true, indicates that config content should also be fetched.
	// Otherwise, only the content hash needs to be returned.
	Content bool

	// FormatSpec, if non-empty, qualifies the format of the Content.
	FormatSpec FormatSpec
}

// B is a configuration backend interface.
type B interface {
	// ServiceURL returns the service URL.
	ServiceURL(context.Context) url.URL

	// Get retrieves a single configuration.
	Get(c context.Context, configSet, path string, p Params) (*Item, error)

	// GetAll retrieves all configurations of a given type.
	GetAll(c context.Context, t GetAllTarget, path string, p Params) ([]*Item, error)

	// ConfigSetURL returns the URL for the specified config set.
	ConfigSetURL(c context.Context, configSet string, p Params) (url.URL, error)

	// GetConfigInterface returns the raw configuration interface of the backend.
	GetConfigInterface(c context.Context, a Authority) config.Interface
}

// Factory is a function that generates a B given a Context.
type Factory func(context.Context) B

// configBackendKey is the Context key for the configuration backend.
var configBackendKey = "go.chromium.org/luci/server/config:backend"

// WithBackend returns a derivative Context with the supplied Backend installed.
func WithBackend(c context.Context, b B) context.Context {
	return WithFactory(c, func(context.Context) B { return b })
}

// WithFactory returns a derivative Context with the supplied BackendFactory
// installed.
func WithFactory(c context.Context, f Factory) context.Context {
	return context.WithValue(c, &configBackendKey, f)
}

// Get returns the Backend that is installed into the Context.
//
// If no Backend is installed in the Context, Get will panic.
func Get(c context.Context) B {
	if f, ok := c.Value(&configBackendKey).(Factory); ok {
		return f(c)
	}
	panic("no Backend factory is installed in the Context")
}
