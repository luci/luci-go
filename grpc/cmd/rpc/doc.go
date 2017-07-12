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

// Command rpc can make RPCs to pRPC servers and display their description.
//
// Subcommand call
//
// call subcommand reads from stdin, sends a request to a specified service
// at a specified server in a special format (defaults to json) and
// prints the response back to stdout.
//
//  $ echo '{"name": "Lucy"}' | rpc call :8080 helloworld.Greeter.SayHello
//  {
//          "message": "Hello Lucy"
//  }
//
// Subcommand show
//
// show subcommand resolves a name and describes the referenced entity
// in proto-like syntax. If name is not specified, lists available services.
//
//  $ rpc show :8080
//  helloworld.Greeter
//  discovery.Discovery
//
// Show a service:
//
//  $ rpc show :8080 helloworld.Greeter
//  // The greeting service definition.
//  service Greeter {
//          // Sends a greeting
//          rpc SayHello(HelloRequest) returns (HelloReply) {};
//  }
//
// Show a method:
//
//  $ rpc show :8080 helloworld.Greeter.SayHello
//  // The greeting service definition.
//  service Greeter {
//          // Sends a greeting
//          rpc SayHello(HelloRequest) returns (HelloReply) {};
//  }
//
//  // The request message containing the user's name.
//  message HelloRequest {
//          string name = 1;
//  }
//
//  // The response message containing the greetings
//  message HelloReply {
//          string message = 1;
//  }
//
// Show a type:
//
//  $ rpc show :8080 helloworld.HelloReply
//  // The response message containing the greetings
//  message HelloReply {
//          string message = 1;
//  }
package main
