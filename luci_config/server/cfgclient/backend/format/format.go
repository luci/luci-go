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

// Package format implements a config client Backend that performs formatting
// on items.
//
// The available formats are registered during init() time via 'Register'.
package format

import (
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend"

	"golang.org/x/net/context"
)

// Backend is a backend.B implementation that applies Formatter
// transformations to the various Items that pass through it.
//
// Formatter transformations are registered explicitly to the Resolver
// descriptions of the types that they operate on. If an Item is already
// formatted, no further transformations will be applied.
type Backend struct {
	// B is the underlying Backend that this Backend will pull data
	// from.
	backend.B
}

// Get implements backend.B.
func (b *Backend) Get(c context.Context, configSet, path string, p backend.Params) (*backend.Item, error) {
	item, err := b.B.Get(c, configSet, path, p)
	if err != nil {
		return nil, err
	}

	if !p.FormatSpec.Unformatted() {
		formatter, err := getFormatter(p.FormatSpec.Formatter)
		if err != nil {
			return nil, errors.Annotate(err, "failed to get formatter for %q", p.FormatSpec).Err()
		}

		if err := b.formatItem(item, formatter, p.FormatSpec); err != nil {
			return nil, errors.Annotate(err, "failed to format item to %q, data %q",
				p.FormatSpec.Formatter, p.FormatSpec.Data).Err()
		}
	}
	return item, nil
}

// GetAll implements backend.B.
func (b *Backend) GetAll(c context.Context, t backend.GetAllTarget, path string, p backend.Params) (
	[]*backend.Item, error) {

	items, err := b.B.GetAll(c, t, path, p)
	if err != nil {
		return nil, err
	}

	if !p.FormatSpec.Unformatted() {
		formatter, err := getFormatter(p.FormatSpec.Formatter)
		if err != nil {
			return nil, errors.Annotate(err, "failed to get formatter for %q, data %q",
				p.FormatSpec.Formatter, p.FormatSpec.Data).Err()
		}

		lme := errors.NewLazyMultiError(len(items))
		for i, item := range items {
			if err := b.formatItem(item, formatter, p.FormatSpec); err != nil {
				lme.Assign(i, err)
			}
		}

		if err := lme.Get(); err != nil {
			return nil, errors.Annotate(err, "failed to format items to %q, data %q",
				p.FormatSpec.Formatter, p.FormatSpec.Data).Err()
		}
	}
	return items, nil
}

func (b *Backend) formatItem(it *backend.Item, formatter Formatter, fs backend.FormatSpec) error {
	if !it.FormatSpec.Unformatted() {
		// Item is already formatted.
		return nil
	}

	// Item is not formatted, so format it.
	content, err := formatter.FormatItem(it.Content, fs.Data)
	if err != nil {
		return errors.Annotate(err, "failed to format item").Err()
	}
	it.Content = content
	it.FormatSpec = fs
	return nil
}
