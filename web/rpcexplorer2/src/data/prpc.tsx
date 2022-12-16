// Copyright 2022 The LUCI Authors.
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

// Descriptors holds all type information about RPC APIs exposed by a server.
export class Descriptors {
  private serviceMap = new Map<string, Service>();

  // TODO: Actually fetch.
  constructor() {
    this.serviceMap.set('service1', new Service('service1'));
    this.serviceMap.set('service2', new Service('service2'));
  }

  // Returns a list of RPC services exposed by a server.
  services(): Service[] {
    return Array.from(this.serviceMap.values());
  }

  // Returns a service by its name or null if no such service.
  service(serviceName: string): Service | null {
    return this.serviceMap.get(serviceName) || null;
  }
}


// Descriptions of a single RPC service.
export class Service {
  // Name of the service, e.g. `discovery.Discovery`.
  name: string;
  // Short description e.g `discovery.Discovery describes services.`.
  help: string;

  private methodMap = new Map<string, Method>();

  // TODO: Actually fetch.
  constructor(name: string, help = '') {
    this.name = name;
    this.help = help;
    this.methodMap.set('method1', new Method('method1'));
    this.methodMap.set('method2', new Method('method2'));
  }

  // Returns a list of RPC methods exposed by this service.
  methods(): Method[] {
    return Array.from(this.methodMap.values());
  }

  // Returns a method by its name or null if no such method.
  method(methodName: string): Method | null {
    return this.methodMap.get(methodName) || null;
  }
}


// Method describes a single RPC method.
export class Method {
  // Name of the method, e.g. `Describe `.
  name: string;
  // Short description e.g `Describe returns ...`.
  help: string;

  // TODO: Actually fetch.
  constructor(name: string, help = '') {
    this.name = name;
    this.help = help;
  }
}


// Loads RPC API descriptors from the server.
export const loadDescriptors = async (): Promise<Descriptors> => {
  // TODO: Implement.
  return new Descriptors();
};
