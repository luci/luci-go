// Copyright 2023 The LUCI Authors.
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

// Package integrationmocks exposes endpoints to simplify integration testing.
//
// Exposed endpoints fake some of Swarming Python server logic, which allows
// running some integration tests entirely within luci-go repo.
package integrationmocks

import (
	"context"

	"google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/hmactoken"
)

// server implements IntegrationMocksServer.
type server struct {
	UnimplementedIntegrationMocksServer

	hmacSecret *hmactoken.Secret
}

// New constructs an IntegrationMocksServer implementation.
func New(ctx context.Context, hmacSecret *hmactoken.Secret) IntegrationMocksServer {
	return &server{
		hmacSecret: hmacSecret,
	}
}

func (s *server) GeneratePollToken(ctx context.Context, msg *internalspb.PollState) (*PollToken, error) {
	tok, err := s.hmacSecret.GenerateToken(msg)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%s", err)
	}
	return &PollToken{PollToken: tok}, nil
}
