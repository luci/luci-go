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
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/proto/google/descutil"
	"go.chromium.org/luci/grpc/discovery"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"

	. "github.com/smartystreets/goconvey/convey"
)

// force test services registration.
var _ = CalcServer(nil)

func TestDiscovery(t *testing.T) {
	Convey("Discovery", t, func() {

		server, err := discovery.New(
			"discovery.Discovery",
			"testservices.Greeter",
			"testservices.Calc",
		)
		So(err, ShouldBeNil)

		c := context.Background()
		res, err := server.Describe(c, nil)
		So(err, ShouldBeNil)

		So(res.Services, ShouldResemble, []string{
			"discovery.Discovery",
			"testservices.Greeter",
			"testservices.Calc",
		})

		desc := res.Description

		// this checks that file deduplication actually works.
		So(len(desc.File), ShouldEqual, 3)

		_, discoveryIndex := descutil.FindService(desc, "discovery.Discovery")
		So(discoveryIndex, ShouldNotEqual, -1)

		_, calcIndex := descutil.FindService(desc, "testservices.Calc")
		So(calcIndex, ShouldNotEqual, -1)

		file, greeterIndex := descutil.FindService(desc, "testservices.Greeter")
		So(greeterIndex, ShouldNotEqual, -1)
		greeter := file.Service[greeterIndex]

		sayHelloIndex := descutil.FindMethodForService(greeter, "SayHello")
		So(sayHelloIndex, ShouldNotEqual, -1)
		sayHello := greeter.Method[sayHelloIndex]

		So(sayHello.GetInputType(), ShouldEqual, ".testservices.HelloRequest")
		_, obj, _ := descutil.Resolve(desc, "testservices.HelloRequest")
		So(obj, ShouldNotBeNil)
		helloReq := obj.(*descriptor.DescriptorProto)
		So(helloReq, ShouldNotBeNil)
		So(helloReq.Field, ShouldHaveLength, 1)
		So(helloReq.Field[0].GetName(), ShouldEqual, "name")
		So(helloReq.Field[0].GetType(), ShouldEqual, descriptor.FieldDescriptorProto_TYPE_STRING)
	})
}
