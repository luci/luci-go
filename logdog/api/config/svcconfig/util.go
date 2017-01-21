// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
