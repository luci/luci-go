// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"encoding/json"
	"log"
	"testing"

	"github.com/maruel/ut"
)

func TestConditionJson(t *testing.T) {
	c := condition{Condition: "OS == \"Linux\""}
	c.Variables.ReadOnly = new(int)
	c.Variables.Command = []string{"python", "generate", "something"}
	c.Variables.Files = []string{"generated"}
	jsonData, err := json.Marshal(&c)
	ut.AssertEqual(t, nil, err)
	t.Logf("Generated jsonData: %s", string(jsonData))
	pc := condition{}
	err = json.Unmarshal(jsonData, &pc)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, c, pc)
}

const sampleIsolateData = `
# This is file comment to be ignored.
{
	'conditions': [
		['OS=="linux" or OS=="mac" or OS=="win"', {
			'variables': {
				'files': [
					'../../testing/test_env.py',
					'<(PRODUCT_DIR)/ui_touch_selection_unittests<(EXECUTABLE_SUFFIX)',
				],
			},
		}],
	],
	'includes': [
		'../../base/base.isolate',
	],
}`

func TestParseIsolate(t *testing.T) {
	parsed, err := parseIsolate([]byte(sampleIsolateData))
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, `OS=="linux" or OS=="mac" or OS=="win"`, parsed.Conditions[0].Condition)
	ut.AssertEqual(t, 2, len(parsed.Conditions[0].Variables.Files))
	ut.AssertEqual(t, []string{"../../base/base.isolate"}, parsed.Includes)
}

func TestParseBadIsolate(t *testing.T) {
	log.Printf("!!!!! NOTE !!!!! Python errors are expected now as part of unittests.")
	// These tests make Python spill out errors into stderr, which might be confusing.
	// However, when real isolate command runs, we want to see these errors.
	if _, err := parseIsolate([]byte("statement = 'is not good'")); err == nil {
		t.Fail()
	}
	if _, err := parseIsolate([]byte("{ python/isolate syntax error")); err == nil {
		t.Fail()
	}
	log.Printf("!!!!! END !!!!!! of expected Python errors.")
	if _, err := parseIsolate([]byte("{'wrong_section': False, 'includes': 'must be a list'}")); err == nil {
		t.Fail()
	}
}
