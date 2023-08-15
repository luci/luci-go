// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package actions

import (
	"io/fs"
)

// ReadLinkFS is the interface implemented by a file system that supports
// symbolic links.
// TODO(fancl): Replace it with https://github.com/golang/go/issues/49580
type ReadLinkFS interface {
	fs.FS

	// ReadLink returns the destination of the named symbolic link.
	// Link destinations will always be slash-separated paths.
	// NOTE: Although in the standard library the link destination is guaranteed
	// to be a path inside FS. We may return host destination for our use cases.
	ReadLink(name string) (string, error)
}
