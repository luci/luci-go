// Copyright 2018 The LUCI Authors.
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

package gitiles

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Validate returns an error if r is invalid.
func (r *LogRequest) Validate() error {
	switch {
	case r.Project == "":
		return errors.New("project is required")
	case r.PageSize < 0:
		return errors.New("page size must not be negative")
	case r.Committish == "":
		return errors.New("committish is required")
	case strings.Contains(r.Committish, ".."):
		return errors.New("committish cannot contain \"..\"; use Ancestor instead")
	default:
		return nil
	}
}

// Validate returns an error if r is invalid.
func (r *RefsRequest) Validate() error {
	switch {
	case r.Project == "":
		return errors.New("project is required")
	case r.RefsPath != "refs" && !strings.HasPrefix(r.RefsPath, "refs/"):
		return fmt.Errorf(`refsPath must be "refs" or start with "refs/": %q`, r.RefsPath)
	default:
		return nil
	}
}

// Validate returns an error if r is invalid.
func (r *ArchiveRequest) Validate() error {
	switch {
	case r.Format == ArchiveRequest_Invalid:
		return errors.New("format must be valid")
	case r.Project == "":
		return errors.New("project is required")
	case r.Ref == "":
		return errors.New("ref is required")
	default:
		return nil
	}
}

// Validate returns an error if r is invalid.
func (r *DownloadFileRequest) Validate() error {
	switch {
	case r.Format != DownloadFileRequest_TEXT:
		return errors.Reason("format must be %s", DownloadFileRequest_TEXT.String()).Err()
	case r.Project == "":
		return errors.New("project is required")
	case r.Committish == "":
		return errors.New("committish is required")
	case strings.HasPrefix(r.Committish, "/"):
		return errors.New("committish must not start with /")
	case strings.HasPrefix(r.Path, "/"):
		return errors.New("path must not start with /")
	default:
		return nil
	}
}
