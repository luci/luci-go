// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package xsrf

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/common/auth/signature"
)

// Timeout represents XSRF token time out.
const Timeout = time.Hour

// validFuture is maximum acceptable future issued time.
// This situation occur only when server's time went back after it issued token.
const validFuture = time.Minute

type sigData struct {
	Key       string
	Signature []byte
	IssueTime int64
}

// toData convert user, action, and expires to data to generate signature.
func toData(user, action string, expires int64) []byte {
	return []byte(fmt.Sprintf("%s:%s:%d", base64.URLEncoding.EncodeToString([]byte(user)), base64.URLEncoding.EncodeToString([]byte(action)), expires))
}

// Generate returns an XSRF token for user and action.
//
// XSRF token is base64 encoded JSON whose structure is:
// {"Key": <key name>, "Signature": <signature>, "IssueTime": <expire time>}
func Generate(c context.Context, user string, action string) (string, error) {
	return generateTokenWithTime(c, user, action, time.Now())
}

// generateTokenWithTime generates token using the given time.
func generateTokenWithTime(c context.Context, user string, action string, now time.Time) (string, error) {
	nowTime := now.UnixNano()
	d := toData(user, action, nowTime)
	key, signed, err := signature.Sign(c, d)
	if err != nil {
		return "", err
	}
	sig, err := json.Marshal(sigData{
		Key:       key,
		Signature: signed,
		IssueTime: nowTime,
	})
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(sig), nil
}

// Check returns nil if the token is valid.
func Check(c context.Context, token, user, action string) error {
	return checkTokenWithTime(c, token, user, action, time.Now())
}

// checkTokenWithTime checks token using the given time.
func checkTokenWithTime(c context.Context, token, user, action string, now time.Time) error {
	if token == "" {
		return fmt.Errorf("token is not given")
	}
	d, err := base64.URLEncoding.DecodeString(token)
	sig := &sigData{}
	if err = json.Unmarshal(d, sig); err != nil {
		return err
	}

	issueTime := time.Unix(0, sig.IssueTime)
	if now.Sub(issueTime) >= Timeout {
		return fmt.Errorf("signature has already expired")
	}
	if issueTime.After(now.Add(validFuture)) {
		return fmt.Errorf("token come from future")
	}

	toVerify := toData(user, action, sig.IssueTime)

	certs, err := signature.PublicCerts(c)
	if err != nil {
		return err
	}
	cert := signature.X509CertByName(certs, sig.Key)
	if cert == nil {
		return fmt.Errorf("cannot find cert")
	}

	return signature.Check(toVerify, cert, sig.Signature)
}
