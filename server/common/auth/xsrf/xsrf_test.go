// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package xsrf

import (
	"testing"
	"time"

	"golang.org/x/net/context"
)

const (
	user   = "user"
	action = "action"
)

func TestShouldRun(t *testing.T) {
	c := context.Background()
	now := time.Now()
	token, err := generateTokenWithTime(c, user, action, now)
	if err != nil {
		t.Fatal(err)
	}
	err = checkTokenWithTime(c, token, user, action, now)
	if err != nil {
		t.Errorf("checkTokenWithTime(c, %v, %v, %v, %v)=%v; want <nil>", token, user, action, now, err)
	}
}

func TestCheckShouldBeErrorForEmptyToken(t *testing.T) {
	c := context.Background()
	now := time.Now()
	token, err := generateTokenWithTime(c, user, action, now)
	if err != nil {
		t.Fatal(err)
	}
	err = checkTokenWithTime(c, "", user, action, now)
	if err == nil {
		t.Errorf("checkTokenWithTime(c, %v, %v, %v, %v)=<nil>; want error", token, user, action, now)
	}
}

func TestCheckShouldBeErrorWithBadUser(t *testing.T) {
	c := context.Background()
	now := time.Now()
	token, err := generateTokenWithTime(c, user, action, now)
	if err != nil {
		t.Fatal(err)
	}
	badUser := "badUser"
	err = checkTokenWithTime(c, token, badUser, action, now)
	if err == nil {
		t.Errorf("checkTokenWithTime(c, %v, %v, %v, %v)=<nil>; want error", token, badUser, action, now)
	}
}

func TestCheckShouldBeErrorWithBadAction(t *testing.T) {
	c := context.Background()
	now := time.Now()
	token, err := generateTokenWithTime(c, user, action, now)
	if err != nil {
		t.Fatal(err)
	}
	badAction := "badAction"
	err = checkTokenWithTime(c, token, user, badAction, now)
	if err == nil {
		t.Errorf("checkTokenWithTime(c, %v, %v, %v, %v)=<nil>; want error", token, user, badAction, now)
	}
}

func TestCheckShouldBeErrorWithExpiredToken(t *testing.T) {
	c := context.Background()
	now := time.Now()
	token, err := generateTokenWithTime(c, user, action, now)
	if err != nil {
		t.Fatal(err)
	}
	currentTime := now.Add(Timeout).Add(time.Second)
	err = checkTokenWithTime(c, token, user, action, currentTime)
	if err == nil {
		t.Errorf("checkTokenWithTime(c, %v, %v, %v, %v)=<nil>; want error", token, user, action, currentTime)
	}
}

func TestCheckShouldBeErrorWithFutureIssuedToken(t *testing.T) {
	c := context.Background()
	now := time.Now()
	token, err := generateTokenWithTime(c, user, action, now.Add(validFuture).Add(time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	err = checkTokenWithTime(c, token, user, action, now)
	if err == nil {
		t.Errorf("checkTokenWithTime(c, %v, %v, %v, %v)=<nil>; want error", token, user, action, now)
	}
}

func TestCheckShouldBeErrorWithEmptyToken(t *testing.T) {
	c := context.Background()
	now := time.Now()
	err := checkTokenWithTime(c, "", user, action, now)
	if err == nil {
		t.Errorf("checkTokenWithTime(c, %v, %v, %v, %v)=<nil>; want error", "", user, action, now)
	}
}

func TestCheckShouldBeErrorWithBrokenBase64(t *testing.T) {
	c := context.Background()
	now := time.Now()
	err := checkTokenWithTime(c, "@not#base64$", user, action, now)
	if err == nil {
		t.Errorf("checkTokenWithTime(c, %v, %v, %v, %v)=<nil>; want error", "", user, action, now)
	}
}
