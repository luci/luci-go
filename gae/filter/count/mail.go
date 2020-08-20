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

package count

import (
	"go.chromium.org/gae/service/mail"
	"golang.org/x/net/context"
)

// MailCounter is the counter object for the Mail service.
type MailCounter struct {
	Send         Entry
	SendToAdmins Entry
}

type mailCounter struct {
	c *MailCounter

	m mail.RawInterface
}

var _ mail.RawInterface = (*mailCounter)(nil)

func (m *mailCounter) Send(msg *mail.Message) error {
	return m.c.Send.up(m.m.Send(msg))
}

func (m *mailCounter) SendToAdmins(msg *mail.Message) error {
	return m.c.SendToAdmins.up(m.m.SendToAdmins(msg))
}

func (m *mailCounter) GetTestable() mail.Testable {
	return m.m.GetTestable()
}

// FilterMail installs a counter Mail filter in the context.
func FilterMail(c context.Context) (context.Context, *MailCounter) {
	state := &MailCounter{}
	return mail.AddFilters(c, func(ic context.Context, u mail.RawInterface) mail.RawInterface {
		return &mailCounter{state, u}
	}), state
}
