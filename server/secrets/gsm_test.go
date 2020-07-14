// Copyright 2020 The LUCI Authors.
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

package secrets

import (
	"context"
	"fmt"
	"testing"

	gax "github.com/googleapis/gax-go/v2"
	"golang.org/x/oauth2"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1beta1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSecretManagerSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fakeTS := oauth2.StaticTokenSource(&oauth2.Token{})

	Convey("Constructor", t, func() {
		Convey("Works", func() {
			s, err := NewSecretManagerSource(ctx, "sm://project/secret", fakeTS)
			So(err, ShouldBeNil)
			So(s.project, ShouldEqual, "project")
			So(s.secret, ShouldEqual, "secret")
		})

		Convey("Not sm://", func() {
			_, err := NewSecretManagerSource(ctx, "zz://project/secret", fakeTS)
			So(err, ShouldErrLike, "not a sm://... URL")
		})

		Convey("Wrong sm:// format", func() {
			_, err := NewSecretManagerSource(ctx, "sm://project", fakeTS)
			So(err, ShouldErrLike, "should have form")
			_, err = NewSecretManagerSource(ctx, "sm://project/secret/zzz", fakeTS)
			So(err, ShouldErrLike, "should have form")
		})
	})

	Convey("ReadSecret", t, func() {
		client := mockedSMClient{
			project:  "project",
			secret:   "secret",
			versions: map[string]*mockedVersion{},
		}

		src := &SecretManagerSource{
			client:    &client,
			secretURL: "sm://project/secret",
			project:   "project",
			secret:    "secret",
		}

		// Reading the latest version and one previous one.
		client.setVersion(1, []byte{1, 2, 3})
		client.setVersion(2, []byte{4, 5, 6})
		secret, err := src.ReadSecret(ctx)
		So(err, ShouldBeNil)
		So(secret, ShouldResemble, &Secret{
			Current: []byte{4, 5, 6},
			Previous: [][]byte{
				{1, 2, 3},
			},
		})

		// A new version is added as latest.
		client.setVersion(3, []byte{7, 8, 9})
		secret, err = src.ReadSecret(ctx)
		So(err, ShouldBeNil)
		So(secret, ShouldResemble, &Secret{
			Current: []byte{7, 8, 9},
			Previous: [][]byte{
				{4, 5, 6},
			},
		})

		// An older version is disabled.
		client.disableVersion(2)
		secret, err = src.ReadSecret(ctx)
		So(err, ShouldBeNil)
		So(secret, ShouldResemble, &Secret{
			Current: []byte{7, 8, 9},
		})

		// An older version is deleted.
		client.deleteVersion(2)
		secret, err = src.ReadSecret(ctx)
		So(err, ShouldBeNil)
		So(secret, ShouldResemble, &Secret{
			Current: []byte{7, 8, 9},
		})

		// The current version is disabled as well, it breaks the secret.
		client.disableVersion(3)
		_, err = src.ReadSecret(ctx)
		So(err, ShouldNotBeNil)
	})
}

type mockedSMClient struct {
	project string
	secret  string

	versions map[string]*mockedVersion
}

type mockedVersion struct {
	name     string
	data     []byte
	disabled bool
}

func (c *mockedSMClient) versionID(ver interface{}) string {
	return fmt.Sprintf("projects/%s/secrets/%s/versions/%v", c.project, c.secret, ver)
}

func (c *mockedSMClient) setVersion(num int, val []byte) {
	mv := &mockedVersion{
		name: c.versionID(num),
		data: val,
	}
	c.versions[c.versionID(num)] = mv
	c.versions[c.versionID("latest")] = mv
}

func (c *mockedSMClient) disableVersion(num int) {
	c.versions[c.versionID(num)].disabled = true
}

func (c *mockedSMClient) deleteVersion(num int) {
	delete(c.versions, c.versionID(num))
}

func (c *mockedSMClient) AccessSecretVersion(ctx context.Context, req *secretmanagerpb.AccessSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error) {
	switch ver := c.versions[req.Name]; {
	case ver == nil:
		return nil, status.Errorf(codes.NotFound, "no such version")
	case ver.disabled:
		return nil, status.Errorf(codes.FailedPrecondition, "disabled version")
	default:
		return &secretmanagerpb.AccessSecretVersionResponse{
			Name:    ver.name,
			Payload: &secretmanagerpb.SecretPayload{Data: ver.data},
		}, nil
	}
}

func (c *mockedSMClient) Close() error {
	return nil
}
