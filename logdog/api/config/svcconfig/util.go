// Copyright 2016 The LUCI Authors.
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

package svcconfig

import (
	"fmt"
)

const (
	// ServiceConfigPath is the config service path of the Config protobuf.
	ServiceConfigPath = "services.cfg"
)

// ProjectConfigPath returns the path of a LogDog project config given the
// LogDog service's name.
func ProjectConfigPath(serviceName string) string { return fmt.Sprintf("%s.cfg", serviceName) }
