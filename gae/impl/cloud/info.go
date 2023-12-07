// Copyright 2016 The LUCI Authors.
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
	"context"
	"time"

	"go.chromium.org/luci/common/errors"

	infoS "go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/gae/service/info/support"
)

// ErrNotImplemented is an error that can be returned to indicate that the
// requested functionality is not implemented.
var ErrNotImplemented = errors.New("not implemented")

// serviceInstanceGlobalInfo is the set of base, immutable info service values.
// These are initialized when the service instance is instantiated.
type serviceInstanceGlobalInfo struct {
	IsDev              bool
	ProjectID          string
	ServiceName        string
	VersionName        string
	InstanceID         string
	RequestID          string
	ServiceAccountName string
}

// infoState is the state of the "service/info" service in the current Context.
// As mutable fields change, new infoState instances pointing to the same base
// data will be installed into the Context.
type infoState struct {
	*serviceInstanceGlobalInfo

	// namespace is the current namespace, or the empty string for no namespace.
	namespace string
}

var infoStateKey = "*cloud.infoState"

func getInfoState(c context.Context) *infoState {
	if is, ok := c.Value(&infoStateKey).(*infoState); ok {
		return is
	}
	panic("no info state in Context")
}

func (ci *infoState) use(c context.Context) context.Context {
	return context.WithValue(c, &infoStateKey, ci)
}

func (ci *infoState) derive(mutate func(*infoState)) *infoState {
	dci := *ci
	mutate(&dci)
	return &dci
}

// infoService is an implementation of the "service/info" Interface that runs
// in a cloud enviornment.
type infoService struct {
	context.Context
	*infoState
}

func useInfo(c context.Context, base *serviceInstanceGlobalInfo) context.Context {
	// Install our initial info state into the Context.
	baseState := &infoState{
		serviceInstanceGlobalInfo: base,
		namespace:                 "",
	}
	c = baseState.use(c)

	return infoS.SetFactory(c, func(ic context.Context) infoS.RawInterface {
		return &infoService{
			Context:   ic,
			infoState: getInfoState(ic),
		}
	})
}

func (i *infoService) AppID() string               { return maybe(i.ProjectID) }
func (i *infoService) FullyQualifiedAppID() string { return maybe(i.ProjectID) }
func (i *infoService) GetNamespace() string        { return i.namespace }
func (i *infoService) IsDevAppServer() bool        { return i.IsDev }

func (*infoService) Datacenter() string             { panic(ErrNotImplemented) }
func (*infoService) DefaultVersionHostname() string { panic(ErrNotImplemented) }
func (i *infoService) InstanceID() string           { return maybe(i.infoState.InstanceID) }
func (*infoService) IsOverQuota(err error) bool     { return false }
func (*infoService) IsTimeoutError(err error) bool  { return false }
func (*infoService) ModuleHostname(module, version, instance string) (string, error) {
	return "", ErrNotImplemented
}
func (i *infoService) ModuleName() string   { return maybe(i.ServiceName) }
func (i *infoService) RequestID() string    { return maybe(i.infoState.RequestID) }
func (*infoService) ServerSoftware() string { panic(ErrNotImplemented) }
func (i *infoService) VersionID() string    { return maybe(i.VersionName) }

func (i *infoService) ServiceAccount() (string, error) {
	if i.ServiceAccountName != "" {
		return i.ServiceAccountName, nil
	}
	return "", ErrNotImplemented
}

func (i *infoService) Namespace(namespace string) (context.Context, error) {
	if err := support.ValidNamespace(namespace); err != nil {
		return i, err
	}

	return i.derive(func(ci *infoState) {
		ci.namespace = namespace
	}).use(i), nil
}

// PublicCertificates returns the set of public certicates bound to the current
// service account. This is done by accessing Google's public certificate
// HTTP endpoint and requesting certificastes for the current service account
// name.
//
// PublicCertificates performs no caching on the result, so multiple requests
// will result in multiple HTTP API calls.
func (i *infoService) PublicCertificates() (certs []infoS.Certificate, err error) {
	return nil, ErrNotImplemented
}

// AccessToken returns an access token for the given set of scopes.
func (i *infoService) AccessToken(scopes ...string) (token string, expiry time.Time, err error) {
	return "", time.Time{}, ErrNotImplemented
}

// SignBytes is implemented using a call to Google Cloud IAM's "signBlob"
// endpoint.
//
// This must be an authenticated call.
//
// https://cloud.google.com/iam/reference/rest/v1/projects.serviceAccounts/signBlob
func (i *infoService) SignBytes(bytes []byte) (keyName string, signature []byte, err error) {
	return "", nil, ErrNotImplemented
}

func (*infoService) GetTestable() infoS.Testable { return nil }

func maybe(v string) string {
	if v != "" {
		return v
	}
	panic(ErrNotImplemented)
}
