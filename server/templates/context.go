// Copyright 2015 The LUCI Authors.
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

package templates

import (
	"errors"
	"html/template"
	"io"

	"golang.org/x/net/context"
)

var (
	errNoBundle = errors.New("templates: context doesn't have templates.Bundle")
	contextKey  = "templates.Bundle"
)

// boundBundle is stored in the context by Use(...). It lives for a duration of
// one request and stores extra information about this request.
type boundBundle struct {
	*Bundle
	*Extra
}

// Use replaces the template bundle in the context. There can be only one bundle
// installed at any time.
//
// It also takes an Extra to be passed to bundle's DefaultArgs(...) callback
// when using Render(...) or MustRender(...) top-level functions.
//
// DefaultArgs(...) can use the context and the given Extra to extract
// information about the environment.
func Use(c context.Context, b *Bundle, e *Extra) context.Context {
	b.EnsureLoaded(c)
	return context.WithValue(c, &contextKey, &boundBundle{b, e})
}

// Get returns template from the currently loaded bundle or error if not found.
func Get(c context.Context, name string) (*template.Template, error) {
	if b, _ := c.Value(&contextKey).(*boundBundle); b != nil {
		return b.Get(name)
	}
	return nil, errNoBundle
}

// Render finds top level template with given name and calls its Execute or
// ExecuteTemplate method (depending on the value of Bundle.DefaultTemplate).
//
// It always renders output into a byte buffer, to avoid partial results in case
// of errors.
func Render(c context.Context, name string, args Args) ([]byte, error) {
	if b, _ := c.Value(&contextKey).(*boundBundle); b != nil {
		return b.Render(c, b.Extra, name, args)
	}
	return nil, errNoBundle
}

// MustRender renders the template into the output writer or panics.
//
// It never writes partial output. It also panics if attempt to write to
// the output fails.
func MustRender(c context.Context, out io.Writer, name string, args Args) {
	if b, _ := c.Value(&contextKey).(*boundBundle); b != nil {
		b.MustRender(c, b.Extra, out, name, args)
		return
	}
	panic(errNoBundle)
}
