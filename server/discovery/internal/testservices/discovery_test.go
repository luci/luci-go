// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testservices

// This test is not in discovery because it needs to a test services
// in different directories.
// However, a generated service depends on server/discovery
// and a test in server/discovery depends on the service.
// This creates a a cyclic import.
// To break the cycle, we move this test from server/discovery.

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/proto/google/descriptor"
	"github.com/luci/luci-go/server/discovery"

	. "github.com/luci/luci-go/common/testing/assertions"
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

		So(res.Services, ShouldResembleV, []string{
			"discovery.Discovery",
			"testservices.Greeter",
			"testservices.Calc",
		})

		desc := &descriptor.FileDescriptorSet{}
		err = proto.Unmarshal(res.FileDescriptionSet, desc)
		So(err, ShouldBeNil)

		// this checks that file deduplication actually works.
		So(len(desc.File), ShouldEqual, 2)

		discovery := desc.FindService("discovery.Discovery")
		So(discovery, ShouldNotBeNil)

		calc := desc.FindService("testservices.Calc")
		So(calc, ShouldNotBeNil)

		serviceDesc := desc.FindService("testservices.Greeter")
		So(serviceDesc, ShouldNotBeNil)

		sayHelloDesc := serviceDesc.FindMethod("SayHello")
		So(sayHelloDesc, ShouldNotBeNil)

		So(sayHelloDesc.GetInputType(), ShouldEqual, ".testservices.HelloRequest")
		helloReq := desc.FindMessage("testservices.HelloRequest")
		So(helloReq, ShouldNotBeNil)
		So(helloReq.Field, ShouldHaveLength, 1)
		So(helloReq.Field[0].GetName(), ShouldEqual, "name")
		So(helloReq.Field[0].GetType(), ShouldEqual, descriptor.FieldDescriptorProto_TYPE_STRING)
	})
}
