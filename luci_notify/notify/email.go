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

package notify

import (
	"fmt"

	"google.golang.org/appengine/mail"

	"golang.org/x/net/context"
)

// Email wraps a GAE Mail API Message to provide a cleaner builder-pattern
// interface to the message, and to abstract away the details of sending
// an email.
type Email struct {
	m mail.Message
}

// BaseEmail starts a new Email from luci-notify.
func BaseEmail() *Email {
	return &Email{
		m: mail.Message{
			Sender: "luci-notify <noreply@luci-notify-dev.appspotmail.com>",
		},
	}
}

// To sets the Email's recipients.
func (e *Email) To(recipients []string) *Email {
	e.m.To = recipients
	return e
}

// Subject sets the Email's subject in a Printf-style interface.
func (e *Email) Subject(subject string, a ...interface{}) *Email {
	e.m.Subject = fmt.Sprintf(subject, a...)
	return e
}

// Body sets the Email's text body in a Printf-style interface.
func (e *Email) Body(body string, a ...interface{}) *Email {
	e.m.Body = fmt.Sprintf(body, a...)
	return e
}

// HTMLBody sets the Email's HTML body in a Printf-style interface.
func (e *Email) HTMLBody(body string, a ...interface{}) *Email {
	e.m.HTMLBody = fmt.Sprintf(body, a...)
	return e
}

// Send sends the Email with the GAE Mail API.
func (e *Email) Send(c context.Context) error {
	return mail.Send(c, &e.m)
}
