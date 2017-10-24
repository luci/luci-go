// Copyright 2017 The LUCI Authors.
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

package utils

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors"
)

const (
	maxTagCount     = 64
	maxTagKeySize   = 128
	maxTagValueSize = 1024
)

// ValidateTags returns an error if some tags are malformed.
//
// Tags are "key:value" pairs that can be passed with some RPCs. They end up
// inside the tokens and/or BigQuery logs.
func ValidateTags(tags []string) error {
	if len(tags) > maxTagCount {
		return fmt.Errorf("too many tags given (%d), expecting up to %d tags", len(tags), maxTagCount)
	}
	var merr errors.MultiError
	for i, tag := range tags {
		parts := strings.SplitN(tag, ":", 2) // ':' in values are allowed
		msg := ""
		switch {
		case len(parts) != 2:
			msg = "not in <key>:<value> form"
		case len(parts[0]) > maxTagKeySize:
			msg = fmt.Sprintf("the key length must not exceed %d", maxTagKeySize)
		case len(parts[1]) > maxTagValueSize:
			msg = fmt.Sprintf("the value length must not exceed %d", maxTagValueSize)
		}
		if msg != "" {
			merr = append(merr, fmt.Errorf("tag #%d: %s", i+1, msg))
		}
	}
	if len(merr) == 0 {
		return nil
	}
	return merr
}
