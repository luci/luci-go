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
  // A list of RPC services exposed by a server.
  readonly services: Service[] = [];

  constructor(desc: FileDescriptorSet, services: string[]) {
    const types = new Types(desc);
    for (const serviceName of services) {
      const desc = types.service(serviceName);
      if (desc) {
        this.services.push(new Service(serviceName, desc));
      }
    }
  }

  // Returns a service by its name or undefined if no such service.
  service(serviceName: string): Service | undefined {
    return this.services.find((service) => service.name == serviceName);
  }
}


// Descriptions of a single RPC service.
export class Service {
  // Name of the service, e.g. `discovery.Discovery`.
  readonly name: string;
  // Short description e.g `discovery.Discovery describes services.`.
  readonly help: string;
  // List of methods exposed by the service.
  readonly methods: Method[] = [];

  constructor(name: string, desc: ServiceDescriptorProto) {
    this.name = name;
    this.help = 'TODO';
    for (const methodDesc of desc.method) {
      this.methods.push(new Method(this.name, methodDesc));
    }
  }

  // Returns a method by its name or undefined if no such method.
  method(methodName: string): Method | undefined {
    return this.methods.find((method) => method.name == methodName);
  }
}


// Method describes a single RPC method.
export class Method {
  // Service name the method belongs to.
  readonly service: string;
  // Name of the method, e.g. `Describe `.
  readonly name: string;
  // Short description e.g `Describe returns ...`.
  readonly help: string;

  constructor(service: string, desc: MethodDescriptorProto) {
    this.service = service;
    this.name = desc.name;
    this.help = 'TODO';
  }

  // Invokes this method, returns prettified JSON response as a string.
  //
  // `authorization` will be used as a value of Authorization header.
  async invoke(request: string, authorization: string): Promise<string> {
    const resp: object = await invokeMethod(
        this.service, this.name, request, authorization,
    );
    return JSON.stringify(resp, null, 2);
  }
}


// Loads RPC API descriptors from the server.
export const loadDescriptors = async (): Promise<Descriptors> => {
  const resp: DescribeResponse = await invokeMethod(
      'discovery.Discovery', 'Describe', '{}', '',
  );
  return new Descriptors(resp.description, resp.services);
};


// /////////////////////////////////////////////////////////////////////////////
// Private guts.


// Types knows how to look up proto descriptors given full proto type names.
class Types {
  // Proto package name => list of FileDescriptorProto where it is defined.
  private packageMap = new Map<string, FileDescriptorProto[]>();

  constructor(desc: FileDescriptorSet) {
    for (const fileDesc of desc.file) {
      const descs = this.packageMap.get(fileDesc.package);
      if (descs === undefined) {
        this.packageMap.set(fileDesc.package, [fileDesc]);
      } else {
        descs.push(fileDesc);
      }
    }
  }

  // Given a full service name returns its descriptor or undefined.
  service(fullName: string): ServiceDescriptorProto | undefined {
    const [pkg, name] = splitFullName(fullName);
    return this.visitPackage(pkg, (fileDesc) => {
      for (const svc of fileDesc.service) {
        if (svc.name == name) {
          return svc;
        }
      }
      return undefined;
    });
  }

  // Visits all FileDescriptorProto that define a particular package.
  private visitPackage<T>(
      pkg: string,
      cb: (desc: FileDescriptorProto) => T | undefined,
  ): T | undefined {
    const descs = this.packageMap.get(pkg);
    if (descs !== undefined) {
      for (const desc of descs) {
        const res = cb(desc);
        if (res !== undefined) {
          return res;
        }
      }
    }
    return undefined;
  }
}


// ".google.protobuf.Empty" => ["google.protobuf", "Empty"].
const splitFullName = (fullName: string): [string, string] => {
  const lastDot = fullName.lastIndexOf('.');
  const start = fullName.startsWith('.') ? 1 : 0;
  return [
    fullName.substring(start, lastDot),
    fullName.substring(lastDot+1),
  ];
};


// invokeMethod sends a pRPC request and parses the response.
const invokeMethod = async <T, >(
  service: string,
  method: string,
  body: string,
  authorization: string,
): Promise<T> => {
  const headers = new Headers();
  headers.set('Accept', 'application/json; charset=utf-8');
  headers.set('Content-Type', 'application/json; charset=utf-8');
  if (authorization) {
    headers.set('Authorization', authorization);
  }

  const response = await fetch(`/prpc/${service}/${method}`, {
    method: 'POST',
    credentials: 'omit',
    headers: headers,
    body: body,
  });
  let responseBody = await response.text();

  // Trim the magic anti-XSS prefix if present.
  const pfx = ')]}\'';
  if (responseBody.startsWith(pfx)) {
    responseBody = responseBody.slice(pfx.length);
  }

  // TODO: Parse pRPC error details.
  if (!response.ok) {
    throw Error(`HTTP ${response.status}: ${responseBody}`);
  }

  return JSON.parse(responseBody) as T;
};


// See go.chromium.org/luci/grpc/discovery/service.proto and protos it imports
// (recursively).
//
// Only fields used by RPC Explorer are exposed below, using their JSONPB names.


interface DescribeResponse {
  description: FileDescriptorSet;
  services: string[];
}

interface FileDescriptorSet {
  file: FileDescriptorProto[];
}

interface FileDescriptorProto {
  name: string;
  package: string;

  messageType: DescriptorProto[];
  service: ServiceDescriptorProto[];

  sourceCodeInfo?: SourceCodeInfo;
}

interface DescriptorProto {
  name: string;
}

interface ServiceDescriptorProto {
  name: string;

  method: MethodDescriptorProto[];
}

interface MethodDescriptorProto {
  name: string;

  inputType: string;
  outputType: string;
}

interface SourceCodeInfo {
  location: SourceCodeInfoLocation[];
}

interface SourceCodeInfoLocation {
  path: number[];
  leadingComments?: string;
}
