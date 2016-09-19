// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package featureBreaker

import (
	"github.com/luci/gae/service/mail"
	"golang.org/x/net/context"
)

type mailState struct {
	*state

	mail.RawInterface
}

var _ mail.RawInterface = (*mailState)(nil)

func (m *mailState) Send(msg *mail.Message) error {
	return m.run(func() error { return m.RawInterface.Send(msg) })
}

func (m *mailState) SendToAdmins(msg *mail.Message) error {
	return m.run(func() error { return m.RawInterface.SendToAdmins(msg) })
}

// FilterMail installs a featureBreaker mail filter in the context.
func FilterMail(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return mail.AddFilters(c, func(ic context.Context, i mail.RawInterface) mail.RawInterface {
		return &mailState{state, i}
	}), state
}
