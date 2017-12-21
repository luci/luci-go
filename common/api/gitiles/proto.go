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

	"github.com/golang/protobuf/ptypes"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/git"
)

// Proto converts this User to its protobuf equivalent.
func (u *User) Proto() (ret *git.Commit_User, err error) {
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

// FromProto parses p into u, opposite of Proto().
func (u *User) FromProto(p *git.Commit_User) error {
	if p == nil {
		*u = User{}
		return nil
	}
	t, err := ptypes.Timestamp(p.Time)
	if err != nil {
		return err
	}
	*u = User{
		Name:  p.Name,
		Email: p.Email,
		Time:  Time{t},
	}
	return nil
}

// Proto converts this TreeDiff to its protobuf equivalent.
func (t *TreeDiff) Proto() (ret *git.Commit_TreeDiff, err error) {
	ret = &git.Commit_TreeDiff{
		OldPath: t.OldPath,
		OldMode: t.OldMode,
		NewPath: t.NewPath,
		NewMode: t.NewMode,
	}

	if val, ok := git.Commit_TreeDiff_ChangeType_value[t.Type]; ok {
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

// FromProto parses p into t, opposite of Proto().
func (t *TreeDiff) FromProto(p *git.Commit_TreeDiff) error {
	if p == nil {
		*t = TreeDiff{}
		return nil
	}
	if p.Type < 0 || int(p.Type) > len(git.Commit_TreeDiff_ChangeType_name) {
		return errors.Reason("bad change type %d", p.Type).Err()
	}
	*t = TreeDiff{
		OldPath: p.OldPath,
		OldMode: p.OldMode,
		NewPath: p.NewPath,
		NewMode: p.NewMode,
		Type:    p.Type.String(),
		OldID:   hex.EncodeToString(p.OldId),
		NewID:   hex.EncodeToString(p.NewId),
	}
	return nil
}

// Proto converts this git.Commit to its protobuf equivalent.
func (c *Commit) Proto() (ret *git.Commit, err error) {
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
		err = errors.Annotate(err, "encoding author").Err()
		return
	}
	if ret.Committer, err = c.Committer.Proto(); err != nil {
		err = errors.Annotate(err, "encoding committer").Err()
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

// FromProto parses p into c, opposite of Proto().
func (c *Commit) FromProto(p *git.Commit) error {
	if p == nil {
		*c = Commit{}
		return nil
	}

	ret := Commit{
		Commit:  hex.EncodeToString(p.Id),
		Tree:    hex.EncodeToString(p.Tree),
		Message: p.Message,
	}
	ret.Author.FromProto(p.Author)
	if len(p.Parents) > 0 {
		ret.Parents = make([]string, len(p.Parents))
		for i, p := range p.Parents {
			ret.Parents[i] = hex.EncodeToString(p)
		}
	}
	if err := ret.Author.FromProto(p.Author); err != nil {
		return errors.Annotate(err, "decoding author").Err()
	}
	if err := ret.Committer.FromProto(p.Committer); err != nil {
		return errors.Annotate(err, "decoding committer").Err()
	}
	if len(p.TreeDiff) > 0 {
		ret.TreeDiff = make([]TreeDiff, len(p.TreeDiff))
		for i, d := range p.TreeDiff {
			if err := ret.TreeDiff[i].FromProto(d); err != nil {
				return errors.Annotate(err, "decoding treediff %d", i).Err()
			}
		}
	}

	*c = ret
	return nil
}

// MarshalCommits marshals commits.
func MarshalCommits(commits []Commit) ([]byte, error) {
	log := &git.Log{
		Commits: make([]*git.Commit, len(commits)),
	}
	for i, c := range commits {
		var err error
		if log.Commits[i], err = c.Proto(); err != nil {
			return nil, errors.Annotate(err, "could not convert commit %s to proto", c.Commit).Err()
		}
	}
	return proto.Marshal(log)
}

// UnmarshalCommits unmarshals commits marshalled by MarshalCommits.
func UnmarshalCommits(data []byte) ([]Commit, error) {
	var log git.Log
	if err := proto.Unmarshal(data, &log); err != nil {
		return nil, err
	}

	commits := make([]Commit, len(log.Commits))
	for i, c := range log.Commits {
		if err := commits[i].FromProto(c); err != nil {
			return nil, errors.Annotate(err, "could not convert commit %s from proto", c.Id).Err()
		}
	}
	return commits, nil
}
