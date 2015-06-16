// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"flag"
	"strings"
)

const (
	// FilterOnAll is a Filter value that, when included in a Filter, causes it to
	// display all log messages.
	FilterOnAll = "all"

	// FilterOnKey is a log field key whose value specifies the context's filter.
	FilterOnKey = "_filterOn"
)

// Filter is a configurable list of strings to filter on, when the 'filterOn'
// property is supplied.
type Filter []string

var _ flag.Value = (*Filter)(nil)

// Set implements flag.Value.
func (f *Filter) Set(v string) error {
	if len(v) > 0 {
		*f = append(*f, v)
	}
	return nil
}

// String implements flag.Value.
func (f *Filter) String() string {
	return strings.Join([]string(*f), ",")
}

// Get returns a filter function that returns true if the Filter passes the
// supplied log flags.
func (f Filter) Get() func(interface{}) bool {
	filterMap := make(map[string]bool)
	for _, filter := range f {
		if filter == FilterOnAll {
			filterMap = nil
			break
		}
		filterMap[filter] = true
	}

	return func(filterOn interface{}) bool {
		if filterMap == nil || filterOn == nil {
			return true
		}

		switch t := filterOn.(type) {
		case string:
			_, ok := filterMap[t]
			return ok

		default:
			return true
		}
	}
}
