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

package testservices

// This test is not in discovery because it needs to a test services
// in different directories.
// However, a generated service depends on grpc/discovery
// and a test in grpc/discovery depends on the service.
// This creates a a cyclic import.
// To break the cycle, we move this test from grpc/discovery.

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/proto/google/descutil"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/grpc/discovery"
)

// force test services registration.
var _ = CalcServer(nil)

func TestDiscovery(t *testing.T) {
	ftt.Run("Discovery", t, func(t *ftt.Test) {

		server := discovery.New(
			"discovery.Discovery",
			"testservices.Greeter",
			"testservices.Calc",
		)

		c := context.Background()
		res, err := server.Describe(c, nil)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, res.Services, should.Match([]string{
			"discovery.Discovery",
			"testservices.Greeter",
			"testservices.Calc",
		}))

		desc := res.Description

		// this checks that file deduplication actually works.
		assert.Loosely(t, len(desc.File), should.Equal(3))

		_, discoveryIndex := descutil.FindService(desc, "discovery.Discovery")
		assert.Loosely(t, discoveryIndex, should.NotEqual(-1))

		_, calcIndex := descutil.FindService(desc, "testservices.Calc")
		assert.Loosely(t, calcIndex, should.NotEqual(-1))

		file, greeterIndex := descutil.FindService(desc, "testservices.Greeter")
		assert.Loosely(t, greeterIndex, should.NotEqual(-1))
		greeter := file.Service[greeterIndex]

		sayHelloIndex := descutil.FindMethodForService(greeter, "SayHello")
		assert.Loosely(t, sayHelloIndex, should.NotEqual(-1))
		sayHello := greeter.Method[sayHelloIndex]

		assert.Loosely(t, sayHello.GetInputType(), should.Equal(".testservices.HelloRequest"))
		_, obj, _ := descutil.Resolve(desc, "testservices.HelloRequest")
		assert.Loosely(t, obj, should.NotBeNil)
		helloReq := obj.(*descriptorpb.DescriptorProto)
		assert.Loosely(t, helloReq, should.NotBeNil)
		assert.Loosely(t, helloReq.Field, should.HaveLength(1))
		assert.Loosely(t, helloReq.Field[0].GetName(), should.Equal("name"))
		assert.Loosely(t, helloReq.Field[0].GetType(), should.Equal(descriptorpb.FieldDescriptorProto_TYPE_STRING))
	})
}
