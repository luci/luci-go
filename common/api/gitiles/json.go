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

package gitiles

import (
	"encoding/json"
	"time"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/git"
)

// This file implements structs to parse Gitiles' JSON.

// ts is a wrapper around time.Time for clean JSON interoperation with
// Gitiles time encoded as a string. This allows for the time to be
// parsed from a Gitiles Commit JSON object at decode time, which makes
// the User struct more idiomatic.
//
// Note: we cannot use name "timestamp" because it is reserved by the
// timestamp proto package.
type ts struct {
	time.Time
}

// MarshalJSON converts Time to an ANSIC string before marshalling.
func (t ts) MarshalJSON() ([]byte, error) {
	stringTime := t.Format(time.ANSIC)
	return json.Marshal(stringTime)
}

// UnmarshalJSON unmarshals an ANSIC string before parsing it into a Time.
func (t *ts) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsedTime, err := time.Parse(time.ANSIC, s)
	if err != nil {
		// Try it with the appended timezone version.
		parsedTime, err = time.Parse(time.ANSIC+" -0700", s)
	}
	t.Time = parsedTime
	return err
}

// user is the author or the committer returned from gitiles.
type user struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Time  ts     `json:"time"`
}

// Proto converts this User to its protobuf equivalent.
func (u *user) Proto() (ret *git.Commit_User, err error) {
	ret = &git.Commit_User{
		Name:  u.Name,
		Email: u.Email,
	}
	if !u.Time.IsZero() {
		ret.Time, err = ptypes.TimestampProto(u.Time.Time)
		err = errors.WrapIf(err, "encoding time")
	}
	return
}

// commit is the information of a commit returned from gitiles.
type commit struct {
	Commit    string                 `json:"commit"`
	Tree      string                 `json:"tree"`
	Parents   []string               `json:"parents"`
	Author    user                   `json:"author"`
	Committer user                   `json:"committer"`
	Message   string                 `json:"message"`
	TreeDiff  []*git.Commit_TreeDiff `json:"tree_diff"`
}

// Proto converts this git.Commit to its protobuf equivalent.
func (c *commit) Proto() (ret *git.Commit, err error) {
	ret = &git.Commit{
		Id:       c.Commit,
		Tree:     c.Tree,
		Parents:  c.Parents,
		Message:  c.Message,
		TreeDiff: c.TreeDiff,
	}

	if ret.Author, err = c.Author.Proto(); err != nil {
		err = errors.Fmt("decoding author: %w", err)
		return
	}
	if ret.Committer, err = c.Committer.Proto(); err != nil {
		err = errors.Fmt("decoding committer: %w", err)
		return
	}

	return
}

// project is the information of a project returned from gitiles.
type project struct {
	Name        string `json:"name"`
	CloneURL    string `json:"clone_url"`
	Description string `json:"description"`
}
