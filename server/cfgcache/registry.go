// Copyright 2026 The LUCI Authors.
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

package cfgcache

import (
	"cmp"
	"context"
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/config/vars"
)

// Registry holds registered [ServicePipeline] and [ProjectPipeline]
// definitions.
type Registry struct {
	mu                   sync.Mutex
	errs                 []error
	finishedRegistration bool
	initialized          bool
	ingestConfigs        map[string]*ingestConfig
}

// register records `ingestConfig` into this Registry.
//
// Panics on a nil registry, but otherwise buffers all errors until
// register adds ic to r, buffering any structural validation errors until
// [Registry.Initialize] is called.
func register[StoreMsg proto.Message](r *Registry, files []File, xfrm TransformFunc[StoreMsg], store Store[StoreMsg], ic *ingestConfig) {
	if r == nil {
		panic("cannot call Register with a nil Registry")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		panic("cannot call Register after Registry.Initialize")
	}
	if r.finishedRegistration {
		panic("cannot call Register after Module.Dependencies() have been gathered")
	}

	// Do all the common validation stuff between ServicePipeline and ProjectPipeline.
	err := func() (err error) {
		if ic.id == "" {
			return errors.New("ID must not be empty")
		}
		if _, ok := r.ingestConfigs[ic.id]; ok {
			return errors.Fmt("duplicate ID %q", ic.id)
		}
		if ic.transformVersion < 0 {
			return errors.New("TransformVersion must not be negative")
		}
		if len(files) == 0 {
			return errors.New("Files must not be empty")
		}

		ic.files = make([]*resolvedFile, len(files))
		for i, f := range files {
			if f == nil {
				return errors.Fmt("Files[%d]: must not be nil", i)
			}
			if f.pattern() == "" {
				if f.isRegex() {
					return errors.Fmt("Files[%d]: PathRegex must not be empty", i)
				}
				return errors.Fmt("Files[%d]: Path must not be empty", i)
			}
			unmarshal, err := f.unmarshalFunc()
			if err != nil {
				return errors.Fmt("Files[%d]: %w", i, err)
			}

			ic.files[i] = &resolvedFile{File: f, unmarshal: unmarshal}
		}

		var singleStoreMsg *SingleFile[StoreMsg]
		if len(files) == 1 {
			singleStoreMsg, _ = files[0].(*SingleFile[StoreMsg])
		}

		if singleStoreMsg == nil && ic.transformVersion == 0 {
			return errors.New("TransformVersion must be non-zero unless Files has a single SingleFile[StoreMsg]")
		}

		switch {
		case xfrm != nil:
			ic.transformFunc = func(c *validation.Context, snap *Snapshot) (proto.Message, error) {
				return xfrm(c, snap)
			}

		case singleStoreMsg != nil:
			ic.transformFunc = func(c *validation.Context, snap *Snapshot) (proto.Message, error) {
				return singleStoreMsg.Get(snap), nil
			}

		default:
			return errors.New("Transform must be set unless Files has a single SingleFile[StoreMsg]")
		}

		if store == nil {
			return errors.New("Storage is required")
		}

		ic.store = storeAdapter[StoreMsg]{store}
		return nil
	}()
	if err != nil {
		r.errs = append(r.errs, errors.Fmt("%s.Register(): %w", ic, err))
		return
	}
	if r.ingestConfigs == nil {
		r.ingestConfigs = make(map[string]*ingestConfig)
	}
	r.ingestConfigs[ic.id] = ic
}

func (r *Registry) finishRegistration() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finishedRegistration {
		panic(errors.Fmt("cannot finishRegistration more than once."))
	}
	r.finishedRegistration = true
}

// Initialize freezes registration and resolves template variables for all
// registered configs using v (or &vars.Vars if v is nil).
func (r *Registry) Initialize(ctx context.Context, v *vars.VarSet) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishedRegistration = true
	if err := errors.Join(r.errs...); err != nil {
		r.initialized = true
		return err
	}
	if r.initialized {
		return errors.New("registry already initialized")
	}
	r.initialized = true

	defer func() {
		if err != nil {
			r.errs = append(r.errs, err)
		}
	}()
	v = cmp.Or(v, &vars.Vars)
	for _, ingest := range r.ingestConfigs {
		if ingest.serviceNamePattern != "" {
			var err error
			ingest.serviceName, err = v.RenderTemplate(ctx, ingest.serviceNamePattern)
			if err != nil {
				return errors.Fmt("%s.Register(): bad serviceNamePattern: %w", ingest, err)
			}
			if _, err := config.ServiceSet(ingest.serviceName); err != nil {
				return errors.Fmt("%s.Register(): bad ServiceName: %w", ingest, err)
			}
		}

		for i, rf := range ingest.files {
			rendered, err := v.RenderTemplate(ctx, rf.pattern())
			if err != nil {
				return errors.Fmt("%s.Register(): Files[%d]: bad path template: %w", ingest, i, err)
			}
			if rf.isRegex() {
				pat, err := pattern.Parse("regex:" + rendered)
				if err != nil {
					return errors.Fmt("%s.Register(): Files[%d]: bad PathRegex %q: %w", ingest, i, rendered, err)
				}
				rf.pat = pat
			} else {
				rf.path = rendered
				rf.pat = pattern.Exact(rendered)
			}
		}
	}
	return nil
}

// InstallValidationRules adds all registered configs in r to rules (or
// &validation.Rules if rules is nil).
//
// Must be called after [Registry.Initialize].
func (r *Registry) InstallValidationRules(rules *validation.RuleSet) {
	r.checkInitialized()
	rules = cmp.Or(rules, &validation.Rules)
	for _, ingest := range r.ingestConfigs {
		var cs string
		if ingest.serviceName != "" {
			cs = fmt.Sprintf("exact:%s/%s", config.ServiceDomain, ingest.serviceName)
		} else {
			cs = fmt.Sprintf("regex:%s/.*", config.ProjectDomain)
		}

		paths := make([]string, len(ingest.files))
		for i, rf := range ingest.files {
			paths[i] = rf.pat.String()
		}

		rules.AddMulti(cs, paths, func(ctx *validation.Context, configSet string, files map[string][]byte) error {
			_, err := ingest.validateAndTransform(ctx, config.Set(configSet), files)
			return err
		})
	}
}

func (r *Registry) checkInitialized() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.initialized {
		panic("registry not initialized")
	}
	if err := errors.Join(r.errs...); err != nil {
		panic(errors.Fmt("registry failed to initialize: %w", err))
	}
}
