// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"errors"
	"regexp"

	"github.com/luci/luci-go/client/archiver"
	"github.com/luci/luci-go/client/internal/common"
)

// IsolatedGenJSONVersion is used in the batcharchive json format.
//
// TODO(tandrii): Migrate to batch_archive.go.
const IsolatedGenJSONVersion = 1

// ValidVariable is the regexp of valid isolate variable name.
const ValidVariable = "[A-Za-z_][A-Za-z_0-9]*"

var validVariableMatcher = regexp.MustCompile(ValidVariable)
var variableSubstitutionMatcher = regexp.MustCompile("<\\(" + ValidVariable + "\\)")

// IsValidVariable returns true if the variable is a valid symbol name.
func IsValidVariable(variable string) bool {
	return validVariableMatcher.MatchString(variable)
}

// Tree to be isolated.
type Tree struct {
	Cwd  string
	Opts ArchiveOptions
}

// ArchiveOptions for achiving trees.
type ArchiveOptions struct {
	Isolate         string            `json:"isolate"`
	Isolated        string            `json:"isolated"`
	Blacklist       common.Strings    `json:"blacklist"`
	PathVariables   common.KeyValVars `json:"path_variables"`
	ExtraVariables  common.KeyValVars `json:"extra_variables"`
	ConfigVariables common.KeyValVars `json:"config_variables"`
}

// Init initializes with non-nil values.
func (a *ArchiveOptions) Init() {
	a.Blacklist = common.Strings{}
	a.PathVariables = common.KeyValVars{}
	a.ExtraVariables = common.KeyValVars{}
	a.ConfigVariables = common.KeyValVars{}
}

// ReplaceVariables replaces any occurrences of '<(FOO)' in 'str' with the
// corresponding variable from 'opts'.
//
// If any substitution refers to a variable that is missing, the returned error will
// refer to the first such variable. In the case of errors, the returned string will
// still contain a valid result for any non-missing substitutions.
func ReplaceVariables(str string, opts ArchiveOptions) (string, error) {
	var err error
	subst := variableSubstitutionMatcher.ReplaceAllStringFunc(str,
		func(match string) string {
			var_name := match[2 : len(match)-1]
			if v, ok := opts.PathVariables[var_name]; ok {
				return v
			}
			if v, ok := opts.ExtraVariables[var_name]; ok {
				return v
			}
			if v, ok := opts.ConfigVariables[var_name]; ok {
				return v
			}
			if err == nil {
				err = errors.New("no value for variable '" + var_name + "'")
			}
			return match
		})
	return subst, err
}

// Archive processes a .isolate, generates a .isolated and archive it.
// Returns a Future to the .isolated.
func Archive(arch archiver.Archiver, opts *ArchiveOptions) archiver.Future {
	// TODO(tandrii): Implement.
	// cmd, deps, readOnly, isolateDir := isolate.LoadIsolateForConfig(data.Dir, content, variables)
	// for _, dep := range deps {
	//   ..
	// }
	// i := isolated.Isolated{}
	// <Serialize>
	// return arch.Push(encoded, strings.SplitN(filepath.Base(opts.Isolate), ".", 2)[0])
	s := archiver.NewSimpleFuture()
	s.Finalize("", errors.New("TODO"))
	return s
}
