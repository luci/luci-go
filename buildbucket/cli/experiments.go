// Copyright 2021 The LUCI Authors.
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

package cli

import (
	"flag"
	"sort"
	"strings"

	"go.chromium.org/luci/common/errors"
)

type experimentsFlag struct {
	experiments map[string]bool
}

func (f *experimentsFlag) Register(fs *flag.FlagSet, help string) {
	f.experiments = map[string]bool{}
	fs.Var(f, "ex", help)
}

func (f *experimentsFlag) Set(exp string) error {
	if len(exp) < 2 {
		return errors.Fmt("expected [+-]experiment_name, got %q", exp)
	}
	switch plusMinus, expname := exp[0], exp[1:]; plusMinus {
	case '+':
		f.experiments[expname] = true
	case '-':
		f.experiments[expname] = false
	default:
		return errors.Fmt("expected [+-]experiment_name, got %q", exp)
	}
	return nil
}

func (f *experimentsFlag) String() string {
	return strings.Join(f.experimentsFlat(), ", ")
}

func (f *experimentsFlag) experimentsFlat() []string {
	bits := make([]string, 0, len(f.experiments))
	for exp, enabled := range f.experiments {
		if enabled {
			bits = append(bits, "+"+exp)
		} else {
			bits = append(bits, "-"+exp)
		}
	}
	sort.Strings(bits)
	return bits
}
