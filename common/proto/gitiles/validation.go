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
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Validate returns an error if r is invalid.
func (r *LogRequest) Validate() error {
	if err := requireProject(r.GetProject()); err != nil {
		return err
	}
	if err := requireCommittish("committish", r.GetCommittish()); err != nil {
		return err
	}
	switch {
	case strings.Contains(r.Committish, ".."):
		return errors.New("committish cannot contain \"..\"; use Ancestor instead")
	case r.PageSize < 0:
		return errors.New("page size must not be negative")
	default:
		return nil
	}
}

// Validate returns an error if r is invalid.
func (r *RefsRequest) Validate() error {
	if err := requireProject(r.GetProject()); err != nil {
		return err
	}
	switch {
	case r.RefsPath != "refs" && !strings.HasPrefix(r.RefsPath, "refs/"):
		return errors.Fmt(`refsPath must be "refs" or start with "refs/": %q`, r.RefsPath)
	default:
		return nil
	}
}

// Validate returns an error if r is invalid.
func (r *ArchiveRequest) Validate() error {
	if err := requireProject(r.GetProject()); err != nil {
		return err
	}
	switch {
	case r.Format == ArchiveRequest_Invalid:
		return errors.New("format must be valid")
	case r.Ref == "":
		return errors.New("ref is required")
	case strings.HasPrefix(r.Path, "/"):
		return errors.New("path must not start with /")
	default:
		return nil
	}
}

// Validate returns an error if r is invalid.
func (r *DownloadFileRequest) Validate() error {
	if err := requireProject(r.GetProject()); err != nil {
		return err
	}
	if err := requireCommittish("committish", r.GetCommittish()); err != nil {
		return err
	}
	if strings.HasPrefix(r.Path, "/") {
		return errors.New("path must not start with /")
	}
	return nil
}

// Validate returns an error if r is invalid.
func (r *DownloadDiffRequest) Validate() error {
	if err := requireProject(r.GetProject()); err != nil {
		return err
	}
	if err := requireCommittish("committish", r.GetCommittish()); err != nil {
		return err
	}
	if base := r.GetBase(); base != "" {
		if err := requireCommittish("base", base); err != nil {
			return err
		}
	}
	if strings.HasPrefix(r.Path, "/") {
		return errors.New("path must not start with /")
	}
	return nil
}

// Validate returns an error if r is invalid.
func (r *ListFilesRequest) Validate() error {
	if err := requireProject(r.GetProject()); err != nil {
		return err
	}
	if err := requireCommittish("committish", r.GetCommittish()); err != nil {
		return err
	}
	if strings.HasSuffix(r.GetCommittish(), "/") {
		return errors.New("committish must not end with /")
	}
	if strings.HasPrefix(r.Path, "/") {
		return errors.New("path must not start with /")
	}
	return nil
}

func requireProject(val string) error {
	if val == "" {
		return errors.New("project is required")
	}
	return nil
}

func requireCommittish(field, val string) error {
	switch {
	case val == "":
		return errors.Fmt("%s is required", field)
	case strings.HasPrefix(val, "/"):
		return errors.Fmt("%s must not start with /", field)
	}
	return nil
}
