// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"fmt"
)

// ProjectConfigSet returns the name of the ConfigSet associated with project.
func ProjectConfigSet(project ProjectName) string {
	return fmt.Sprintf("projects/%s", project)
}
