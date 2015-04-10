// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"fmt"
	"strings"

	"github.com/kr/pretty"
)

// Strings accumulates string values from repeated flags.
//
// Use with flag.Var to accumulate values from "-flag s1 -flag s2".
type Strings []string

func (c *Strings) String() string {
	return pretty.Sprintf("%v", []string(*c))
}

func (c *Strings) Set(value string) error {
	*c = append(*c, value)
	return nil
}

// KeyValVars is a set of variables names and values for isolate processing.
//
// It can be used with flag.Var(). In that case, it accumulates multiple
// key-value for a given flag.
//
// Use with flag.Var() to accumulate values from
// "--flag key=value --flag foo=bar".
//
// If the same key appears several times, the value of last occurence is used.
type KeyValVars map[string]string

func (c KeyValVars) String() string {
	return pretty.Sprintf("%v", map[string]string(c))
}

func (c KeyValVars) Set(value string) error {
	kv := strings.SplitN(value, "=", 2)
	if len(kv) != 2 {
		return fmt.Errorf("please use FOO=BAR for key-value argument")
	}
	key, value := kv[0], kv[1]
	c[key] = value
	return nil
}
