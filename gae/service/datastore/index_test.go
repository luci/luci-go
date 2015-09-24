// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// adapted from github.com/golang/appengine/datastore

package datastore

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

var indexDefinitionTests = []struct {
	id       *IndexDefinition
	builtin  bool
	compound bool
	str      string
	yaml     []string
}{
	{
		id:      &IndexDefinition{Kind: "kind"},
		builtin: true,
		str:     "B:kind",
	},

	{
		id: &IndexDefinition{
			Kind:   "kind",
			SortBy: []IndexColumn{{Property: ""}},
		},
		builtin: true,
		str:     "B:kind/",
	},

	{
		id: &IndexDefinition{
			Kind:   "kind",
			SortBy: []IndexColumn{{Property: "prop"}},
		},
		builtin: true,
		str:     "B:kind/prop",
	},

	{
		id: &IndexDefinition{
			Kind:     "Kind",
			Ancestor: true,
			SortBy: []IndexColumn{
				{Property: "prop"},
				{Property: "other", Descending: true},
			},
		},
		compound: true,
		str:      "C:Kind|A/prop/-other",
		yaml: []string{
			"- kind: Kind",
			"  ancestor: yes",
			"  properties:",
			"  - name: prop",
			"  - name: other",
			"    direction: desc",
		},
	},
}

func TestIndexDefinition(t *testing.T) {
	t.Parallel()

	Convey("Test IndexDefinition", t, func() {
		for _, tc := range indexDefinitionTests {
			Convey(tc.str, func() {
				So(tc.id.String(), ShouldEqual, tc.str)
				So(tc.id.Builtin(), ShouldEqual, tc.builtin)
				So(tc.id.Compound(), ShouldEqual, tc.compound)
				yaml, _ := tc.id.YAMLString()
				So(yaml, ShouldEqual, strings.Join(tc.yaml, "\n"))
			})
		}
	})
}
