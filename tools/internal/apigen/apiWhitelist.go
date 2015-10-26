// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apigen

import (
	"errors"
	"flag"
	"strings"
)

// apiWhitelist is a flag.Value type that appends to the list on add.
type apiWhitelist []string

var _ flag.Value = (*apiWhitelist)(nil)

func (w *apiWhitelist) String() string {
	if *w == nil {
		return ""
	}
	return strings.Join(*w, ",")
}

func (w *apiWhitelist) Set(s string) error {
	if len(s) == 0 {
		return errors.New("cannot whitelist empty string")
	}

	*w = append(*w, s)
	return nil
}
