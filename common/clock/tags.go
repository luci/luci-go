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

package clock

import "context"

var clockTagKey = "clock-tag"

// Tag returns a derivative Context with the supplied Tag appended to it.
//
// Tag chains can be used by timers to identify themselves.
func Tag(c context.Context, v string) context.Context {
	return context.WithValue(c, &clockTagKey, append(Tags(c), v))
}

// Tags returns a copy of the set of tags in the current Context.
func Tags(c context.Context) []string {
	if tags, ok := c.Value(&clockTagKey).([]string); ok && len(tags) > 0 {
		tclone := make([]string, len(tags))
		copy(tclone, tags)
		return tclone
	}
	return nil
}
