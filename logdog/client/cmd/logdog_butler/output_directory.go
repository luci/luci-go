// Copyright 2019 The LUCI Authors.
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

package main

import (
	"errors"

	"go.chromium.org/luci/common/flag/multiflag"

	"go.chromium.org/luci/logdog/client/butler/output"
	dirOutput "go.chromium.org/luci/logdog/client/butler/output/directory"
)

func init() {
	registerOutputFactory(&dirOutputFactory{})
}

type dirOutputFactory struct {
	dirOutput.Options
}

func (d *dirOutputFactory) option() multiflag.Option {
	opt := newOutputOption("directory", "Debug output that writes stream data to multiple files.", d)

	flags := opt.Flags()
	flags.StringVar(&d.Path, "path", "", "Base directory for all output files.")

	return opt
}

func (d *dirOutputFactory) configOutput(a *application) (output.Output, error) {
	if d.Path == "" {
		return nil, errors.New("missing required output path")
	}
	return d.New(a), nil
}

func (d *dirOutputFactory) scopes() []string { return nil }
