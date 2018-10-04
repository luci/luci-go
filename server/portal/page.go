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

package portal

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"sync"
)

// Page controls how some portal section (usually corresponding to a key in
// global settings JSON blob) is displayed and edited in UI.
//
// Packages that wishes to expose UI for managing their settings register a page
// via RegisterPage(...) call during init() time.
type Page interface {
	// Title is used in UI to name this page.
	Title(c context.Context) (string, error)

	// Overview is optional HTML paragraph describing this page.
	Overview(c context.Context) (template.HTML, error)

	// Fields describes the schema of the settings on the page (if any).
	Fields(c context.Context) ([]Field, error)

	// Actions is additional list of actions to present on the page.
	//
	// Each action is essentially a clickable button that triggers a parameterless
	// callback.
	Actions(c context.Context) ([]Action, error)

	// ReadSettings returns a map "field ID => field value to display".
	//
	// It is called when rendering the settings page.
	ReadSettings(c context.Context) (map[string]string, error)

	// WriteSettings saves settings described as a map "field ID => field value".
	//
	// All values are validated using field validators first.
	WriteSettings(c context.Context, values map[string]string, who, why string) error
}

// Field is description of a single UI element of the page.
//
// Its ID acts as a key in map used by ReadSettings\WriteSettings.
type Field struct {
	ID             string             // page unique ID
	Title          string             // human friendly name
	Type           FieldType          // how the field is displayed and behaves
	Placeholder    string             // optional placeholder value
	Validator      func(string) error // optional value validation
	Help           template.HTML      // optional help text
	ChoiceVariants []string           // valid only for FieldChoice
}

// FieldType describes look and feel of UI field, see the enum below.
type FieldType string

// Note: exact values here are important. They are referenced in the HTML
// template that renders the settings page. See server/portal/*.
const (
	FieldText     FieldType = "text"     // one line of text, editable
	FieldChoice   FieldType = "choice"   // pick one of predefined choices
	FieldStatic   FieldType = "static"   // one line of text, read only
	FieldPassword FieldType = "password" // one line of text, editable but obscured
)

// IsEditable returns true for fields that can be edited.
func (f FieldType) IsEditable() bool {
	return f != FieldStatic
}

// Action corresponds to a button that triggers a parameterless callback.
type Action struct {
	ID           string        // page-unique ID
	Title        string        // what's displayed on the button
	Help         template.HTML // optional help text
	Confirmation string        // optional text for "Are you sure?" confirmation prompt

	// Callback is executed on click.
	//
	// The context is derived through the base middleware in the router. The
	// return values is presented to the user as is.
	Callback func(c context.Context) (template.HTML, error)
}

// BasePage can be embedded into Page implementers to provide default
// behavior.
type BasePage struct{}

// Title is used in UI to name this portal page.
func (BasePage) Title(c context.Context) (string, error) {
	return "Untitled portal page", nil
}

// Overview is optional HTML paragraph describing this portal page.
func (BasePage) Overview(c context.Context) (template.HTML, error) {
	return "", nil
}

// Fields describes the schema of the settings on the page (if any).
func (BasePage) Fields(c context.Context) ([]Field, error) {
	return nil, nil
}

// Actions is additional list of actions to present on the page.
func (BasePage) Actions(c context.Context) ([]Action, error) {
	return nil, nil
}

// ReadSettings returns a map "field ID => field value to display".
func (BasePage) ReadSettings(c context.Context) (map[string]string, error) {
	return nil, nil
}

// WriteSettings saves settings described as a map "field ID => field value".
func (BasePage) WriteSettings(c context.Context, values map[string]string, who, why string) error {
	return errors.New("not implemented")
}

// RegisterPage makes exposes UI for a portal page (identified by given
// unique key).
//
// Should be called once when application starts (e.g. from init() of a package
// that defines the page). Panics if such key is already registered.
func RegisterPage(pageKey string, p Page) {
	registry.registerPage(pageKey, p)
}

// GetPages returns a map with all registered pages.
func GetPages() map[string]Page {
	return registry.getPages()
}

////////////////////////////////////////////////////////////////////////////////
// Internal stuff.

var registry pageRegistry

type pageRegistry struct {
	lock  sync.RWMutex
	pages map[string]Page
}

func (r *pageRegistry) registerPage(pageKey string, p Page) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.pages == nil {
		r.pages = make(map[string]Page)
	}
	if existing, _ := r.pages[pageKey]; existing != nil {
		panic(fmt.Errorf("portal page for %s is already registered: %T", pageKey, existing))
	}
	r.pages[pageKey] = p
}

func (r *pageRegistry) getPages() map[string]Page {
	r.lock.RLock()
	defer r.lock.RUnlock()
	cpy := make(map[string]Page, len(r.pages))
	for k, v := range r.pages {
		cpy[k] = v
	}
	return cpy
}
