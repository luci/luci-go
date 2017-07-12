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

package dm

import (
	"fmt"

	"github.com/luci/luci-go/common/errors"
)

// Normalize returns an error iff the TemplateInstantiation is invalid.
func (t *TemplateInstantiation) Normalize() error {
	if t.Project == "" {
		return errors.New("empty project")
	}
	return t.Specifier.Normalize()
}

func checkAttemptNums(name string, lst []*AttemptList_Nums) error {
	for i, nums := range lst {
		if err := nums.Normalize(); err != nil {
			return fmt.Errorf("%s[%d]: %s", name, i, err)
		}
		if len(nums.Nums) == 0 {
			return fmt.Errorf("%s[%d]: empty attempt list", name, i)
		}
	}
	return nil
}

// Normalize returns an error iff the request is invalid.
func (r *EnsureGraphDataReq) Normalize() error {
	if r.ForExecution != nil {
		if err := r.ForExecution.Normalize(); err != nil {
			return err
		}
	}

	if err := r.RawAttempts.Normalize(); err != nil {
		return err
	}

	hasAttempts := false
	if r.RawAttempts != nil {
		for _, nums := range r.RawAttempts.To {
			if len(nums.Nums) == 0 {
				return errors.New("EnsureGraphDataReq.attempts must only include valid (non-0, non-empty) attempt numbers")
			}
			hasAttempts = true
		}
	}

	if len(r.Quest) != len(r.QuestAttempt) {
		return errors.New("mismatched quest_attempt v. quest lengths")
	}

	if len(r.TemplateQuest) != len(r.TemplateAttempt) {
		return errors.New("mismatched template_attempt v. template_quest lengths")
	}

	if err := checkAttemptNums("template_attempts", r.TemplateAttempt); err != nil {
		return err
	}

	for i, q := range r.TemplateQuest {
		if err := q.Normalize(); err != nil {
			return fmt.Errorf("template_quests[%d]: %s", i, err)
		}
	}

	if err := checkAttemptNums("quest_attempt", r.QuestAttempt); err != nil {
		return err
	}

	for i, desc := range r.Quest {
		if err := desc.Normalize(); err != nil {
			return fmt.Errorf("quest[%d]: %s", i, err)
		}
	}

	if len(r.Quest) == 0 && len(r.TemplateQuest) == 0 && !hasAttempts {
		return errors.New("EnsureGraphDataReq must have at least one of quests, template_quests and raw_attempts")
	}

	if r.Limit == nil {
		r.Limit = &EnsureGraphDataReq_Limit{}
	}
	if r.Limit.MaxDataSize == 0 {
		r.Limit.MaxDataSize = DefaultLimitMaxDataSize
	}
	if r.Limit.MaxDataSize > MaxLimitMaxDataSize {
		r.Limit.MaxDataSize = MaxLimitMaxDataSize
	}

	if r.Include == nil {
		r.Include = &EnsureGraphDataReq_Include{Attempt: &EnsureGraphDataReq_Include_Options{}}
	} else if r.Include.Attempt == nil {
		r.Include.Attempt = &EnsureGraphDataReq_Include_Options{}
	}
	return nil
}
