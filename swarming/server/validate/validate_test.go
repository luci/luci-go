// Copyright 2024 The LUCI Authors.
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

package validate

import (
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDimensionKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dim string
		err any
	}{
		{"good", nil},
		{strings.Repeat("a", maxDimensionKeyLen), nil},
		{"", "cannot be empty"},
		{strings.Repeat("a", maxDimensionKeyLen+1), "should be no longer"},
		{"bad key", "should match"},
	}

	for _, cs := range cases {
		t.Run(cs.dim, func(t *testing.T) {
			assert.That(t, DimensionKey(cs.dim), should.ErrLike(cs.err))
		})
	}
}

func TestDimensionValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dim string
		err any
	}{
		{"good value", nil},
		{strings.Repeat("a", maxDimensionValLen), nil},
		{"", "cannot be empty"},
		{strings.Repeat("a", maxDimensionValLen+1), "should be no longer"},
		{" bad value", "no leading or trailing spaces"},
		{"bad value ", "no leading or trailing spaces"},
	}

	for _, cs := range cases {
		t.Run(cs.dim, func(t *testing.T) {
			assert.That(t, DimensionValue(cs.dim), should.ErrLike(cs.err))
		})
	}
}

func TestTag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tag string
		err any
	}{
		// OK
		{"k:v", nil},
		{"", "tag must be in key:value form"},
		{fmt.Sprintf("%s:v", strings.Repeat("k", maxDimensionKeyLen)), nil},
		{fmt.Sprintf("k:%s", strings.Repeat("v", maxDimensionValLen)), nil},
		// key
		{":v", "the key cannot be empty"},
		{fmt.Sprintf("%s:v", strings.Repeat("k", maxDimensionKeyLen+1)),
			"should be no longer"},
		// value
		{"k:", "the value cannot be empty"},
		{"k: v", "no leading or trailing spaces"},
		{"k:v ", "no leading or trailing spaces"},
		{fmt.Sprintf("k:%s", strings.Repeat("v", maxDimensionValLen+1)),
			"should be no longer"},
		// reserved
		{"swarming.terminate:1", "reserved"},
	}

	for _, cs := range cases {
		t.Run(cs.tag, func(t *testing.T) {
			assert.That(t, Tag(cs.tag), should.ErrLike(cs.err))
		})
	}
}

func TestPriority(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p   int32
		err any
	}{
		{40, nil},
		{0, nil},
		{255, nil},
		{-1, "must be between 0 and 255"},
		{256, "must be between 0 and 255"},
	}

	for _, cs := range cases {
		t.Run(fmt.Sprint(cs.p), func(t *testing.T) {
			assert.That(t, Priority(cs.p), should.ErrLike(cs.err))
		})
	}
}

func TestServiceAccount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		sa  string
		err any
	}{
		{"sa@service-accounts.com", nil},
		{strings.Repeat("l", maxServiceAccountLength+1), "too long"},
		{"", "invalid"},
		{"invalid", "invalid"},
	}

	for _, cs := range cases {
		t.Run(cs.sa, func(t *testing.T) {
			assert.That(t, ServiceAccount(cs.sa), should.ErrLike(cs.err))
		})
	}
}

func TestBotPingTolerance(t *testing.T) {
	t.Parallel()
	cases := []struct {
		bpt int64
		err any
	}{
		{300, nil},
		{60, nil},
		{1200, nil},
		{-1, "must be between 60 and 1200"},
		{1201, "must be between 60 and 1200"},
	}

	for _, cs := range cases {
		t.Run(fmt.Sprint(cs.bpt), func(t *testing.T) {
			assert.That(t, BotPingTolerance(cs.bpt), should.ErrLike(cs.err))
		})
	}
}
