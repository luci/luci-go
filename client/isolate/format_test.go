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

func TestAssert(t *testing.T) {
	ok := func() (ok bool) {
		ok = false
		defer func() {
			if e := recover(); e != nil {
				ok = true
			}
		}()
		assert(false)
		return
	}()
	if !ok {
		t.Fail()
	}
}

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
}`

const sampleIsolateDataWithIncludes = `
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
	parsed, err := parseIsolate([]byte(sampleIsolateDataWithIncludes))
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

func makeVVs(ss ...string) []variableValue {
	vs := make([]variableValue, len(ss))
	for i, s := range ss {
		vs[i] = createVariableValueTryInt(s)
	}
	return vs
}

func toVVs(vs []variableValue) []string {
	ks := make([]string, len(vs))
	for i, v := range vs {
		ks[i] = v.String()
	}
	return ks
}

func toVVs2D(vs [][]variableValue) [][]string {
	ks := make([][]string, len(vs))
	for i, v := range vs {
		ks[i] = toVVs(v)
	}
	return ks
}

func TestVerifyIsolate(t *testing.T) {
	parsed, err := parseIsolate([]byte(sampleIsolateData))
	if err != nil {
		t.FailNow()
		return
	}
	varsAndValues, err := parsed.verify()
	if err != nil {
		t.Fatalf("failed verification: %s", err)
	}
	vars, ok := varsAndValues.getSortedValues("OS")
	ut.AssertEqual(t, true, ok)
	ut.AssertEqual(t, []string{"linux", "mac", "win"}, toVVs(vars))
}

func TestMatchConfigs(t *testing.T) {
	expectations := []struct {
		cond string
		conf []string
		all  [][]variableValue
		out  [][]variableValue
	}{
		{"OS==\"win\"", []string{"OS"},
			[][]variableValue{makeVVs("win"), makeVVs("mac"), makeVVs("linux")},
			[][]variableValue{makeVVs("win")},
		},
		{"(foo==1 or foo==2) and bar==\"b\"", []string{"foo", "bar"},
			[][]variableValue{makeVVs("1", "a"), makeVVs("1", "b"), makeVVs("2", "a"), makeVVs("2", "b")},
			[][]variableValue{makeVVs("1", "b"), makeVVs("2", "b")},
		},
		{"bar==\"b\"", []string{"foo", "bar"},
			[][]variableValue{makeVVs("1", "a"), makeVVs("1", "b"), makeVVs("2", "a"), makeVVs("2", "b")},
			[][]variableValue{makeVVs("1", "b"), makeVVs("2", "b"), makeVVs("", "b")},
		},
		{"foo==1 or bar==\"b\"", []string{"foo", "bar"},
			[][]variableValue{makeVVs("1", "a"), makeVVs("1", "b"), makeVVs("2", "a"), makeVVs("2", "b")},
			[][]variableValue{makeVVs("1", "a"), makeVVs("1", "b"), makeVVs("2", "b"), makeVVs("1", "")},
		},
	}
	for _, one := range expectations {
		out := matchConfigs(one.cond, one.conf, one.all)
		ut.AssertEqual(t, toVVs2D(one.out), toVVs2D(out))
	}
}

func TestLoadIsolateAsConfig(t *testing.T) {
	isolate, err := LoadIsolateAsConfig("/s/swarming", []byte(sampleIsolateData), []byte("# filecomment"))
	if err != nil {
		t.Error(err)
	}
	ut.AssertEqual(t, isolate.FileComment, []byte("# filecomment"))
	ut.AssertEqual(t, []string{"OS"}, isolate.ConfigVariables)
}
