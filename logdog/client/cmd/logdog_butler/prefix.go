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

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os/user"

	log "go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/logdog/common/types"
)

const (
	// userPrefixBytes is the size, in bytes, of the user prefix random component.
	userPrefixBytes = 30
)

// getCurrentUser returns the current user name.
//
// It's a variable so that it can be overridden for testing.
var getCurrentUser = func() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

// generateRandomPrefix generates a new log prefix in the "user" space.
func generateRandomUserPrefix(ctx context.Context) (types.StreamName, error) {
	username, err := getCurrentUser()
	if err != nil {
		log.Warningf(log.SetError(ctx, err), "Failed to lookup current user name.")
		username = "user"
	}

	buf := make([]byte, userPrefixBytes)
	c, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	if c != len(buf) {
		return "", errors.New("main: failed to fill user prefix bytes")
	}

	streamName, err := types.MakeStreamName("s_", "butler", "users", username, base64.URLEncoding.EncodeToString(buf))
	if err != nil {
		return "", fmt.Errorf("failed to generate user prefix: %s", err)
	}
	return streamName, nil
}
