// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/common/isolated"
)

var osPathSeparator = string(os.PathSeparator)

// ReadOnlyValue defines permissions on isolated files.
type ReadOnlyValue int

const (
	NotSet        ReadOnlyValue = -1
	Writeable     ReadOnlyValue = 0
	FilesReadOnly ReadOnlyValue = 1
	DirsReadOnly  ReadOnlyValue = 2
)

func (r ReadOnlyValue) ToIsolated() (out *isolated.ReadOnlyValue) {
	switch r {
	case NotSet:
	case Writeable:
		out = new(isolated.ReadOnlyValue)
		*out = isolated.Writeable
	case FilesReadOnly:
		out = new(isolated.ReadOnlyValue)
		*out = isolated.FilesReadOnly
	case DirsReadOnly:
		out = new(isolated.ReadOnlyValue)
		*out = isolated.DirsReadOnly
	default:
		log.Printf("invalid ReadOnlyValue %d", r)
	}
	return
}

type variableValue struct {
	S *string `json:",omitempty"`
	I *int    `json:",omitempty"`
}

func createVariableValueTryInt(s string) variableValue {
	v := variableValue{}
	if i, err := strconv.Atoi(s); err == nil {
		v.I = &i
	} else {
		v.S = &s
	}
	return v
}

func (v variableValue) String() string {
	if v.S != nil {
		return *v.S
	} else if v.I != nil {
		return fmt.Sprintf("%d", *v.I)
	}
	return ""
}

// compare returns 0 if equal, 1 if lhs < right, else -1.
// Order: unbound < 1 < 2 < "abc" < "cde" .
func (lhs variableValue) compare(rhs variableValue) int {
	if lhs.I != nil {
		if rhs.I != nil {
			// Both integers.
			if *lhs.I < *rhs.I {
				return 1
			} else if *lhs.I > *rhs.I {
				return -1
			} else {
				return 0
			}
		} else if rhs.S != nil {
			// int vs string
			return 1
		} else {
			// int vs Unbound.
			return -1
		}
	} else if lhs.S != nil {
		if rhs.S != nil {
			// Both strings.
			if *lhs.S < *rhs.S {
				return 1
			} else if *lhs.S > *rhs.S {
				return -1
			} else {
				return 0
			}
		} else {
			// string vs (int | unbound)
			return -1
		}
	} else if rhs.isBound() {
		// unbound vs (int|string)
		return 1
	} else {
		// unbound vs unbound
		return 0
	}
}

func (v variableValue) isBound() bool {
	return v.S != nil || v.I != nil
}

// For indexing by variableValue in a map.
type variableValueKey string

func (v variableValue) key() variableValueKey {
	if v.S != nil {
		return variableValueKey("~" + *v.S)
	}
	if v.I != nil {
		return variableValueKey(string(*v.I))
	}
	return variableValueKey("")
}

type variablesAndValues map[string]map[variableValueKey]variableValue

// getSortedValues returns sorted values of given variable and whether it exists.
func (v variablesAndValues) getSortedValues(varName string) ([]variableValue, bool) {
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

func (v variablesAndValues) cartesianProductOfValues(orderedKeys []string) ([][]variableValue, error) {
	if len(orderedKeys) == 0 {
		return [][]variableValue{}, nil
	}
	// Prepare ordered by orderedKeys list of variableValue
	allValues := make([][]variableValue, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		valuesSet := v[key]
		values := make([]variableValue, 0, len(valuesSet))
		for _, value := range valuesSet {
			values = append(values, value)
		}
		allValues = append(allValues, values)
	}
	// Precompute length of output for alloc and for assertion at the end.
	length := 1
	for _, values := range allValues {
		length *= len(values)
	}
	if length <= 0 {
		return nil, errors.New("some variable had empty valuesSet?")
	}
	out := make([][]variableValue, 0, length)
	// indices[i] points to index in allValues[i]; stop once indices[-1] == len(allValues[-1]).
	indices := make([]int, len(orderedKeys))
	for {
		next := make([]variableValue, len(orderedKeys))
		for i, values := range allValues {
			if indices[i] == len(values) {
				if i+1 == len(orderedKeys) {
					if length != len(out) {
						return nil, errors.New("internal error")
					}
					return out, nil
				}
				indices[i] = 0
				indices[i+1]++
			}
			next[i] = values[indices[i]]
		}
		out = append(out, next)
		indices[0]++
	}
	return nil, errors.New("unreachable code")
}

func matchConfigs(condition string, configVariables []string, allConfigs [][]variableValue) ([][]variableValue, error) {
	// TODO(tandrii): get rid of Python here...
	// While the looping can be done in Go, it'd require multiple calls to Python,
	// which isn't better for performance. Ideally, we could parse ast in Python once,
	// and evaluate the AST in Go later with each set of variable values.

	// This code is copy-paste from Python isolate. The Go->Python->Go is done through json,
	// through stdio|stdout pipes.
	pythonCode := `
import json, sys, itertools
def match_configs(expr, config_variables, all_configs):
	combinations = []
	for bound_variables in itertools.product((True, False), repeat=len(config_variables)):
		# Add the combination of variables bound.
		combinations.append(
			(
				[c for c, b in zip(config_variables, bound_variables) if b],
				set(
					tuple(v if b else None for v, b in zip(line, bound_variables))
					for line in all_configs)
			))
	out = []
	for variables, configs in combinations:
		# Strip variables and see if expr can still be evaluated.
		for values in configs:
			globs = {'__builtins__': None}
			globs.update(zip(variables, (v for v in values if v is not None)))
			try:
				assertion = eval(expr, globs, {})
			except NameError:
				continue
			if not isinstance(assertion, bool):
				raise IsolateError('Invalid condition')
			if assertion:
				out.append(values)
	return out
input = json.loads(sys.stdin.read())
all_configs = [[v['I'] if 'I' in v else v['S'] for v in conf] for conf in input['a']]
out = match_configs(input['cond'], input['conf'], all_configs)
print json.dumps([[{'I': v} if isinstance(v, int) else {'S': v} for v in vs] for vs in out])
`
	f, err := ioutil.TempFile("", "isolate")
	if err != nil {
		return nil, err
	}
	_, err = io.WriteString(f, pythonCode)
	if err != nil {
		return nil, err
	}
	f.Close()
	defer func() {
		_ = os.Remove(f.Name())
	}()
	m := map[string]interface{}{}
	m["cond"] = condition
	m["conf"] = configVariables
	m["a"] = allConfigs
	jsonDataIn, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	jsonDataOut, err := common.RunPython(jsonDataIn, f.Name())
	if err != nil {
		return nil, err
	}
	var out [][]variableValue
	err = json.Unmarshal(jsonDataOut, &out)
	return out, err
}

type parsedIsolate struct {
	Includes   []string
	Conditions []condition
	Variables  *variables
}

func (p *parsedIsolate) verify() (variablesAndValues, error) {
	varsAndValues := variablesAndValues{}
	for _, cond := range p.Conditions {
		if err := cond.verify(varsAndValues); err != nil {
			return varsAndValues, err
		}
	}
	if p.Variables != nil {
		return varsAndValues, p.Variables.verify()
	}
	return varsAndValues, nil
}

// condition represents conditional part of an isolate file.
type condition struct {
	Condition string
	Variables variables
	// Helper to store variable names in Condition strings, set by verify method.
	variableNames *[]string
}

// MarshalJSON implements json.Marshaler interface.
func (p *condition) MarshalJSON() ([]byte, error) {
	d := [2]json.RawMessage{}
	var err error
	if d[0], err = json.Marshal(&p.Condition); err != nil {
		return nil, err
	}
	m := map[string]variables{"variables": p.Variables}
	if d[1], err = json.Marshal(&m); err != nil {
		return nil, err
	}
	return json.Marshal(&d)
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (p *condition) UnmarshalJSON(data []byte) error {
	var d []json.RawMessage
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	if len(d) != 2 {
		return errors.New("condition must be a list with two items.")
	}
	if err := json.Unmarshal(d[0], &p.Condition); err != nil {
		return err
	}
	m := map[string]variables{}
	if err := json.Unmarshal(d[1], &m); err != nil {
		return err
	}
	var ok bool
	if p.Variables, ok = m["variables"]; !ok {
		return errors.New("variables item is required in condition.")
	}
	return nil
}

// verify ensures Condition is in correct format.
// Updates argument variablesAndValues and also local variableNames.
func (p *condition) verify(varsAndValues variablesAndValues) error {
	// TODO: can we get rid of Python here?
	// This code is copy-paste from Python isolate. It expects condition expression on stdin verbatim,
	// and sends result as json on stdout. Note, that format of variables_and_values is modified to be
	// easily unmarshalled by encoding/json package into variableValue struct.
	pythonCode := `
import json, sys, ast
def verify_ast(expr, variables_and_values):
	"""Verifies that |expr| is of the form
	expr ::= expr ( "or" | "and" ) expr
		| identifier "==" ( string | int )
	Also collects the variable identifiers and string/int values in the dict
	|variables_and_values|, in the form {'var': set([val1, val2, ...]), ...}.
	"""
	assert isinstance(expr, (ast.BoolOp, ast.Compare))
	if isinstance(expr, ast.BoolOp):
		assert isinstance(expr.op, (ast.And, ast.Or))
		for subexpr in expr.values:
			verify_ast(subexpr, variables_and_values)
	else:
		assert isinstance(expr.left.ctx, ast.Load)
		assert len(expr.ops) == 1
		assert isinstance(expr.ops[0], ast.Eq)
		var_values = variables_and_values.setdefault(expr.left.id, [])
		rhs = expr.comparators[0]
		assert isinstance(rhs, (ast.Str, ast.Num))
		if isinstance(rhs, ast.Num):
			assert int(rhs.n) == rhs.n, "only ints are allowed, but %r given" % rhs.n
			val = {'I': int(rhs.n)}  # Has to be int(), otherwise Go will fail to read it.
		else:
			val = {'S': rhs.s}
		var_values.append(val)

test_ast = compile(sys.stdin.read(), '<condition>', 'eval', ast.PyCF_ONLY_AST)
variables_and_values = {}
verify_ast(test_ast.body, variables_and_values)
print json.dumps(variables_and_values)
`
	f, err := ioutil.TempFile("", "isolate")
	_, _ = io.WriteString(f, pythonCode)
	f.Close()
	defer func() {
		_ = os.Remove(f.Name())
	}()
	jsonData, err := common.RunPython([]byte(p.Condition), f.Name())
	if err != nil {
		return fmt.Errorf("failed to verify Condition string %s: %s", p.Condition, err)
	}
	tmpVarsAndValues := map[string][]variableValue{}
	if err = json.Unmarshal(jsonData, &tmpVarsAndValues); err != nil {
		return err
	}
	p.variableNames = new([]string)
	for varName, tmpValueList := range tmpVarsAndValues {
		if len(tmpValueList) == 0 {
			return fmt.Errorf("var %s has empty valueSet", varName)
		}
		*p.variableNames = append(*p.variableNames, varName)
		valueSet := varsAndValues[varName]
		if valueSet == nil {
			valueSet = map[variableValueKey]variableValue{}
			varsAndValues[varName] = valueSet
		}
		for _, value := range tmpValueList {
			valueSet[value.key()] = value
		}
	}
	if err = p.Variables.verify(); err != nil {
		return err
	}
	return nil
}

// variables represents variable as part of condition or top level in an isolate file.
type variables struct {
	Command []string `json:"command"`
	Files   []string `json:"files"`
	// ReadOnly has 1 as default, according to specs.
	// Just as Python-isolate also uses None as default, this code uses nil.
	ReadOnly *int `json:"read_only"`
}

func (p *variables) isEmpty() bool {
	return len(p.Command) == 0 && len(p.Files) == 0 && p.ReadOnly == nil
}

func (p *variables) verify() error {
	if p.ReadOnly == nil || (0 <= *p.ReadOnly && *p.ReadOnly <= 2) {
		return nil
	}
	return errors.New("read_only must be 0, 1, 2, or undefined.")
}

func parseIsolate(content []byte) (*parsedIsolate, error) {
	// Isolate file is a Python expression, which is easiest to interprete with cPython itself.
	// Go and Python both have excellent json support, so use this for Python->Go communication.
	// The isolate file contents is passed to Python's stdin, the resulting json is dumped into stdout.
	// In case of exceptions during interpretation or json serialization,
	// Python exists with non-0 return code, obtained here as err.
	pythonCode := `
import json, sys
globs = {'__builtins__': None}
locs = {}
try:
	value = eval(sys.stdin.read(), globs, locs)
except TypeError as e:
	e.args = list(e.args) + [content]
	raise
assert locs == {}, locs
assert globs == {'__builtins__': None}, globs
print json.dumps(value)
`
	f, err := ioutil.TempFile("", "isolate")
	_, _ = io.WriteString(f, pythonCode)
	f.Close()
	defer func() {
		_ = os.Remove(f.Name())
	}()
	jsonData, err := common.RunPython(content, f.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate isolate: %s", err)
	}
	parsed := &parsedIsolate{}
	if err := json.Unmarshal(jsonData, parsed); err != nil {
		return nil, fmt.Errorf("failed to parse isolate: %s; %s", err, jsonData)
	}
	return parsed, nil
}

// configName defines a config as an ordered set of bound and unbound variable values.
type configName []variableValue

func (lhs configName) compare(rhs configName) int {
	// Bound value is less than unbound one.
	assert(len(lhs) == len(rhs))
	for i, l := range lhs {
		if r := l.compare(rhs[i]); r != 0 {
			return r
		}
	}
	return 0
}

func (c configName) Equals(o configName) bool {
	if len(c) != len(o) {
		return false
	}
	return c.compare(o) == 0
}

func (c configName) key() string {
	parts := make([]string, 0, len(c))
	for _, v := range c {
		if !v.isBound() {
			parts = append(parts, "∀")
		} else {
			parts = append(parts, "∃", v.String())
		}
	}
	return strings.Join(parts, "\x00")
}

type configPair struct {
	key   configName
	value *ConfigSettings
}

// configPairs implements interface for sort package sorting.
type configPairs []configPair

func (c configPairs) Len() int {
	return len(c)
}

func (c configPairs) Less(i, j int) bool {
	return c[i].key.compare(c[j].key) > 0
}

func (c configPairs) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

// Configs represents a processed .isolate file.
//
// Stores the file in a processed way, split by configuration.
//
// At this point, we don't know all the possibilities. So mount a partial view
// that we have.
//
// This class doesn't hold isolateDir, since it is dependent on the final
// configuration selected. It is implicitly dependent on which .isolate defines
// the 'command' that will take effect.
type Configs struct {
	FileComment []byte
	// ConfigVariables contains names only, sorted by name; the order is same as in byConfig.
	ConfigVariables []string
	// The config key are lists of values of vars in the same order as ConfigSettings.
	byConfig map[string]configPair
}

// makeConfigsV deduces ConfigVariables from information collected during verification of isolate.
func makeConfigsV(fileComment []byte, varsAndValues variablesAndValues) *Configs {
	configVariables := make([]string, 0, len(varsAndValues))
	for varName := range varsAndValues {
		configVariables = append(configVariables, varName)
	}
	sort.Strings(configVariables)
	return makeConfigs(fileComment, configVariables)
}

func makeConfigs(fileComment []byte, configVariables []string) *Configs {
	c := &Configs{fileComment, configVariables, map[string]configPair{}}
	assert(sort.IsSorted(sort.StringSlice(c.ConfigVariables)))
	return c
}

func (c *Configs) getSortedConfigPairs() configPairs {
	pairs := make([]configPair, 0, len(c.byConfig))
	for _, pair := range c.byConfig {
		pairs = append(pairs, pair)
	}
	out := configPairs(pairs)
	sort.Sort(out)
	return out
}

// GetConfig returns all configs that matches this config as a single ConfigSettings.
//
// Returns nil if none apply.
func (c *Configs) GetConfig(configName configName) (*ConfigSettings, error) {
	// Order byConfig according to configNames ordering function.
	out := &ConfigSettings{}
	for _, pair := range c.getSortedConfigPairs() {
		ok := true
		for i, confKey := range configName {
			if pair.key[i].isBound() && pair.key[i].compare(confKey) != 0 {
				ok = false
				break
			}
		}
		if ok {
			var err error
			if out, err = out.union(pair.value); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// setConfig sets the ConfigSettings for this key.
//
// The key is a tuple of bounded or unbounded variables. The global variable
// is the key where all values are unbounded.
func (c *Configs) setConfig(confName configName, value *ConfigSettings) {
	assert(len(confName) == len(c.ConfigVariables))
	assert(value != nil)
	key := confName.key()
	pair, ok := c.byConfig[key]
	assert(!ok, "setConfig must not override existing keys (%s => %v)", key, pair.value)
	c.byConfig[key] = configPair{confName, value}
}

// union returns a new Configs instance, the union of variables from self and rhs.
//
// Uses lhs.FileComment if available, otherwise rhs.FileComment.
// It keeps ConfigVariables sorted in the output.
func (lhs *Configs) union(rhs *Configs) (*Configs, error) {
	// Merge the keys of ConfigVariables for each Configs instances. All the new
	// variables will become unbounded. This requires realigning the keys.
	configVariables := uniqueMergeSortedStrings(
		lhs.ConfigVariables, rhs.ConfigVariables)
	out := makeConfigs(lhs.FileComment, configVariables)
	if len(lhs.FileComment) == 0 {
		out.FileComment = rhs.FileComment
	}

	byConfig := configPairs(append(
		lhs.expandConfigVariables(configVariables),
		rhs.expandConfigVariables(configVariables)...))
	if len(byConfig) == 0 {
		return out, nil
	}
	// Take union of ConfigSettings with the same configName (key),
	// in order left, right.
	// Thus, preserve the order between left, right while sorting.
	sort.Stable(byConfig)
	last := byConfig[0]
	for _, curr := range byConfig[1:] {
		if last.key.compare(curr.key) == 0 {
			if val, err := last.value.union(curr.value); err != nil {
				return out, err
			} else {
				last.value = val
			}
		} else {
			out.setConfig(last.key, last.value)
			last = curr
		}
	}
	out.setConfig(last.key, last.value)
	return out, nil
}

// expandConfigVariables returns new configPair list for newConfigVars.
func (c *Configs) expandConfigVariables(newConfigVars []string) []configPair {
	// Get mapping from old config vars list to new one.
	mapping := make([]int, len(newConfigVars))
	i := 0
	for n, nk := range newConfigVars {
		if i == len(c.ConfigVariables) || c.ConfigVariables[i] > nk {
			mapping[n] = -1
		} else if c.ConfigVariables[i] == nk {
			mapping[n] = i
			i++
		} else {
			// Must never happen because newConfigVars and c.configVariables are sorted ASC,
			// and newConfigVars contain c.configVariables as a subset.
			panic("unreachable code")
		}
	}
	// Expands configName to match newConfigVars.
	getNewconfigName := func(old configName) configName {
		newConfig := make(configName, len(mapping))
		for k, v := range mapping {
			if v != -1 {
				newConfig[k] = old[v]
			}
		}
		return newConfig
	}
	// Compute new byConfig.
	out := make([]configPair, 0, len(c.byConfig))
	for _, pair := range c.byConfig {
		out = append(out, configPair{getNewconfigName(pair.key), pair.value})
	}
	return out
}

func createReadOnlyValue(readOnly *int) ReadOnlyValue {
	if readOnly == nil {
		return NotSet
	}
	return ReadOnlyValue(*readOnly)
}

// ConfigSettings represents the dependency variables for a single build configuration.
//
// The structure is immutable.
type ConfigSettings struct {
	// Files is the list of dependencies. The items use '/' as a path separator.
	Files []string
	// Command is the actual command to run.
	Command []string
	// ReadOnly describes how to map the files.
	ReadOnly ReadOnlyValue
	// IsolateDir is the path where to start the command from.
	// It uses the OS' native path separator and it must be an absolute path.
	IsolateDir string
}

func createConfigSettings(values variables, isolateDir string) *ConfigSettings {
	if isolateDir == "" {
		// It must be an empty object if isolateDir is not set.
		assert(values.isEmpty(), values)
	} else {
		assert(filepath.IsAbs(isolateDir))
	}
	c := &ConfigSettings{
		make([]string, len(values.Files)),
		values.Command,
		createReadOnlyValue(values.ReadOnly),
		isolateDir,
	}
	copy(c.Files, values.Files)
	sort.Strings(c.Files)
	return c
}

// union merges two config settings together into a new instance.
//
// A new instance is not created and self or rhs is returned if the other
// object is the empty object.
//
// self has priority over rhs for Command. Use the same IsolateDir as the
// one having a Command.
//
// Dependencies listed in rhs are patch adjusted ONLY if they don't start with
// a path variable, e.g. the characters '<('.
func (lhs *ConfigSettings) union(rhs *ConfigSettings) (*ConfigSettings, error) {
	// When an object has IsolateDir == "", it means it is the empty object.
	if lhs.IsolateDir == "" {
		return rhs, nil
	}
	if rhs.IsolateDir == "" {
		return lhs, nil
	}

	if common.IsWindows() && strings.ToLower(lhs.IsolateDir)[0] != strings.ToLower(rhs.IsolateDir)[0] {
		return nil, errors.New("All .isolate files must be on same drive")
	}

	// Takes the difference between the two isolateDir. Note that while
	// isolateDir is in native path case, all other references are in posix.
	useRhs := false
	var command []string
	if len(lhs.Command) > 0 {
		useRhs = false
		command = lhs.Command
	} else if len(rhs.Command) > 0 {
		useRhs = true
		command = rhs.Command
	} else {
		// If self doesn't define any file, use rhs.
		useRhs = len(lhs.Files) == 0
	}

	readOnly := rhs.ReadOnly
	if lhs.ReadOnly != NotSet {
		readOnly = lhs.ReadOnly
	}

	lRelCwd, rRelCwd := lhs.IsolateDir, rhs.IsolateDir
	lFiles, rFiles := lhs.Files, rhs.Files
	if useRhs {
		// Rebase files in rhs.
		lRelCwd, rRelCwd = rhs.IsolateDir, lhs.IsolateDir
		lFiles, rFiles = rhs.Files, lhs.Files
	}

	rebasePath, err := filepath.Rel(rRelCwd, lRelCwd)
	if err != nil {
		return nil, err
	}
	rebasePath = strings.Replace(rebasePath, osPathSeparator, "/", 0)

	filesSet := map[string]bool{}
	for _, f := range lFiles {
		filesSet[f] = true
	}
	for _, f := range rFiles {
		// Rebase item.
		if !(strings.HasPrefix(f, "<(") || rebasePath == ".") {
			// paths are posix here.
			f = path.Join(rebasePath, f)
		}
		filesSet[f] = true
	}
	// Remove duplicates.
	files := make([]string, 0, len(filesSet))
	for f := range filesSet {
		files = append(files, f)
	}
	sort.Strings(files)
	return &ConfigSettings{files, command, readOnly, lRelCwd}, nil
}

// LoadIsolateAsConfig parses one .isolate file and returns a Configs instance.
//
//  Arguments:
//    isolateDir: only used to load relative includes so it doesn't depend on
//                 cwd.
//    value: is the loaded dictionary that was defined in the gyp file.
//    fileComment: comments found at the top of the file so it can be preserved.
//
//  The expected format is strict, anything diverting from the format below will result in error:
//  {
//    'includes': [
//      'foo.isolate',
//    ],
//    'conditions': [
//      ['OS=="vms" and foo=42', {
//        'variables': {
//          'command': [
//            ...
//          ],
//          'files': [
//            ...
//          ],
//          'read_only': 0,
//        },
//      }],
//      ...
//    ],
//    'variables': {
//      ...
//    },
//  }
func LoadIsolateAsConfig(isolateDir string, content []byte, fileComment []byte) (*Configs, error) {
	// isolateDir must be in native style.
	if !filepath.IsAbs(isolateDir) {
		return nil, fmt.Errorf("%s is not an absolute path", isolateDir)
	}
	parsed, err := parseIsolate(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse isolate (isolateDir: %s): %s", isolateDir, err)
	}
	varsAndValues, err := parsed.verify()
	if err != nil {
		return nil, fmt.Errorf("failed to verify isolate (isolateDir: %s): %s", isolateDir, err)
	}
	isolate := makeConfigsV(fileComment, varsAndValues)
	// Add global variables. The global variables are on the empty tuple key.
	globalconfigName := make([]variableValue, len(isolate.ConfigVariables))
	if parsed.Variables != nil {
		isolate.setConfig(globalconfigName, createConfigSettings(*parsed.Variables, isolateDir))
	} else {
		isolate.setConfig(globalconfigName, createConfigSettings(variables{}, isolateDir))
	}
	// Add configuration-specific variables.
	allConfigs, err := varsAndValues.cartesianProductOfValues(isolate.ConfigVariables)
	if err != nil {
		return nil, err
	}
	for _, cond := range parsed.Conditions {
		configs, err := matchConfigs(cond.Condition, isolate.ConfigVariables, allConfigs)
		if err != nil {
			return nil, err
		}
		newConfigs := makeConfigs(nil, isolate.ConfigVariables)
		for _, config := range configs {
			newConfigs.setConfig(configName(config), createConfigSettings(cond.Variables, isolateDir))
		}
		if isolate, err = isolate.union(newConfigs); err != nil {
			return nil, err
		}
	}
	// If the .isolate contains command, ignore any command in child .isolate.
	rootHasCommand := false
	for _, pair := range isolate.byConfig {
		if len(pair.value.Command) > 0 {
			rootHasCommand = true
			break
		}
	}
	// Load the includes. Process them in reverse so the last one take precedence.
	for i := len(parsed.Includes) - 1; i >= 0; i-- {
		if included, err := loadIncludedIsolate(isolateDir, parsed.Includes[i]); err != nil {
			return nil, err
		} else {
			if rootHasCommand {
				// Strip any command in the imported isolate. It is because the chosen
				// command is not related to the one in the top-most .isolate, since the
				// configuration is flattened.
				for _, pair := range included.byConfig {
					pair.value.Command = []string{}
				}
			}
			if isolate, err = isolate.union(included); err != nil {
				return nil, err
			}
		}
	}
	return isolate, nil
}

func loadIncludedIsolate(isolateDir, include string) (*Configs, error) {
	if filepath.IsAbs(include) {
		return nil, fmt.Errorf("failed to load configuration; absolute include path %s", include)
	}
	includedIsolate := filepath.Clean(filepath.Join(isolateDir, include))
	if common.IsWindows() && (strings.ToLower(includedIsolate)[0] != strings.ToLower(isolateDir)[0]) {
		return nil, errors.New("can't reference a .isolate file from another drive")
	}
	content, err := ioutil.ReadFile(includedIsolate)
	if err != nil {
		return nil, err
	}
	return LoadIsolateAsConfig(filepath.Dir(includedIsolate), content, nil)
}

// LoadIsolateForConfig loads the .isolate file and returns
// the information unprocessed but filtered for the specific OS.
//
// Returns:
//   command, dependencies, readOnly flag, relDir, error.
//
// relDir and dependencies are fixed to use os.PathSeparator.
func LoadIsolateForConfig(isolateDir string, content []byte, configVariables common.KeyValVars) (
	[]string, []string, ReadOnlyValue, string, error) {
	// Load the .isolate file, process its conditions, retrieve the command and dependencies.
	isolate, err := LoadIsolateAsConfig(isolateDir, content, nil)
	if err != nil {
		return nil, nil, NotSet, "", err
	}
	configName := configName{}
	missingVars := []string{}
	for _, variable := range isolate.ConfigVariables {
		if value, ok := configVariables[variable]; ok {
			configName = append(configName, createVariableValueTryInt(value))
		} else {
			missingVars = append(missingVars, variable)
		}
	}
	if len(missingVars) > 0 {
		sort.Strings(missingVars)
		err = fmt.Errorf("these configuration variables were missing from the command line: %v", missingVars)
		return nil, nil, NotSet, "", err
	}
	// A configuration is to be created with all the combinations of free variables.
	config, err := isolate.GetConfig(configName)
	if err != nil {
		return nil, nil, NotSet, "", err
	}
	dependencies := config.Files
	relDir := config.IsolateDir
	if os.PathSeparator != '/' {
		dependencies = make([]string, len(config.Files))
		for i, f := range config.Files {
			dependencies[i] = strings.Replace(f, "/", osPathSeparator, -1)
		}
		relDir = strings.Replace(relDir, "/", osPathSeparator, -1)
	}
	return config.Command, dependencies, config.ReadOnly, relDir, nil
}
