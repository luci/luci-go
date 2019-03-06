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
	"errors"

	"go.chromium.org/luci/common/flag/multiflag"
	"go.chromium.org/luci/logdog/client/butler/output"
	fileOutput "go.chromium.org/luci/logdog/client/butler/output/file"
)

func init() {
	registerOutputFactory(&fileOutputFactory{})
}

type fileOutputFactory struct {
	fileOutput.Options
}

func (f *fileOutputFactory) option() multiflag.Option {
	opt := newOutputOption("file", "Debug output that writes stream data to a single protobuf file.", f)

	flags := opt.Flags()
	flags.StringVar(&f.Path, "path", "", "Stream output text protobuf path.")
	flags.BoolVar(&f.Track, "track", false,
		"Track each sent message and dump at the end. This adds CPU/memory overhead.")

	return opt
}

func (f *fileOutputFactory) configOutput(a *application) (output.Output, error) {
	if f.Path == "" {
		return nil, errors.New("missing required output path")
	}
	return f.New(a), nil
}

func (f *fileOutputFactory) scopes() []string { return nil }
