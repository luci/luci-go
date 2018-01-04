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
	"encoding/hex"
	"encoding/json"
	"strings"
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
		err = errors.Annotate(err, "encoding time").Err()
	}
	return
}

// treeDiff shows the pertinent 'diff' information between two Commit objects.
type treeDiff struct {
	// Type is one of the KnownTreeDiffTypes.
	Type    string `json:"type"`
	OldID   string `json:"old_id"`
	OldMode uint32 `json:"old_mode"`
	OldPath string `json:"old_path"`
	NewID   string `json:"new_id"`
	NewMode uint32 `json:"new_mode"`
	NewPath string `json:"new_path"`
}

// Proto converts this TreeDiff to its protobuf equivalent.
func (t *treeDiff) Proto() (ret *git.Commit_TreeDiff, err error) {
	ret = &git.Commit_TreeDiff{
		OldPath: t.OldPath,
		OldMode: t.OldMode,
		NewPath: t.NewPath,
		NewMode: t.NewMode,
	}

	if val, ok := git.Commit_TreeDiff_ChangeType_value[strings.ToUpper(t.Type)]; ok {
		ret.Type = git.Commit_TreeDiff_ChangeType(val)
	} else {
		err = errors.Reason("bad change type: %q", t.Type).Err()
		return
	}

	if t.OldID != "" {
		if ret.OldId, err = hex.DecodeString(t.OldID); err != nil {
			err = errors.Annotate(err, "decoding OldID").Err()
			return
		}
	}

	if t.NewID != "" {
		if ret.NewId, err = hex.DecodeString(t.NewID); err != nil {
			err = errors.Annotate(err, "decoding NewID").Err()
			return
		}
	}

	return
}

// commit is the information of a commit returned from gitiles.
type commit struct {
	Commit    string     `json:"commit"`
	Tree      string     `json:"tree"`
	Parents   []string   `json:"parents"`
	Author    user       `json:"author"`
	Committer user       `json:"committer"`
	Message   string     `json:"message"`
	TreeDiff  []treeDiff `json:"tree_diff"`
}

// Proto converts this git.Commit to its protobuf equivalent.
func (c *commit) Proto() (ret *git.Commit, err error) {
	ret = &git.Commit{}

	if ret.Id, err = hex.DecodeString(c.Commit); err != nil {
		err = errors.Annotate(err, "decoding id").Err()
		return
	}
	if ret.Tree, err = hex.DecodeString(c.Tree); err != nil {
		err = errors.Annotate(err, "decoding tree").Err()
		return
	}
	if len(c.Parents) > 0 {
		ret.Parents = make([][]byte, len(c.Parents))
		for i, p := range c.Parents {
			if ret.Parents[i], err = hex.DecodeString(p); err != nil {
				err = errors.Annotate(err, "decoding parent %d", i).Err()
				return
			}
		}
	}
	if ret.Author, err = c.Author.Proto(); err != nil {
		err = errors.Annotate(err, "decoding author").Err()
		return
	}
	if ret.Committer, err = c.Committer.Proto(); err != nil {
		err = errors.Annotate(err, "decoding committer").Err()
		return
	}
	ret.Message = c.Message

	if len(c.TreeDiff) > 0 {
		ret.TreeDiff = make([]*git.Commit_TreeDiff, len(c.TreeDiff))
		for i, d := range c.TreeDiff {
			if ret.TreeDiff[i], err = d.Proto(); err != nil {
				err = errors.Annotate(err, "decoding treediff %d", i).Err()
				return
			}
		}
	}

	return
}
