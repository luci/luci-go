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

package isolate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestConditionJson(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should properly support JSON in conditional section of file.`, t, func(t *ftt.Test) {
		c := condition{Condition: "OS == \"Linux\""}
		c.Variables.Files = []string{"generated"}
		jsonData, err := json.Marshal(&c)
		assert.Loosely(t, err, should.BeNil)
		t.Logf("Generated jsonData: %s", string(jsonData))
		pc := condition{}
		err = json.Unmarshal(jsonData, &pc)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pc, should.Resemble(c))
	})
}

func TestVariableValueOrder(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should properly handle variable value ordering.`, t, func(t *ftt.Test) {
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
			assert.Loosely(t, e.res, should.Resemble(e.l.compare(e.r)))
			assert.Loosely(t, -e.res, should.Resemble(e.r.compare(e.l)))
		}
	})
}

func TestConfigNameComparison(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should support comparison of config names.`, t, func(t *ftt.Test) {
		c := func(s ...string) configName { return configName(makeVVs(s...)) }
		assert.Loosely(t, c().Equals(c()), should.BeTrue)
		assert.Loosely(t, c("unbound").compare(c("1")), should.Match(1))
	})
}

func TestProcessConditionBad(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should handle bad condition formats.`, t, func(t *ftt.Test) {
		expectations := []string{
			"wrong condition1",
			"invalidConditionOp is False",
			"a == 1.1", // Python isolate_format is actually OK with this.
			"a = 1",
		}
		for _, e := range expectations {
			_, err := processCondition(condition{Condition: e}, variablesValuesSet{})
			assert.Loosely(t, err, should.NotBeNil)
		}
	})
}

func TestConditionEvaluate(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should properly evaluate conditions.`, t, func(t *ftt.Test) {
		const (
			T int = 1
			F int = 0
			E int = -1
		)
		expectations := []struct {
			cond string
			vals map[string]string
			exp  int
		}{
			{"A=='w'", map[string]string{"A": "w"}, T},
			{"A=='m'", map[string]string{"A": "w"}, F},
			{"A=='m'", map[string]string{"a": "w"}, E},
			{"A==1 or B=='b'", map[string]string{"A": "1"}, T},
			{"A==1 or B=='b'", map[string]string{"B": "b"}, E},
			{"A==1 and B=='b'", map[string]string{"A": "1"}, E},

			{"(A=='w')", map[string]string{"A": "w"}, T},
			{"A==1 or (B==2 and C==3)", map[string]string{"A": "1"}, T},
			{"A==1 or (B==2 and C==3)", map[string]string{"A": "0"}, E},
			{"A==1 or (B==2 and C==3)", map[string]string{"A": "0", "B": "0"}, F},
			{"A==1 or (B==2 and C==3)", map[string]string{"A": "2", "B": "2"}, E},
			{"A==1 or (B==2 and C==3)", map[string]string{"A": "2", "B": "2", "C": "3"}, T},

			{"(A==1 or A==2) and B==3", map[string]string{"A": "1", "B": "3"}, T},
			{"(A==1 or A==2) and B==3", map[string]string{"A": "2", "B": "3"}, T},
			{"(A==1 or A==2) and B==3", map[string]string{"B": "3"}, E},
		}
		for _, e := range expectations {
			c, err := processCondition(condition{Condition: e.cond}, variablesValuesSet{})
			assert.Loosely(t, err, should.BeNil)
			isTrue, err := c.evaluate(func(v string) variableValue {
				if value, ok := e.vals[v]; ok {
					return makeVariableValue(value)
				}
				must(variableValue{}.isBound() == false)
				return variableValue{}
			})
			if e.exp == E {
				assert.Loosely(t, err, should.ErrLike(errUnbound))
			} else {
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, e.exp == T, should.Resemble(isTrue))
			}
		}
	})
}

func TestMatchConfigs(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should properly match configs.`, t, func(t *ftt.Test) {
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
		for _, e := range expectations {
			c, err := processCondition(condition{Condition: e.cond}, variablesValuesSet{})
			assert.Loosely(t, err, should.BeNil)
			out := c.matchConfigs(makeConfigVariableIndex(e.conf), e.all)
			assert.Loosely(t, vvToStr2D(vvSort(out)), should.Resemble(vvToStr2D(vvSort(e.out))))
		}
	})
}

func TestCartesianProductOfValues(t *testing.T) {
	t.Parallel()
	ftt.Run(`Tests the cartesian product of values.`, t, func(t *ftt.Test) {
		set := func(vs ...string) map[variableValueKey]variableValue {
			out := map[variableValueKey]variableValue{}
			for _, v := range makeVVs(vs...) {
				out[v.key()] = v
			}
			return out
		}
		test := func(vvs variablesValuesSet, keys []string, expected ...[]variableValue) {
			res, err := vvs.cartesianProductOfValues(keys)
			assert.Loosely(t, err, should.BeNil)
			vvSort(expected)
			vvSort(res)
			assert.Loosely(t, vvToStr2D(res), should.Resemble(vvToStr2D(expected)))
		}
		keys := func(vs ...string) []string { return vs }

		vvs := variablesValuesSet{}
		test(vvs, keys())

		vvs["OS"] = set("win", "unbound")
		test(vvs, keys("OS"), makeVVs("win"), makeVVs("unbound"))

		vvs["bit"] = set("32")

		test(vvs, keys("OS"), makeVVs("win"), makeVVs("unbound")) // bit var name must be ignored.
		test(vvs, keys("bit", "OS"), makeVVs("32", "win"), makeVVs("32", "unbound"))
	})
}

func TestParseBadIsolate(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should handle bad formats.`, t, func(t *ftt.Test) {
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
			_, err := processIsolate([]byte(badIsolate))
			assert.Loosely(t, err, should.NotBeNil)
		}
	})
}

func TestPythonToGoString(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should properly transform from Python to Go.`, t, func(t *ftt.Test) {
		expectations := []struct {
			in, out, left string
		}{
			{`''`, `""`, ``},
			{`''\'`, `""`, `\'`},
			{`'''`, `""`, `'`},
			{`""`, `""`, ``},
			{`"" and`, `""`, ` and`},
			{`'"' "w`, `"\""`, ` "w`},
			{`"'"`, `"'"`, ``},
			{`"\'"`, `"'"`, ``},
			{`"ok" or`, `"ok"`, ` or`},
			{`'\\"\\'`, `"\\\"\\"`, ``},
			{`"\\\"\\"`, `"\\\"\\"`, ``},
			{`"'\\'"`, `"'\\'"`, ``},
			{`'"\'\'"'`, `"\"''\""`, ``},
			{`'"\'\'"'`, `"\"''\""`, ``},
			{`'∀ unicode'`, `"∀ unicode"`, ``},
		}
		for _, e := range expectations {
			goChunk, left, err := pythonToGoString([]rune(e.in))
			t.Logf("in: `%s` eg: `%s` g: `%s` el: `%s` l: `%s` err: %s", e.in, e.out, goChunk, e.left, string(left), err)
			assert.Loosely(t, string(left), should.Resemble(e.left))
			assert.Loosely(t, goChunk, should.Resemble(e.out))
			assert.Loosely(t, err, should.BeNil)
		}
	})
}

func TestPythonToGoStringError(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should handle errors in transforming from Python to Go.`, t, func(t *ftt.Test) {
		for _, e := range []string{`'"`, `"'`, `'\'`, `"\"`, `'""`, `"''`} {
			goChunk, left, err := pythonToGoString([]rune(e))
			t.Logf("in: `%s`, g: `%s`, l: `%s`, err: %s", e, goChunk, string(left), err)
			assert.Loosely(t, err, should.ErrLike(errParseCondition))
		}
	})
}

func TestPythonToGoNonString(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should handle invalid strings in transforming from Python to Go.`, t, func(t *ftt.Test) {
		expectations := []struct {
			in, out, left string
		}{
			{`and`, `&&`, ``},
			{`or`, `||`, ``},
			{` or('str'`, ` ||(`, `'str'`},
			{`)or(`, `)||(`, ``},
			{`andor`, `andor`, ``},
			{`)whatever("string...`, `)whatever(`, `"string...`},
		}
		for _, e := range expectations {
			goChunk, left := pythonToGoNonString([]rune(e.in))
			t.Logf("in: `%s` eg: `%s` g: `%s` el: `%s` l: `%s`", e.in, e.out, goChunk, e.left, string(left))
			assert.Loosely(t, string(left), should.Resemble(e.left))
			assert.Loosely(t, goChunk, should.Resemble(e.out))
		}
	})
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
				'files': [
					'64linuxOrWin',
					'<(PRODUCT_DIR)/unittest<(EXECUTABLE_SUFFIX)',
				],
			},
		}],
		['bit==32 or (OS=="mac" and bit==64)', {
			'variables': {
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
				'files': [
					'inc_file',
					'<(DIR)/inc_unittest',
				],
			},
		}],
	],
}`

var sampleIsolateDataWithIncludes = addIncludesToSample(sampleIsolateData, sampleIncludes)

func TestProcessIsolate(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should properly process and verify inputs.`, t, func(t *ftt.Test) {
		p, err := processIsolate([]byte(sampleIsolateDataWithIncludes))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, p.conditions[0].condition, should.Match(`(OS=="linux" and bit==64) or OS=="win"`))
		assert.Loosely(t, p.conditions[0].variables.Files, should.HaveLength(2))
		assert.Loosely(t, p.includes, should.Resemble([]string{"inc/included.isolate"}))

		vars, ok := getSortedVarValues(p.varsValsSet, "OS")
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, vvToStr(vars), should.Resemble(vvToStr(makeVVs("linux", "mac", "win"))))
		vars, ok = getSortedVarValues(p.varsValsSet, "bit")
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, vvToStr(vars), should.Resemble(vvToStr(makeVVs("32", "64"))))
	})
}

func TestLoadIsolateAsConfig(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should properly load a config from a isolate file.`, t, func(t *ftt.Test) {
		root := "/dir"
		if runtime.GOOS == "windows" {
			root = "x:\\dir"
		}
		isolate, err := LoadIsolateAsConfig(root, []byte(sampleIsolateData))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, isolate.ConfigVariables, should.Resemble([]string{"OS", "bit"}))
	})
}

func TestLoadIsolateForConfigMissingVars(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should properly handle missing variables when loading a config.`, t, func(t *ftt.Test) {
		isoData := []byte(sampleIsolateData)
		root := "/dir"
		if runtime.GOOS == "windows" {
			root = "x:\\dir"
		}
		_, _, err := LoadIsolateForConfig(root, isoData, nil)
		assert.Loosely(t, err, should.NotBeNil)
		t.Run(fmt.Sprintf("Verify error message: %s", err), func(t *ftt.Test) {
			assert.Loosely(t, err.Error(), should.ContainSubstring("variables were missing"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("bit"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("OS"))
			_, _, err = LoadIsolateForConfig(root, isoData, map[string]string{"bit": "32"})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("variables were missing"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("OS"))
		})
	})
}

func TestLoadIsolateForConfig(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should properly load and return config data from a isolate file.`, t, func(t *ftt.Test) {
		// Case linux64, matches first condition.
		root := "/dir"
		if runtime.GOOS == "windows" {
			root = "x:\\dir"
		}
		vars := map[string]string{"bit": "64", "OS": "linux"}
		deps, dir, err := LoadIsolateForConfig(root, []byte(sampleIsolateData), vars)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, dir, should.Resemble(root))
		assert.Loosely(t, deps, should.Resemble([]string{"64linuxOrWin", filepath.Join("<(PRODUCT_DIR)", "unittest<(EXECUTABLE_SUFFIX)")}))

		// Case win64, matches only first condition.
		vars = map[string]string{"bit": "64", "OS": "win"}
		deps, dir, err = LoadIsolateForConfig(root, []byte(sampleIsolateData), vars)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, dir, should.Resemble(root))
		assert.Loosely(t, deps, should.Resemble([]string{
			"64linuxOrWin",
			filepath.Join("<(PRODUCT_DIR)", "unittest<(EXECUTABLE_SUFFIX)"),
		}))

		// Case mac64, matches only second condition.
		vars = map[string]string{"bit": "64", "OS": "mac"}
		deps, dir, err = LoadIsolateForConfig(root, []byte(sampleIsolateData), vars)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, dir, should.Resemble(root))
		assert.Loosely(t, deps, should.BeEmpty)

		// Case win32, both first and second condition match.
		vars = map[string]string{"bit": "32", "OS": "win"}
		deps, dir, err = LoadIsolateForConfig(root, []byte(sampleIsolateData), vars)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, dir, should.Resemble(root))
		assert.Loosely(t, deps, should.Resemble([]string{
			"64linuxOrWin",
			filepath.Join("<(PRODUCT_DIR)", "unittest<(EXECUTABLE_SUFFIX)"),
		}))
	})
}

func TestLoadIsolateAsConfigWithIncludes(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should load a config from isolate with includes.`, t, func(t *ftt.Test) {
		tmpDir := t.TempDir()
		err := os.Mkdir(filepath.Join(tmpDir, "inc"), 0777)
		assert.Loosely(t, err, should.BeNil)
		err = os.WriteFile(filepath.Join(tmpDir, "inc", "included.isolate"), []byte(sampleIncIsolateData), 0777)
		assert.Loosely(t, err, should.BeNil)

		// Test failures.
		absIncData := addIncludesToSample(sampleIsolateData, "'includes':['/abs/path']")
		_, _, err = LoadIsolateForConfig(tmpDir, []byte(absIncData), nil)
		assert.Loosely(t, err, should.NotBeNil)

		_, _, err = LoadIsolateForConfig(filepath.Join(tmpDir, "wrong-dir"),
			[]byte(sampleIsolateDataWithIncludes), nil)
		assert.Loosely(t, err, should.NotBeNil)

		// Test Successful loading.
		// Case mac32, matches only second condition from main isolate and one in included.
		vars := map[string]string{"bit": "64", "OS": "linux"}
		deps, dir, err := LoadIsolateForConfig(tmpDir, []byte(sampleIsolateDataWithIncludes), vars)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, dir, should.Resemble(filepath.Join(tmpDir, "inc")))
		assert.Loosely(t, deps, should.Resemble([]string{
			filepath.Join("..", "64linuxOrWin"),
			filepath.Join("<(DIR)", "inc_unittest"), // no rebasing for this.
			filepath.Join("<(PRODUCT_DIR)", "unittest<(EXECUTABLE_SUFFIX)"),
			"inc_file",
		}))
	})
}

func TestConfigSettingsUnionLeft(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should properly handle config setting merging.`, t, func(t *ftt.Test) {
		left := &ConfigSettings{
			Files:      []string{"../../le/f/t", "foo/"}, // Must be POSIX.
			IsolateDir: absToOS("/tmp/bar"),              // In native path.
		}
		right := &ConfigSettings{
			Files:      []string{"../ri/g/ht", "bar/"},
			IsolateDir: absToOS("/var/lib"),
		}

		out, err := left.union(right)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, out.IsolateDir, should.Resemble(left.IsolateDir))
		assert.Loosely(t, left.IsolateDir, should.Resemble(absToOS("/tmp/bar")))
		assert.Loosely(t, out.Files, should.Resemble([]string{"../../le/f/t", "../../var/lib/bar/", "../../var/ri/g/ht", "foo/"}))
	})
}

func TestConfigSettingsUnionRight(t *testing.T) {
	t.Parallel()
	ftt.Run(`Isolate should properly handle config setting merging.`, t, func(t *ftt.Test) {
		left := &ConfigSettings{
			Files:      []string{"../../le/f/t", "foo/"}, // Must be POSIX.
			IsolateDir: absToOS("/tmp/bar"),              // In native path.
		}
		right := &ConfigSettings{
			Files:      []string{"../ri/g/ht", "bar/"},
			IsolateDir: absToOS("/var/lib"),
		}

		out, err := left.union(right)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, out.IsolateDir, should.Resemble(left.IsolateDir))
		assert.Loosely(t, right.IsolateDir, should.Resemble(absToOS("/var/lib")))
		assert.Loosely(t, out.Files, should.Resemble([]string{"../../le/f/t", "../../var/lib/bar/", "../../var/ri/g/ht", "foo/"}))
	})
}

func TestParseIsolate(t *testing.T) {
	t.Parallel()
	ftt.Run("test parseIsolate having double quotation with escape", t, func(t *ftt.Test) {
		content := []byte(`{"variables": {"command": ["{\"test_args\": <(test_args)}"]}}`)
		_, err := parseIsolate(content)
		assert.Loosely(t, err, should.BeNil)
	})
}

// Helper functions.

// absToOS converts a POSIX path to OS specific format.
func absToOS(p string) string {
	if runtime.GOOS == "windows" {
		return "e:" + strings.Replace(p, "/", "\\", -1)
	}
	return p
}

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
			vs[i] = makeVariableValue(s[1:])
			if vs[i].I == nil {
				vs[i].S = &s
			}
		} else {
			vs[i] = makeVariableValue(s)
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

func vvSort(vss [][]variableValue) [][]variableValue {
	tmpMap := make(map[string][]variableValue, len(vss))
	keys := make([]string, len(vss))
	for i, vs := range vss {
		key := configName(vs).key()
		keys[i] = key
		tmpMap[key] = vs
	}
	sort.Strings(keys)
	for i, key := range keys {
		vss[i] = tmpMap[key]
	}
	return vss
}

func addIncludesToSample(sample, includes string) string {
	must(sample[len(sample)-1] == '}')
	return sample[:len(sample)-1] + includes + "\n}"
}

func getSortedVarValues(v variablesValuesSet, varName string) ([]variableValue, bool) {
	valueSet, ok := v[varName]
	if !ok {
		return nil, false
	}
	keys := make([]string, 0, len(valueSet))
	for key := range valueSet {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	values := make([]variableValue, len(valueSet))
	for i, key := range keys {
		values[i] = valueSet[variableValueKey(key)]
	}
	return values, true
}
