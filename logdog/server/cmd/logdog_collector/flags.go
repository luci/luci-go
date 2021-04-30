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

package main

import (
	"flag"

	"go.chromium.org/luci/logdog/server/service"
)

// CommandLineFlags contains collector service configuration.
//
// It is exposed via CLI flags.
type CommandLineFlags struct {
	// Storage contains the intermediate storage (e.g. BigTable) flags.
	//
	// All fields are required.
	Storage service.StorageFlags

	// TODO(crbug.com/1204268): Move the rest.
}

// Register registers flags in the flag set.
func (f *CommandLineFlags) Register(fs *flag.FlagSet) {
	f.Storage.Register(fs)
}

// Validate returns an error if some parsed flags have invalid values.
func (f *CommandLineFlags) Validate() error {
	if err := f.Storage.Validate(); err != nil {
		return err
	}
	return nil
}
