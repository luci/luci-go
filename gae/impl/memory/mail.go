// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"fmt"
	net_mail "net/mail"
	"net/textproto"
	"path/filepath"
	"strings"
	"sync"

	"github.com/luci/gae/service/mail"
	"github.com/luci/gae/service/user"
	"golang.org/x/net/context"
)

type mailData struct {
	sync.Mutex
	queue       []*mail.TestMessage
	admins      []string
	adminsPlain []string
}

// mailImpl is a contextual pointer to the current mailData.
type mailImpl struct {
	data *mailData

	c context.Context
}

var _ mail.Interface = (*mailImpl)(nil)

// useMail adds a mail.Interface implementation to context, accessible
// by mail.Get(c)
func useMail(c context.Context) context.Context {
	data := &mailData{
		admins:      []string{"admin@example.com"},
		adminsPlain: []string{"admin@example.com"},
	}

	return mail.SetFactory(c, func(c context.Context) mail.Interface {
		return &mailImpl{data, c}
	})
}

func parseEmails(emails ...string) error {
	for _, e := range emails {
		if _, err := net_mail.ParseAddress(e); err != nil {
			return fmt.Errorf("invalid email (%q): %s", e, err)
		}
	}
	return nil
}

func checkMessage(msg *mail.TestMessage, adminsPlain []string, user string) error {
	sender, err := net_mail.ParseAddress(msg.Sender)
	if err != nil {
		return fmt.Errorf("unparsable Sender address: %s: %s", msg.Sender, err)
	}
	senderOK := user != "" && sender.Address == user
	if !senderOK {
		for _, a := range adminsPlain {
			if sender.Address == a {
				senderOK = true
				break
			}
		}
	}
	if !senderOK {
		return fmt.Errorf("invalid Sender: %s", msg.Sender)
	}

	if len(msg.To) == 0 && len(msg.Cc) == 0 && len(msg.Bcc) == 0 {
		return fmt.Errorf("one of To, Cc or Bcc must be non-empty")
	}

	if err := parseEmails(msg.To...); err != nil {
		return err
	}
	if err := parseEmails(msg.Cc...); err != nil {
		return err
	}
	if err := parseEmails(msg.Bcc...); err != nil {
		return err
	}

	if len(msg.Body) == 0 && len(msg.HTMLBody) == 0 {
		return fmt.Errorf("one of Body or HTMLBody must be non-empty")
	}

	if len(msg.Attachments) > 0 {
		msg.MIMETypes = make([]string, len(msg.Attachments))
		for i := range msg.Attachments {
			n := msg.Attachments[i].Name
			ext := strings.TrimLeft(strings.ToLower(filepath.Ext(n)), ".")
			if badExtensions.Has(ext) {
				return fmt.Errorf("illegal attachment extension for %q", n)
			}
			mimetype := extensionMapping[ext]
			if mimetype == "" {
				mimetype = "application/octet-stream"
			}
			msg.MIMETypes[i] = mimetype
		}
	}

	fixKeys := map[string]string{}
	for k := range msg.Headers {
		canonK := textproto.CanonicalMIMEHeaderKey(k)
		if !okHeaders.Has(canonK) {
			return fmt.Errorf("disallowed header: %s", k)
		}
		if canonK != k {
			fixKeys[k] = canonK
		}
	}
	for k, canonK := range fixKeys {
		vals := msg.Headers[k]
		delete(msg.Headers, k)
		msg.Headers[canonK] = vals
	}

	return nil
}

func (m *mailImpl) sendImpl(msg *mail.Message) error {
	email := ""
	userSvc := user.Get(m.c)
	if u := userSvc.Current(); u != nil {
		email = u.Email
	}

	m.data.Lock()
	adminsPlain := m.data.adminsPlain[:]
	m.data.Unlock()

	testMsg := &mail.TestMessage{Message: *msg}

	if err := checkMessage(testMsg, adminsPlain, email); err != nil {
		return err
	}
	m.data.Lock()
	m.data.queue = append(m.data.queue, testMsg)
	m.data.Unlock()
	return nil
}

func (m *mailImpl) Send(msg *mail.Message) error {
	return m.sendImpl(msg.Copy())
}

func (m *mailImpl) SendToAdmins(msg *mail.Message) error {
	msg = msg.Copy()
	m.data.Lock()
	ads := m.data.admins[:]
	m.data.Unlock()

	msg.To = make([]string, len(ads))
	copy(msg.To, ads)

	return m.sendImpl(msg)
}

func (m *mailImpl) Testable() mail.Testable {
	return m
}

func (m *mailImpl) SetAdminEmails(emails ...string) {
	adminsPlain := make([]string, len(emails))
	for i, e := range emails {
		adr, err := net_mail.ParseAddress(e)
		if err != nil {
			panic(fmt.Errorf("invalid email (%q): %s", e, err))
		}
		adminsPlain[i] = adr.Address
	}

	m.data.Lock()
	m.data.admins = emails
	m.data.adminsPlain = adminsPlain
	m.data.Unlock()
}

func (m *mailImpl) SentMessages() []*mail.TestMessage {
	m.data.Lock()
	msgs := m.data.queue[:]
	m.data.Unlock()

	ret := make([]*mail.TestMessage, len(msgs))
	for i, m := range msgs {
		ret[i] = m.Copy()
	}
	return ret
}

func (m *mailImpl) Reset() {
	m.data.Lock()
	m.data.queue = nil
	m.data.Unlock()
}
