// Copyright 2017 The LUCI Authors.
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

package cloud

import (
	infoS "go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
)

// ErrNotImplemented is an error that can be returned to indicate that the
// requested functionality is not implemented.
var ErrNotImplemented = errors.New("not implemented")

// ServiceProvider is a set of functionality which can be used to fetch system
// data.
//
// Service methods should implement their own caching as appropriate.
//
// Service methods should return ErrNotImplemented if a given function is not
// implemented.
type ServiceProvider interface {
	// PublicCertificates returns the set of public certificates belonging to the
	// current service's service account.
	PublicCertificates(c context.Context) ([]infoS.Certificate, error)

	// TokenSource returns a token source for the specified combination of scopes.
	TokenSource(c context.Context, scopes ...string) (oauth2.TokenSource, error)

	// SignBytes signs the specified bytes with a private key belonging to the
	// current system service.
	SignBytes(c context.Context, bytes []byte) (keyName string, signature []byte, err error)
}

////////////////////////////////////////////////////////////////////////////////

// notImplementedServiceProvider is a ServiceProvider that does not implement
// any functionality.
type notImplementedServiceProvider struct{}

func (notImplementedServiceProvider) PublicCertificates(c context.Context) ([]infoS.Certificate, error) {
	return nil, ErrNotImplemented
}

func (notImplementedServiceProvider) TokenSource(c context.Context, scopes ...string) (oauth2.TokenSource, error) {
	return nil, ErrNotImplemented
}

func (notImplementedServiceProvider) SignBytes(bytes []byte) (string, []byte, error) {
	return "", nil, ErrNotImplemented
}
