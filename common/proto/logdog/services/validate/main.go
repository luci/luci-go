// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the LogDog Coordinator validation binary. This simply
// loads configuration from a text protobuf and verifies that it works.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/proto/logdog/services"
	"github.com/luci/luci-go/common/render"
)

func main() {
	src := flag.String("in", "", "Path to the input text protobuf.")
	flag.Parse()

	if *src == "" {
		log.Fatalln("Missing required input file path (-in).")
	}

	d, err := ioutil.ReadFile(*src)
	if err != nil {
		log.Fatalln("Failed to read input file: %v", err)
	}

	c := services.Config{}
	if err := proto.UnmarshalText(string(d), &c); err != nil {
		log.Fatalln("Failed to unmarshal input file: %v", err)
	}
	fmt.Println("Successfully unmarshalled configuration:\n", render.DeepRender(c))
}
