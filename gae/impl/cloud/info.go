// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cloud

import (
	"errors"
	"time"

	infoS "github.com/luci/gae/service/info"

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
	*infoState

	ic context.Context
}

func useInfo(c context.Context) context.Context {
	var baseInfoState infoState
	c = baseInfoState.use(c)

	return infoS.SetFactory(c, func(ic context.Context) infoS.RawInterface {
		return &infoService{
			infoState: getInfoState(ic),
			ic:        ic,
		}
	})
}

func (i *infoService) AppID() string                { panic(errNotImplemented) }
func (i *infoService) FullyQualifiedAppID() string  { return "" }
func (i *infoService) GetNamespace() (string, bool) { return i.namespace, (i.namespace != "") }

func (i *infoService) Datacenter() string             { panic(errNotImplemented) }
func (i *infoService) DefaultVersionHostname() string { panic(errNotImplemented) }
func (i *infoService) InstanceID() string             { panic(errNotImplemented) }
func (i *infoService) IsDevAppServer() bool           { panic(errNotImplemented) }
func (i *infoService) IsOverQuota(err error) bool     { panic(errNotImplemented) }
func (i *infoService) IsTimeoutError(err error) bool  { panic(errNotImplemented) }
func (i *infoService) ModuleHostname(module, version, instance string) (string, error) {
	return "", errNotImplemented
}
func (i *infoService) ModuleName() string              { panic(errNotImplemented) }
func (i *infoService) RequestID() string               { panic(errNotImplemented) }
func (i *infoService) ServerSoftware() string          { panic(errNotImplemented) }
func (i *infoService) ServiceAccount() (string, error) { return "", errNotImplemented }
func (i *infoService) VersionID() string               { panic(errNotImplemented) }

func (i *infoService) Namespace(namespace string) (context.Context, error) {
	return i.MustNamespace(namespace), nil
}

func (i *infoService) MustNamespace(namespace string) context.Context {
	return i.derive(func(ci *infoState) {
		ci.namespace = namespace
	}).use(i.ic)
}

func (i *infoService) AccessToken(scopes ...string) (token string, expiry time.Time, err error) {
	return "", time.Time{}, errNotImplemented
}

func (i *infoService) PublicCertificates() ([]infoS.Certificate, error) {
	return nil, errNotImplemented
}

func (i *infoService) SignBytes(bytes []byte) (keyName string, signature []byte, err error) {
	return "", nil, errNotImplemented
}

func (i *infoService) Testable() infoS.Testable { return nil }
