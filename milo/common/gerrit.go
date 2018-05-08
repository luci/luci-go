// Copyright 2018 The LUCI Authors.
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

package common

import (
	"errors"

	"golang.org/x/net/context"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

var gerritClientFactoryKey = "gerrit client factory"

// GerritFactory creates a Gerrit client.
type GerritFactory func(c context.Context, host string) (gerritpb.GerritClient, error)

// WithGerritFactory returns a context with the Gerrit client factory.
func WithGerritFactory(c context.Context, factory GerritFactory) context.Context {
	return context.WithValue(c, &gerritClientFactoryKey, factory)
}

// GetGerritFactory returns the GerritFactory installed into c using
// WithGerritFactory if it is there, otherwise returns nil.
func GetGerritFactory(c context.Context) GerritFactory {
	if f, ok := c.Value(&gerritClientFactoryKey).(GerritFactory); ok {
		return f
	}
	return nil
}

// CreateGerritClient creates a Gerrit client using the factory installed into
// c using WithGerritFactory.
func CreateGerritClient(c context.Context, host string) (gerritpb.GerritClient, error) {
	factory := GetGerritFactory(c)
	if factory == nil {
		return nil, errors.New("no Gerrit client factory")
	}
	return factory(c, host)
}
