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

package main

import (
	"go.chromium.org/luci/common/flag/multiflag"

	"go.chromium.org/luci/logdog/client/butler/output"
	logOutput "go.chromium.org/luci/logdog/client/butler/output/log"
)

func init() {
	registerOutputFactory(&logOutputFactory{})
}

type logOutputFactory struct {
	bundleSize int
}

var _ outputFactory = (*logOutputFactory)(nil)

func (f *logOutputFactory) option() multiflag.Option {
	opt := newOutputOption("log", "Debug output that writes to STDOUT.", f)

	flags := opt.Flags()
	flags.IntVar(&f.bundleSize, "bundle-size", 1024*1024,
		"Maximum bundle size.")

	return opt
}

func (f *logOutputFactory) configOutput(a *application) (output.Output, error) {
	return logOutput.New(a, f.bundleSize), nil
}

func (f *logOutputFactory) scopes() []string { return nil }
