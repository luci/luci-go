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

// Package seccfg interprets SecurityConfig proto message.
package seccfg

import (
	"regexp"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/auth/service/protocol"
)

// SecurityConfig is parsed queryable form of SecurityConfig proto message.
type SecurityConfig struct {
	internalServices *regexp.Regexp // may be nil
}

// Parse parses serialized SecurityConfig proto message.
func Parse(blob []byte) (*SecurityConfig, error) {
	// Empty config is allowed.
	if len(blob) == 0 {
		return &SecurityConfig{}, nil
	}

	msg := protocol.SecurityConfig{}
	if err := proto.Unmarshal(blob, &msg); err != nil {
		return nil, errors.Fmt("failed to deserialize SecurityConfig: %w", err)
	}

	secCfg := &SecurityConfig{}

	// Assemble a single regexp from a bunch of 'internal_service_regexp'.
	if len(msg.InternalServiceRegexp) > 0 {
		b := strings.Builder{}
		b.WriteString("^(")
		for idx, re := range msg.InternalServiceRegexp {
			if idx != 0 {
				b.WriteRune('|')
			}
			b.WriteRune('(')
			b.WriteString(re)
			b.WriteRune(')')
		}
		b.WriteString(")$")

		var err error
		if secCfg.internalServices, err = regexp.Compile(b.String()); err != nil {
			return nil, errors.Fmt("failed to compile internal_service_regexp: %w", err)
		}
	}

	return secCfg, nil
}

// IsInternalService returns true if the given hostname belongs to a service
// that is a part of the current LUCI deployment.
func (cfg *SecurityConfig) IsInternalService(hostname string) bool {
	if cfg.internalServices == nil {
		return false
	}
	return cfg.internalServices.MatchString(hostname)
}
