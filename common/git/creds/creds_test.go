// Copyright 2025 The LUCI Authors.
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

package creds

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestReadAttrs(t *testing.T) {
	t.Parallel()
	input := strings.NewReader(`host=example.com
path=foo.git
capability[]=authtype
protocol=https
nonexistence=you and me
`)
	got, err := ReadAttrs(input)
	if err != nil {
		t.Error(err)
	}
	want := &Attrs{
		Protocol:     "https",
		Host:         "example.com",
		Path:         "foo.git",
		Capabilities: []string{"authtype"},
	}
	assert.That(t, got, should.Match(want))
}

func TestReadAttrs_multivalue(t *testing.T) {
	t.Parallel()
	input := strings.NewReader(`capability[]=authtype
capability[]=state
`)
	got, err := ReadAttrs(input)
	if err != nil {
		t.Error(err)
	}
	want := &Attrs{
		Capabilities: []string{"authtype", "state"},
	}
	assert.That(t, got, should.Match(want))
}

func TestReadAttrs_multivalue_reset(t *testing.T) {
	t.Parallel()
	input := strings.NewReader(`capability[]=authtype
capability[]=
capability[]=state
`)
	got, err := ReadAttrs(input)
	if err != nil {
		t.Error(err)
	}
	want := &Attrs{
		Capabilities: []string{"state"},
	}
	assert.That(t, got, should.Match(want))
}
