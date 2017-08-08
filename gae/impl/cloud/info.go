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
	"time"

	infoS "go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/info/support"

	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"
)

var errNotImplemented = errors.New("not implemented")

// cloudInfo is a reconstruction of the info service for the cloud API.
//
// It will return information sufficent for datastore operation.
type infoState struct {
	// namespace is the current namesapce, or the empty string for no namespace.
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

type infoService struct {
	context.Context
	*infoState
}

func useInfo(c context.Context) context.Context {
	var baseInfoState infoState
	c = baseInfoState.use(c)

	return infoS.SetFactory(c, func(ic context.Context) infoS.RawInterface {
		return &infoService{
			Context:   ic,
			infoState: getInfoState(ic),
		}
	})
}

func (*infoService) AppID() string               { panic(errNotImplemented) }
func (*infoService) FullyQualifiedAppID() string { return "" }
func (i *infoService) GetNamespace() string      { return i.namespace }
func (*infoService) IsDevAppServer() bool        { return false }

func (*infoService) Datacenter() string             { panic(errNotImplemented) }
func (*infoService) DefaultVersionHostname() string { panic(errNotImplemented) }
func (*infoService) InstanceID() string             { panic(errNotImplemented) }
func (*infoService) IsOverQuota(err error) bool     { panic(errNotImplemented) }
func (*infoService) IsTimeoutError(err error) bool  { panic(errNotImplemented) }
func (*infoService) ModuleHostname(module, version, instance string) (string, error) {
	return "", errNotImplemented
}
func (*infoService) ModuleName() string              { panic(errNotImplemented) }
func (*infoService) RequestID() string               { panic(errNotImplemented) }
func (*infoService) ServerSoftware() string          { panic(errNotImplemented) }
func (*infoService) ServiceAccount() (string, error) { return "", errNotImplemented }
func (*infoService) VersionID() string               { panic(errNotImplemented) }

func (i *infoService) Namespace(namespace string) (context.Context, error) {
	if err := support.ValidNamespace(namespace); err != nil {
		return i, err
	}

	return i.derive(func(ci *infoState) {
		ci.namespace = namespace
	}).use(i), nil
}

func (*infoService) AccessToken(scopes ...string) (token string, expiry time.Time, err error) {
	return "", time.Time{}, errNotImplemented
}

func (*infoService) PublicCertificates() ([]infoS.Certificate, error) {
	return nil, errNotImplemented
}

func (*infoService) SignBytes(bytes []byte) (keyName string, signature []byte, err error) {
	return "", nil, errNotImplemented
}

func (*infoService) GetTestable() infoS.Testable { return nil }
