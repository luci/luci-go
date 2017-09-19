// Copyright 2015 The LUCI Authors.
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

package featureBreaker

import (
	"go.chromium.org/gae/service/mail"
	"golang.org/x/net/context"
)

type mailState struct {
	*state

	c context.Context
	mail.RawInterface
}

var _ mail.RawInterface = (*mailState)(nil)

func (m *mailState) Send(msg *mail.Message) error {
	return m.run(m.c, func() error { return m.RawInterface.Send(msg) })
}

func (m *mailState) SendToAdmins(msg *mail.Message) error {
	return m.run(m.c, func() error { return m.RawInterface.SendToAdmins(msg) })
}

// FilterMail installs a featureBreaker mail filter in the context.
func FilterMail(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return mail.AddFilters(c, func(ic context.Context, i mail.RawInterface) mail.RawInterface {
		return &mailState{state, ic, i}
	}), state
}
