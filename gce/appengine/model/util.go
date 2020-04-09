// Copyright 2020 The LUCI Authors.
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

package model

import (
	"go.chromium.org/luci/gce/api/config/v1"
	project "go.chromium.org/luci/gce/api/projects/v1"
)

// CopyToBinaryInConfig copies the config field to binary config field in Config.
func CopyToBinaryInConfig(cfg *Config) {
	cfg.BinaryConfig = config.BinaryConfig{
		cfg.Config,
	}
}

// CopyToBinaryInProject copies the config field to binary config field in Project.
func CopyToBinaryInProject(p *Project) {
	p.BinaryConfig = project.BinaryConfig{
		p.Config,
	}
}

// CopyToBinaryInVM copies the attributes field to binary attributes field in VM.
func CopyToBinaryInVM(vm *VM) {
	vm.BinaryAttributes = config.BinaryVM{
		vm.Attributes,
	}
}
