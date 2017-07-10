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

package mail

import (
	"fmt"
	net_mail "net/mail"
	"reflect"

	"google.golang.org/appengine/mail"
)

// Attachment is a mimic of https://godoc.org/google.golang.org/appengine/mail#Attachment
//
// It's provided here for convenience, and is compile-time checked to be
// identical.
type Attachment struct {
	// Name must be set to a valid file name.
	Name      string
	Data      []byte
	ContentID string
}

var _ Attachment = (Attachment)(mail.Attachment{})

// Message is a mimic of https://godoc.org/google.golang.org/appengine/mail#Message
//
// It's provided here for convenience, and is init-time checked to be identical
// (not statically because []Attachement prevents static casting).
type Message struct {
	Sender      string
	ReplyTo     string
	To, Cc, Bcc []string
	Subject     string
	Body        string
	HTMLBody    string
	Attachments []Attachment
	Headers     net_mail.Header
}

func init() {
	mt := reflect.TypeOf(mail.Message{})
	ot := reflect.TypeOf(Message{})
	if mt.NumField() != ot.NumField() {
		panic(fmt.Errorf("mismatched number of fields %s v %s", mt, ot))
	}

	for i := 0; i < mt.NumField(); i++ {
		mf := mt.Field(i)
		of := mt.Field(i)
		if mf.Name != of.Name {
			panic(fmt.Errorf("mismatched names %s v %s", mf.Name, of.Name))
		}
		if mf.Name == "Attachments" {
			if !mf.Type.Elem().ConvertibleTo(of.Type.Elem()) {
				panic(fmt.Errorf("mismatched element type for Attachments %s v %s",
					mf.Type, of.Type))
			}
		} else {
			if mf.Type != of.Type {
				panic(fmt.Errorf("mismatched type for field %s: %s v %s", mf.Name, mf.Type, of.Type))
			}
		}
	}
}

func dupStrs(strs []string) []string {
	if len(strs) > 0 {
		ret := make([]string, len(strs))
		copy(ret, strs)
		return ret
	}
	return nil
}

// ToSDKMessage returns a copy of this Message that's compatible with the native
// SDK's Message type. It only needs to be used by implementations (like
// "impl/prod") which need an SDK compatible object
func (m *Message) ToSDKMessage() *mail.Message {
	if m == nil {
		return nil
	}

	m = m.Copy()

	ret := &mail.Message{
		Sender:   m.Sender,
		ReplyTo:  m.ReplyTo,
		Subject:  m.Subject,
		Body:     m.Body,
		HTMLBody: m.HTMLBody,
		To:       m.To,
		Cc:       m.Cc,
		Bcc:      m.Bcc,
		Headers:  m.Headers,
	}

	ret.Attachments = make([]mail.Attachment, len(m.Attachments))
	for i := range m.Attachments {
		ret.Attachments[i] = (mail.Attachment)(m.Attachments[i])
	}
	return ret
}

// Copy returns a duplicate Message
func (m *Message) Copy() *Message {
	if m == nil {
		return nil
	}

	ret := *m

	ret.To = dupStrs(m.To)
	ret.Cc = dupStrs(m.Cc)
	ret.Bcc = dupStrs(m.Bcc)

	if len(m.Headers) > 0 {
		ret.Headers = make(net_mail.Header, len(m.Headers))
		for k, vals := range m.Headers {
			ret.Headers[k] = dupStrs(vals)
		}
	}

	if len(m.Attachments) > 0 {
		ret.Attachments = make([]Attachment, len(m.Attachments))
		copy(ret.Attachments, m.Attachments)
	}

	return &ret
}
