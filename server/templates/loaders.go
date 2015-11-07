// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package templates

import (
	"html/template"
	"strings"

	"golang.org/x/net/context"
)

// AssetsLoader returns Loader that loads templates from the given assets map.
//
// The map is expected to have special structure:
//   * 'pages/' contain all top-level templates that will be loaded.
//   * 'includes/' contain templates that will be associated with every
//      top-level template from 'pages/'.
//
// Only templates from 'pages/' are included in the output map.
func AssetsLoader(assets map[string]string) Loader {
	return func(c context.Context, funcMap template.FuncMap) (map[string]*template.Template, error) {
		// Pick all includes.
		includes := []string(nil)
		for name, body := range assets {
			if strings.HasPrefix(name, "includes/") {
				includes = append(includes, body)
			}
		}

		// Parse all top level templates and associate them with all includes.
		toplevel := map[string]*template.Template{}
		for name, body := range assets {
			if strings.HasPrefix(name, "pages/") {
				t, err := template.New(name).Funcs(funcMap).Parse(body)
				if err != nil {
					return nil, err
				}
				// TODO(vadimsh): There's probably a way to avoid reparsing includes
				// all the time.
				for _, includeSrc := range includes {
					if _, err := t.Parse(includeSrc); err != nil {
						return nil, err
					}
				}
				toplevel[name] = t
			}
		}

		return toplevel, nil
	}
}
