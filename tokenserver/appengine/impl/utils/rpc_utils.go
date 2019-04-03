// Copyright 2019 The LUCI Authors.
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

package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
)

const (
	// maxAllowedMinValidityDuration specifies the maximum project identity token validity period
	// that a client may ask for.
	maxAllowedMinValidityDuration = 30 * time.Minute

	// DefaultMinValidityDuration is a value for minimal returned token lifetime if 'min_validity_duration'
	// field is not specified in the request.
	DefaultMinValidityDuration = 5 * time.Minute
)

// RPC interface specifies custom functionality implemented per RPC object.
type RPC interface {
	Name() string
}

// ValidateProject validates a LUCI project string.
func ValidateProject(c context.Context, project string) error {
	if project == "" {
		return fmt.Errorf("luci_project is empty")
	}
	return nil
}

// ValidateAndNormalizeRequest validates and normalizes RPC requests.
func ValidateAndNormalizeRequest(c context.Context, oauthScope []string, durationSecs *int64, auditTags []string) error {
	minDur := time.Duration(*durationSecs) * time.Second
	// Test error cases
	switch {
	case len(oauthScope) <= 0:
		return fmt.Errorf("oauth_scope is required")
	case minDur < 0:
		return fmt.Errorf("min_validity_duration must be positive")
	case minDur > maxAllowedMinValidityDuration:
		return fmt.Errorf("min_validity_duration must not exceed %d", maxAllowedMinValidityDuration/time.Second)
	}
	if err := ValidateTags(auditTags); err != nil {
		return fmt.Errorf("bad audit_tags - %s", err)
	}

	// Perform normalization
	if minDur == 0 {
		*durationSecs = int64(DefaultMinValidityDuration.Seconds())
	}
	return nil
}

// LogRequest logs the RPC request.
func LogRequest(c context.Context, rpc RPC, req proto.Message, caller identity.Identity) {
	if !logging.IsLogging(c, logging.Debug) {
		return
	}
	m := jsonpb.Marshaler{Indent: " "}
	dump, err := m.MarshalToString(req)
	if err != nil {
		panic(err)
	}
	logging.Debugf(c, "Identity: %s, %s:\n%s", caller, rpc.Name(), dump)
}
