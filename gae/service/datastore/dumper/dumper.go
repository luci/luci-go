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

// Package dumper implements a very VERY dumb datastore-dumping debugging aid.
// You shouldn't plan on having this work with the production datastore with any
// appreciable amount of data.
//
// This will take an arbitrary query (or even a query for every entity in the
// entire datastore), and print every entity to some output stream.
package dumper

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	ds "go.chromium.org/gae/service/datastore"

	"golang.org/x/net/context"
)

// Key is a key into a PropFilterMap
type Key struct {
	Kind     string
	PropName string
}

// A PropFilterMap maps from Kind+PropertyName tuples to a formatting function. You
// may use this to specially format particular properties.
type PropFilterMap map[Key]func(ds.Property) string

// KindFilterMap maps from a Kind to a formatting function. You may use this to
// specially format particular Kinds. If this function returns an empty string,
// the default formatting function (including any PropFilterMap entries) will be
// used.
type KindFilterMap map[string]func(*ds.Key, ds.PropertyMap) string

// Config is a configured dumper.
type Config struct {
	// OutStream is the output stream to use. If this is nil, os.Stdout will be
	// used.
	OutStream io.Writer

	// WithSpecial, if true, includes entities which have kinds that begin and
	// end with "__". By default, these entities are skipped.
	WithSpecial bool

	// PropFilters is an optional property filter map for controlling the
	// rendering of certain Kind/Property values.
	PropFilters PropFilterMap

	// KindFilters is an optional kind filter for controlling the rendering of
	// certain Kind values.
	KindFilters KindFilterMap
}

// Query will dump everything matching the provided query.
//
// If the provided query is nil, a kindless query without any filters will be
// used.
func (cfg Config) Query(c context.Context, q *ds.Query) (n int, err error) {
	if q == nil {
		q = ds.NewQuery("")
	}

	out := cfg.OutStream
	if out == nil {
		out = os.Stdout
	}

	fmtVal := func(kind, name string, prop ds.Property) string {
		if fn := cfg.PropFilters[Key{kind, name}]; fn != nil {
			return fn(prop)
		}
		return prop.String()
	}

	prnt := func(format string, args ...interface{}) (err error) {
		var amt int
		amt, err = fmt.Fprintf(out, format, args...)
		n += amt
		return
	}

	prop := func(kind, name string, pdata ds.PropertyData) (err error) {
		switch t := pdata.(type) {
		case ds.Property:
			return prnt("  %q: %s\n", name, fmtVal(kind, name, t))

		case ds.PropertySlice:
			if len(t) <= 1 {
				return prnt("  %q: [%s]\n", name, fmtVal(kind, name, t[0]))
			}
			if err = prnt("  %q: [\n    %s", name, fmtVal(kind, name, t[0])); err != nil {
				return
			}
			for _, v := range t[1:] {
				if err = prnt(",\n    %s", fmtVal(kind, name, v)); err != nil {
					return
				}
			}

		default:
			return fmt.Errorf("unknown PropertyData %T", t)
		}
		return prnt("\n  ]\n")
	}

	err = ds.Run(c, q, func(pm ds.PropertyMap) error {
		key := ds.GetMetaDefault(pm, "key", nil).(*ds.Key)
		if !cfg.WithSpecial && strings.HasPrefix(key.Kind(), "__") && strings.HasSuffix(key.Kind(), "__") {
			return nil
		}
		if err := prnt("\n%s:\n", key); err != nil {
			return err
		}
		pm, _ = pm.Save(false)

		// See if we have a KindFilter for this
		if flt, ok := cfg.KindFilters[key.Kind()]; ok {
			if kindOut := flt(key, pm); kindOut != "" {
				for _, l := range strings.Split(kindOut, "\n") {
					if err := prnt("  %s\n", l); err != nil {
						return err
					}
				}
				return nil
			}
		}

		keys := make([]string, 0, len(pm))
		for k := range pm {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := prop(key.Kind(), k, pm[k]); err != nil {
				return err
			}
		}
		return nil
	})
	return
}

// Query dumps the provided query to stdout without special entities and with
// default rendering.
func Query(c context.Context, q *ds.Query) {
	Config{}.Query(c, q)
}

// All dumps all entities to stdout without special entities and with default
// rendering.
func All(c context.Context) {
	Config{}.Query(c, nil)
}
