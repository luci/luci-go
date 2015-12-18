// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils contains a bunch of small functions used by task/ subpackages.
package utils

import (
	"fmt"
	"strings"
)

// KV is key and value strings.
type KV struct {
	Key   string
	Value string
}

// ValidateKVList makes sure each string in the list is valid key-value pair.
func ValidateKVList(kind string, list []string, sep rune) error {
	for _, item := range list {
		if !strings.ContainsRune(item, sep) {
			return fmt.Errorf("bad %s, not a 'key%svalue' pair: %q", kind, string(sep), item)
		}
	}
	return nil
}

// UnpackKVList takes validated list of k-v pair strings and returns list of
// structs.
//
// Silently skips malformed strings. Use ValidateKVList to detect them before
// calling this function.
func UnpackKVList(list []string, sep rune) (out []KV) {
	for _, item := range list {
		idx := strings.IndexRune(item, sep)
		if idx == -1 {
			continue
		}
		out = append(out, KV{
			Key:   item[:idx],
			Value: item[idx+1:],
		})
	}
	return out
}
