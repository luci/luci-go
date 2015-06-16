// Copyright (c) 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nestedflagset

import (
	"strings"
)

// Token is a single name[=value] string.
type token string

// split breaks a token into its name and value components.
func (t token) split() (name, value string) {
	split := strings.SplitN(string(t), "=", 2)
	name = split[0]
	if len(split) == 2 {
		value = split[1]
	}
	return
}
