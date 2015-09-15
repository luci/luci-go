// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"github.com/kr/pretty"
)

// Strings accumulates string values from repeated flags.
//
// Use with flag.Var to accumulate values from "-flag s1 -flag s2".
type Strings []string

func (c *Strings) String() string {
	return pretty.Sprintf("%v", []string(*c))
}

// Set is needed to implements flag.Var interface.
func (c *Strings) Set(value string) error {
	*c = append(*c, value)
	return nil
}
