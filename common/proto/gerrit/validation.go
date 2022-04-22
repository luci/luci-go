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

package gerritpb

import (
	"go.chromium.org/luci/common/errors"
)

// Validate returns an error if r is invalid.
func (r *GetChangeRequest) Validate() error {
	switch {
	case r.Number <= 0:
		return errors.New("number must be positive")
	default:
		return nil
	}
}

// Validate returns an error if r is invalid.
func (r *SetReviewRequest) Validate() error {
	if r.Number <= 0 {
		return errors.New("number must be positive")
	}
	if err := r.GetNotifyDetails().Validate(); err != nil {
		return err
	}
	for _, att := range r.GetAddToAttentionSet() {
		if err := att.Validate(); err != nil {
			return err
		}
	}
	for _, att := range r.GetRemoveFromAttentionSet() {
		if err := att.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (n *NotifyDetails) Validate() error {
	for _, r := range n.GetRecipients() {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (n *NotifyDetails_Recipient) Validate() error {
	if n.GetRecipientType() == NotifyDetails_RECIPIENT_TYPE_UNSPECIFIED {
		return errors.New("must specify recipient type")
	}
	return nil
}

func (r *AttentionSetRequest) Validate() error {
	if r.Number <= 0 {
		return errors.New("number must be positive")
	}
	if r.GetInput() == nil {
		return errors.New("input is required")
	}
	return r.GetInput().Validate()
}

func (i *AttentionSetInput) Validate() error {
	if i.GetUser() == "" {
		return errors.New("user is required")
	}
	if i.GetReason() == "" {
		return errors.New("reason is required")
	}
	return nil
}
