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

// TestMessage is the message struct which will be returned from SentMessages.
//
// It augments the Message struct by also including the derived MIMEType for any
// attachments.
type TestMessage struct {
	Message

	// MIMETypes is guaranteed to be the same length as the attachments in the
	// Message, and will be populated with the derived MIME types for the
	// attachments.
	MIMETypes []string
}

// Copy duplicates this TestMessage.
func (t *TestMessage) Copy() *TestMessage {
	if t == nil {
		return nil
	}
	ret := &TestMessage{Message: *t.Message.Copy()}
	if len(t.MIMETypes) > 0 {
		ret.MIMETypes = make([]string, len(t.MIMETypes))
		copy(ret.MIMETypes, t.MIMETypes)
	}
	return ret
}

// Testable is the interface for mail service implementations which are able
// to be tested (like impl/memory).
type Testable interface {
	// Sets the list of admin emails. By default, testing implementations should
	// use ["admin@example.com"].
	SetAdminEmails(emails ...string)

	// SentMessages returns a copy of all messages which were successfully sent
	// via the mail API.
	SentMessages() []*TestMessage

	// Reset clears the SentMessages queue.
	Reset()
}
