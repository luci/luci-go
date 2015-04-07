// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/luci/luci-go/client/internal/common"
)

type parsedIsolate struct {
	Includes   []string
	Conditions []condition
	Variables  *variables
}

// condition represents conditional part of an isolate file.
type condition struct {
	Condition string
	Variables variables
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
