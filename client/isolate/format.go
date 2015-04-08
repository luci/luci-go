// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/luci/luci-go/client/internal/common"
)

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
		return fmt.Sprintf("%v", *v.S)
	} else if v.I != nil {
		return fmt.Sprintf("%d", *v.I)
	}
	return ""
}

func (lhs variableValue) compare(rhs variableValue) int {
	switch {
	case lhs.isBound() && rhs.isBound():
		l, r := lhs.String(), rhs.String()
		if l < r {
			return -1
		} else if l > r {
			return 1
		}
		return 0
	case lhs.isBound():
		return -1
	case rhs.isBound():
		return 1
	default:
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
		return variableValueKey("S" + *v.S)
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
	for key, _ := range valueSet {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	values := make([]variableValue, len(valueSet))
	for i, key := range keys {
		values[i] = valueSet[variableValueKey(key)]
	}
	return values, true
}

func (v variablesAndValues) cartesianProductOfValues(orderedKeys []string) [][]variableValue {
	if len(orderedKeys) == 0 {
		return [][]variableValue{}
	}
	// Prepare ordered by orderedKeys list of variableValue
	allValues := make([][]variableValue, 0, len(v))
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
	for _, values := range v {
		length *= len(values)
	}
	assert(length > 0, "some variable had empty valuesSet?!")
	out := make([][]variableValue, 0, length)
	// indices[i] points to index in allValues[i]; stop once indices[-1] == len(allValues[-1]).
	indices := make([]int, len(v))
	for {
		next := make([]variableValue, len(v))
		for i, values := range allValues {
			if indices[i] == len(values) {
				if i+1 == len(orderedKeys) {
					return out
				}
				indices[i] = 0
				indices[i+1]++
			}
			next[i] = values[indices[i]]
		}
		out = append(out, next)
	}
	panic("unreachable code")
	return nil
}

func matchConfigs(condition string, configVariables []string, allConfigs [][]variableValue) [][]variableValue {
	// TODO: get rid of Python here...
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
	m := map[string]interface{}{}
	m["cond"] = condition
	m["conf"] = configVariables
	m["a"] = allConfigs
	jsonDataIn, err := json.Marshal(m)
	assertNoError(err)
	jsonDataOut, err := common.RunPython(jsonDataIn, "-c", pythonCode)
	assertNoError(err)
	var out [][]variableValue
	err = json.Unmarshal(jsonDataOut, &out)
	assertNoError(err)
	return out
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
	// Helper to store variable names in Condition strings, set by Verify method.
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
			val = {'I': rhs.n}
		else:
			val = {'S': rhs.s}
			var_values.append(val)

test_ast = compile(sys.stdin.read(), '<condition>', 'eval', ast.PyCF_ONLY_AST)
variables_and_values = {}
verify_ast(test_ast.body, variables_and_values)
print json.dumps(variables_and_values)
`
	jsonData, err := common.RunPython([]byte(p.Condition), "-c", pythonCode)
	if err != nil {
		return fmt.Errorf("failed to verify Condition string %s: %s", p.Condition, err)
	}
	tmpVarsAndValues := map[string][]variableValue{}
	err = json.Unmarshal(jsonData, &tmpVarsAndValues)
	assert(err == nil)
	p.variableNames = new([]string)
	for varName, tmpValueList := range tmpVarsAndValues {
		*p.variableNames = append(*p.variableNames, varName)
		valueSet := varsAndValues[varName]
		if valueSet == nil {
			valueSet = make(map[variableValueKey]variableValue)
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
	// read_only has 1 as default, according to specs.
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
	jsonData, err := common.RunPython(content, "-c", "import json, sys; print json.dumps(eval(sys.stdin.read()))")
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate isolate: %s", err)
	}
	parsed := &parsedIsolate{}
	if err := json.Unmarshal(jsonData, parsed); err != nil {
		return nil, fmt.Errorf("failed to parse isolate: %s", err)
	}
	return parsed, nil
}
