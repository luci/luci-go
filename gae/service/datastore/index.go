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

package datastore

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"
)

// IndexColumn represents a sort order for a single entity field.
type IndexColumn struct {
	Property   string
	Descending bool
}

// ParseIndexColumn takes a spec in the form of /\s*-?\s*.+\s*/, and
// returns an IndexColumn. Examples are:
//   `- Field `:  IndexColumn{Property: "Field", Descending: true}
//   `Something`: IndexColumn{Property: "Something", Descending: false}
//
// `+Field` is invalid. `` is invalid.
func ParseIndexColumn(spec string) (IndexColumn, error) {
	col := IndexColumn{}
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "-") {
		col.Descending = true
		col.Property = strings.TrimSpace(spec[1:])
	} else if strings.HasPrefix(spec, "+") {
		return col, fmt.Errorf("datastore: invalid order: %q", spec)
	} else {
		col.Property = strings.TrimSpace(spec)
	}
	if col.Property == "" {
		return col, fmt.Errorf("datastore: empty order: %q", spec)
	}
	return col, nil
}

func (i IndexColumn) cmp(o IndexColumn) int {
	// sort ascending first
	if !i.Descending && o.Descending {
		return -1
	} else if i.Descending && !o.Descending {
		return 1
	}
	return cmpString(i.Property, o.Property)()
}

// UnmarshalYAML deserializes a index.yml `property` into an IndexColumn.
func (i *IndexColumn) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var m map[string]string
	if err := unmarshal(&m); err != nil {
		return err
	}

	name, ok := m["name"]
	if !ok {
		return fmt.Errorf("datastore: missing required key `name`: %v", m)
	}
	i.Property = name

	i.Descending = false // default direction is "asc"
	if v, ok := m["direction"]; ok && v == "desc" {
		i.Descending = true
	}

	return nil
}

// MarshalYAML serializes an IndexColumn into a index.yml `property`.
func (i *IndexColumn) MarshalYAML() (interface{}, error) {
	direction := "asc"

	if i.Descending {
		direction = "desc"
	}

	return yaml.Marshal(map[string]string{
		"name":      i.Property,
		"direction": direction,
	})
}

// String returns a human-readable version of this IndexColumn which is
// compatible with ParseIndexColumn.
func (i IndexColumn) String() string {
	ret := ""
	if i.Descending {
		ret = "-"
	}
	return ret + i.Property
}

// GQL returns a correctly formatted Cloud Datastore GQL literal which
// is valid for the `ORDER BY` clause.
//
// The flavor of GQL that this emits is defined here:
//   https://cloud.google.com/datastore/docs/apis/gql/gql_reference
func (i IndexColumn) GQL() string {
	if i.Descending {
		return gqlQuoteName(i.Property) + " DESC"
	}
	return gqlQuoteName(i.Property)
}

// IndexDefinition holds the parsed definition of a datastore index definition.
type IndexDefinition struct {
	Kind     string        `yaml:"kind"`
	Ancestor bool          `yaml:"ancestor"`
	SortBy   []IndexColumn `yaml:"properties"`
}

// MarshalYAML serializes an IndexDefinition into a index.yml `index`.
func (id *IndexDefinition) MarshalYAML() (interface{}, error) {
	if id.Builtin() || !id.Compound() {
		return nil, fmt.Errorf("cannot generate YAML for %s", id)
	}

	return yaml.Marshal(map[string]interface{}{
		"kind":       id.Kind,
		"ancestor":   id.Ancestor,
		"properties": id.SortBy,
	})
}

// Equal returns true if the two IndexDefinitions are equivalent.
func (id *IndexDefinition) Equal(o *IndexDefinition) bool {
	if id.Kind != o.Kind || id.Ancestor != o.Ancestor || len(id.SortBy) != len(o.SortBy) {
		return false
	}
	for i, col := range id.SortBy {
		if col != o.SortBy[i] {
			return false
		}
	}
	return true
}

// Normalize returns an IndexDefinition which has a normalized SortBy field.
//
// This is just appending __key__ if it's not explicitly the last field in this
// IndexDefinition.
func (id *IndexDefinition) Normalize() *IndexDefinition {
	if len(id.SortBy) > 0 && id.SortBy[len(id.SortBy)-1].Property == "__key__" {
		return id
	}
	ret := *id
	ret.SortBy = make([]IndexColumn, len(id.SortBy), len(id.SortBy)+1)
	copy(ret.SortBy, id.SortBy)
	ret.SortBy = append(ret.SortBy, IndexColumn{Property: "__key__"})
	return &ret
}

// GetFullSortOrder gets the full sort order for this IndexDefinition,
// including an extra "__ancestor__" column at the front if this index has
// Ancestor set to true.
func (id *IndexDefinition) GetFullSortOrder() []IndexColumn {
	id = id.Normalize()
	if !id.Ancestor {
		return id.SortBy
	}
	ret := make([]IndexColumn, 0, len(id.SortBy)+1)
	ret = append(ret, IndexColumn{Property: "__ancestor__"})
	return append(ret, id.SortBy...)
}

// PrepForIdxTable normalize and then flips the IndexDefinition.
func (id *IndexDefinition) PrepForIdxTable() *IndexDefinition {
	return id.Normalize().Flip()
}

// Flip returns an IndexDefinition with its SortBy field in reverse order.
func (id *IndexDefinition) Flip() *IndexDefinition {
	ret := *id
	ret.SortBy = make([]IndexColumn, 0, len(id.SortBy))
	for i := len(id.SortBy) - 1; i >= 0; i-- {
		ret.SortBy = append(ret.SortBy, id.SortBy[i])
	}
	return &ret
}

// Yeah who needs templates, right?
// <flames>This is fine.</flames>

func cmpBool(a, b bool) func() int {
	return func() int {
		if a == b {
			return 0
		}
		if a && !b { // >
			return 1
		}
		return -1
	}
}

func cmpInt(a, b int) func() int {
	return func() int {
		if a == b {
			return 0
		}
		if a > b {
			return 1
		}
		return -1
	}
}

func cmpString(a, b string) func() int {
	return func() int {
		if a == b {
			return 0
		}
		if a > b {
			return 1
		}
		return -1
	}
}

// Less returns true iff id is ordered before o.
func (id *IndexDefinition) Less(o *IndexDefinition) bool {
	decide := func(v int) (ret, keepGoing bool) {
		if v > 0 {
			return false, false
		}
		if v < 0 {
			return true, false
		}
		return false, true
	}

	factors := []func() int{
		cmpBool(id.Builtin(), o.Builtin()),
		cmpString(id.Kind, o.Kind),
		cmpBool(id.Ancestor, o.Ancestor),
		cmpInt(len(id.SortBy), len(o.SortBy)),
	}
	for _, f := range factors {
		ret, keepGoing := decide(f())
		if !keepGoing {
			return ret
		}
	}
	for idx := range id.SortBy {
		ret, keepGoing := decide(id.SortBy[idx].cmp(o.SortBy[idx]))
		if !keepGoing {
			return ret
		}
	}
	return false
}

// Builtin returns true iff the IndexDefinition is one of the automatic built-in
// indexes.
func (id *IndexDefinition) Builtin() bool {
	return !id.Ancestor && len(id.SortBy) <= 1
}

// Compound returns true iff this IndexDefinition is a valid compound index
// definition.
//
// NOTE: !Builtin() does not imply Compound().
func (id *IndexDefinition) Compound() bool {
	if id.Kind == "" || id.Builtin() {
		return false
	}
	for _, sb := range id.SortBy {
		if sb.Property == "" || sb.Property == "__ancestor__" {
			return false
		}
	}
	return true
}

// YAMLString returns the YAML representation of this IndexDefinition.
//
// If the index definition is Builtin() or not Compound(), this will return
// an error.
func (id *IndexDefinition) YAMLString() (string, error) {
	if id.Builtin() || !id.Compound() {
		return "", fmt.Errorf("cannot generate YAML for %s", id)
	}

	ret := bytes.Buffer{}

	first := true
	ws := func(s string, indent int) {
		nl := "\n"
		if first {
			nl = ""
			first = false
		}
		fmt.Fprintf(&ret, "%s%s%s", nl, strings.Repeat("  ", indent), s)
	}

	ws(fmt.Sprintf("- kind: %s", id.Kind), 0)
	if id.Ancestor {
		ws("ancestor: yes", 1)
	}
	ws("properties:", 1)
	for _, o := range id.SortBy {
		ws(fmt.Sprintf("- name: %s", o.Property), 1)
		if o.Descending {
			ws("direction: desc", 2)
		}
	}
	return ret.String(), nil
}

func (id *IndexDefinition) String() string {
	ret := bytes.Buffer{}
	wr := func(r rune) {
		_, err := ret.WriteRune(r)
		if err != nil {
			panic(err)
		}
	}

	ws := func(s string) {
		_, err := ret.WriteString(s)
		if err != nil {
			panic(err)
		}
	}

	if id.Builtin() {
		wr('B')
	} else {
		wr('C')
	}
	wr(':')
	ws(id.Kind)
	if id.Ancestor {
		ws("|A")
	}
	for _, sb := range id.SortBy {
		wr('/')
		if sb.Descending {
			wr('-')
		}
		ws(sb.Property)
	}
	return ret.String()
}
