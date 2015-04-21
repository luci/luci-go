// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/maruel/ut"
)

func TestConditionJson(t *testing.T) {
	t.Parallel()
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

func TestVariableValueOrder(t *testing.T) {
	t.Parallel()
	I := func(i int) variableValue { return variableValue{nil, &i} }
	S := func(s string) variableValue { return variableValue{&s, nil} }
	unbound := variableValue{}
	expectations := []struct {
		res int
		l   variableValue
		r   variableValue
	}{
		{0, unbound, unbound},
		{1, unbound, I(1)},
		{1, unbound, S("s")},
		{0, I(1), I(1)},
		{1, I(1), I(2)},
		{1, I(1), S("1")},
		{0, S("s"), S("s")},
		{1, S("a"), S("b")},
	}
	for _, e := range expectations {
		ut.AssertEqualf(t, e.res, e.l.compare(e.r), "%d != (%v < %v)", e.res, e.l.String(), e.r.String())
		ut.AssertEqualf(t, -e.res, e.r.compare(e.l), "%d != (%v > %v)", -e.res, e.l.String(), e.r.String())
	}
}

func TestConfigNameComparison(t *testing.T) {
	t.Parallel()
	C := func(s ...string) configName { return configName(makeVVs(s...)) }
	ut.AssertEqual(t, true, C().Equals(C()))
	ut.AssertEqual(t, 1, C("unbound").compare(C("1")))
}

func TestMatchConfigs(t *testing.T) {
	t.Parallel()
	unbound := "unbound" // Treated specially by makeVVs to create unbound variableValue.
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
			[][]variableValue{makeVVs("1", "b"), makeVVs("2", "b"), makeVVs(unbound, "b")},
		},
		{"foo==1 or bar==\"b\"", []string{"foo", "bar"},
			[][]variableValue{makeVVs("1", "a"), makeVVs("1", "b"), makeVVs("2", "a"), makeVVs("2", "b")},
			[][]variableValue{makeVVs("1", "a"), makeVVs("1", "b"), makeVVs("2", "b"), makeVVs("1", unbound)},
		},
	}
	for _, one := range expectations {
		out := matchConfigs(one.cond, one.conf, one.all)
		ut.AssertEqual(t, vvToStr2D(one.out), vvToStr2D(out))
	}
}

func TestCartesianProductOfValues(t *testing.T) {
	t.Parallel()
	set := func(vs ...string) map[variableValueKey]variableValue {
		out := map[variableValueKey]variableValue{}
		for _, v := range makeVVs(vs...) {
			out[v.key()] = v
		}
		return out
	}
	test := func(vvs variablesAndValues, keys []string, expected ...[]variableValue) {
		res := vvs.cartesianProductOfValues(keys)
		vvSort(expected)
		vvSort(res)
		ut.AssertEqual(t, vvToStr2D(expected), vvToStr2D(res))
	}
	keys := func(vs ...string) []string { return vs }

	vvs := variablesAndValues{}
	test(vvs, keys())

	vvs["OS"] = set("win", "unbound")
	test(vvs, keys("OS"), makeVVs("win"), makeVVs("unbound"))

	vvs["bit"] = set("32")

	test(vvs, keys("OS"), makeVVs("win"), makeVVs("unbound")) // bit var name must be ignored.
	test(vvs, keys("bit", "OS"), makeVVs("32", "win"), makeVVs("32", "unbound"))
}

func TestParseBadIsolate(t *testing.T) {
	t.Parallel()
	// These tests make Python spill out errors into stderr, which might be confusing.
	// However, when real isolate command runs, we want to see these errors.
	badIsolates := []string{
		"statement = 'is not good'",
		"{'includes': map(str, [1,2])}",
		"{ python/isolate syntax error",
		"{'wrong_section': False, 'includes': 'must be a list'}",
		"{'conditions': ['must be list of conditions', {}]}",
		"{'conditions': [['', {'variables-missing': {}}]]}",
		"{'variables': ['bad variables type']}",
	}
	for _, badIsolate := range badIsolates {
		if iso, err := parseIsolate([]byte(badIsolate)); err == nil {
			t.Logf("succeeded at parsing bad isolate: %s %s", badIsolate, iso.Includes)
			t.Fail()
		}
	}
}

func TestVerifyCondition(t *testing.T) {
	t.Parallel()
	c := condition{}
	varsAndValues := variablesAndValues{}
	badConditions := []string{
		"wrong condition1",
		"invalidConditionOp is False",
		"a == 1.1", // Python isolate_format is actually OK with this.
		"a = 1",
	}
	for _, bad := range badConditions {
		c.Condition = bad
		err := c.verify(varsAndValues)
		ut.AssertEqualf(t, true, err != nil, "must fail at %v condition", bad)
	}
}

func TestVerifyVariables(t *testing.T) {
	t.Parallel()
	v := variables{}
	badRo := -2
	v.ReadOnly = &badRo
	ut.AssertEqual(t, true, v.verify() != nil)
}

const sampleIncludes = `
	'includes': [
		'inc/included.isolate',
	],`

const sampleIsolateData = `
# This is file comment to be ignored.
{
	'conditions': [
		['(OS=="linux" and bit==64) or OS=="win"', {
			'variables': {
				'command': ['python', '64linuxOrWin'],
				'files': [
					'64linuxOrWin',
					'<(PRODUCT_DIR)/unittest<(EXECUTABLE_SUFFIX)',
				],
			},
		}],
		['bit==32 or (OS=="mac" and bit==64)', {
			'variables': {
				'command': ['python', '32orMac64'],
				'read_only': 2,
			},
		}],
	],
}`

const sampleIncIsolateData = `
{
	'conditions': [
		['OS=="linux" or OS=="win" or OS=="mac"', {
			'variables': {
				'command': ['to', 'be', 'ignored'],
				'files': [
					'inc_file',
					'<(DIR)/inc_unittest',
				],
			},
		}],
	],
}`

var sampleIsolateDataWithIncludes = addIncludesToSample(sampleIsolateData, sampleIncludes)

func TestParseIsolate(t *testing.T) {
	t.Parallel()
	parsed, err := parseIsolate([]byte(sampleIsolateDataWithIncludes))
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, `(OS=="linux" and bit==64) or OS=="win"`, parsed.Conditions[0].Condition)
	ut.AssertEqual(t, 2, len(parsed.Conditions[0].Variables.Files))
	ut.AssertEqual(t, []string{"inc/included.isolate"}, parsed.Includes)
}

func TestVerifyIsolate(t *testing.T) {
	t.Parallel()
	parsed, err := parseIsolate([]byte(sampleIsolateData))
	if err != nil {
		t.FailNow()
		return
	}
	varsAndValues, err := parsed.verify()
	ut.AssertEqualf(t, nil, err, "failed verification: %s", err)
	vars, ok := varsAndValues.getSortedValues("OS")
	ut.AssertEqual(t, true, ok)
	ut.AssertEqual(t, vvToStr(makeVVs("linux", "mac", "win")), vvToStr(vars))
	vars, ok = varsAndValues.getSortedValues("bit")
	ut.AssertEqual(t, true, ok)
	ut.AssertEqual(t, vvToStr(makeVVs("32", "64")), vvToStr(vars))
}

func TestLoadIsolateAsConfig(t *testing.T) {
	t.Parallel()
	isolate, err := LoadIsolateAsConfig("/dir", []byte(sampleIsolateData), []byte("# filecomment"))
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, isolate.FileComment, []byte("# filecomment"))
	ut.AssertEqual(t, []string{"OS", "bit"}, isolate.ConfigVariables)
}

func TestLoadIsolateForConfigMissingVars(t *testing.T) {
	t.Parallel()
	isoData := []byte(sampleIsolateData)
	_, _, _, _, err := LoadIsolateForConfig("/", isoData, common.KeyValVars{})
	ut.AssertEqual(t, true, err != nil)
	ut.AssertEqualf(t, true, strings.Contains(err.Error(), "variables were missing"), "%s", err)
	ut.AssertEqualf(t, true, strings.Contains(err.Error(), "bit"), "%s", err)
	ut.AssertEqualf(t, true, strings.Contains(err.Error(), "OS"), "%s", err)
	_, _, _, _, err = LoadIsolateForConfig("/", isoData, common.KeyValVars{"bit": "32"})
	ut.AssertEqual(t, true, err != nil)
	ut.AssertEqualf(t, true, strings.Contains(err.Error(), "variables were missing"), "%s", err)
	ut.AssertEqualf(t, true, strings.Contains(err.Error(), "OS"), "%s", err)
}

// TODO(tandrii): make sure these tests pass on windows.

func TestLoadIsolateForConfig(t *testing.T) {
	t.Parallel()
	// Case linux64, matches first condition.
	cmd, deps, ro, dir, err := LoadIsolateForConfig("/dir", []byte(sampleIsolateData),
		common.KeyValVars{"bit": "64", "OS": "linux"})
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "/dir", dir)
	ut.AssertEqual(t, NotSet, ro) // first condition has no read_only specified.
	ut.AssertEqual(t, []string{"python", "64linuxOrWin"}, cmd)
	ut.AssertEqual(t, []string{"64linuxOrWin", "<(PRODUCT_DIR)/unittest<(EXECUTABLE_SUFFIX)"}, deps)

	// Case win64, matches only first condition.
	cmd, deps, ro, dir, err = LoadIsolateForConfig("/dir", []byte(sampleIsolateData),
		common.KeyValVars{"bit": "64", "OS": "win"})
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "/dir", dir)
	ut.AssertEqual(t, NotSet, ro) // first condition has no read_only specified.
	ut.AssertEqual(t, []string{"python", "64linuxOrWin"}, cmd)
	// This is weird, but py isolate_format gives the same output.
	ut.AssertEqual(t, []string{
		"64linuxOrWin",
		"64linuxOrWin",
		"<(PRODUCT_DIR)/unittest<(EXECUTABLE_SUFFIX)",
		"<(PRODUCT_DIR)/unittest<(EXECUTABLE_SUFFIX)",
	}, deps)

	// Case mac32, matches only second condition.
	cmd, deps, ro, dir, err = LoadIsolateForConfig("/dir", []byte(sampleIsolateData),
		common.KeyValVars{"bit": "64", "OS": "mac"})
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "/dir", dir)
	ut.AssertEqual(t, DirsReadOnly, ro) // second condition has read_only 2.
	ut.AssertEqual(t, []string{"python", "32orMac64"}, cmd)
	ut.AssertEqual(t, []string{}, deps)

	// Case win32, both first and second condition match.
	cmd, deps, ro, dir, err = LoadIsolateForConfig("/dir", []byte(sampleIsolateData),
		common.KeyValVars{"bit": "32", "OS": "win"})
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "/dir", dir)
	ut.AssertEqual(t, DirsReadOnly, ro) // first condition no read_only, but second has 2.
	ut.AssertEqual(t, []string{"python", "32orMac64"}, cmd)
	// This is weird, but py isolate_format gives the same output.
	ut.AssertEqual(t, []string{
		"64linuxOrWin",
		"64linuxOrWin",
		"<(PRODUCT_DIR)/unittest<(EXECUTABLE_SUFFIX)",
		"<(PRODUCT_DIR)/unittest<(EXECUTABLE_SUFFIX)",
	}, deps)
}

func TestLoadIsolateAsConfigWithIncludes(t *testing.T) {
	t.Parallel()
	tmpDir, err := ioutil.TempDir("", "test-isofmt-")
	ut.AssertEqual(t, nil, err)
	defer os.RemoveAll(tmpDir)
	err = os.Mkdir(filepath.Join(tmpDir, "inc"), 0777)
	ut.AssertEqual(t, nil, err)
	ioutil.WriteFile(filepath.Join(tmpDir, "inc", "included.isolate"), []byte(sampleIncIsolateData), 0777)

	// Test failures.
	absIncData := addIncludesToSample(sampleIsolateData, "'includes':['/abs/path']")
	_, _, _, _, err = LoadIsolateForConfig(tmpDir, []byte(absIncData), common.KeyValVars{})
	ut.AssertEqualf(t, true, err != nil && strings.Contains(err.Error(), "absolute include path"), "%v", err)

	_, _, _, _, err = LoadIsolateForConfig(filepath.Join(tmpDir, "wrong-dir"),
		[]byte(sampleIsolateDataWithIncludes), common.KeyValVars{})
	ut.AssertEqualf(t, true, err != nil && strings.Contains(err.Error(), "no such file"), "%v", err)

	// Test Successfull loading.
	// Case mac32, matches only second condition from main isolate and one in included.
	cmd, deps, ro, dir, err := LoadIsolateForConfig(tmpDir, []byte(sampleIsolateDataWithIncludes),
		common.KeyValVars{"bit": "64", "OS": "linux"})
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, tmpDir, dir)
	ut.AssertEqual(t, NotSet, ro) // first condition has no read_only specified.
	ut.AssertEqual(t, []string{"python", "64linuxOrWin"}, cmd)
	ut.AssertEqual(t, []string{
		"../inc_file",
		"64linuxOrWin",
		"<(DIR)/inc_unittest", // no rebasing for this.
		"<(PRODUCT_DIR)/unittest<(EXECUTABLE_SUFFIX)",
	}, deps)
}

// Helper functions.

// makeVVs simplifies creating variableValue:
// "unbound" => unbound
// "123" => int(123)
// "s123" => string("123")
func makeVVs(ss ...string) []variableValue {
	vs := make([]variableValue, len(ss))
	for i, s := range ss {
		if s == "unbound" {
			continue
		} else if strings.HasPrefix(s, "s") {
			vs[i] = createVariableValueTryInt(s[1:])
			if vs[i].I == nil {
				vs[i].S = &s
			}
		} else {
			vs[i] = createVariableValueTryInt(s)
		}
	}
	return vs
}

func vvToStr(vs []variableValue) []string {
	ks := make([]string, len(vs))
	for i, v := range vs {
		ks[i] = v.String()
	}
	return ks
}

func vvToStr2D(vs [][]variableValue) [][]string {
	ks := make([][]string, len(vs))
	for i, v := range vs {
		ks[i] = vvToStr(v)
	}
	return ks
}

func vvSort(vss [][]variableValue) {
	tmpMap := map[string][]variableValue{}
	keys := []string{}
	for _, vs := range vss {
		key := configName(vs).key()
		keys = append(keys, key)
		tmpMap[key] = vs
	}
	sort.Strings(keys)
	for i, key := range keys {
		vss[i] = tmpMap[key]
	}
}

func addIncludesToSample(sample, includes string) string {
	assert(sample[len(sample)-1] == '}')
	return sample[:len(sample)-1] + includes + "\n}"
}
