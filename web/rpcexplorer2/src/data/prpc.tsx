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


// gRPC status codes (see google/rpc/code.proto). Kept as UPPER_CASE to simplify
// converting them to a canonical string representation via StatusCode[code].
export enum StatusCode {
  OK = 0,
  CANCELLED = 1,
  UNKNOWN = 2,
  INVALID_ARGUMENT = 3,
  DEADLINE_EXCEEDED = 4,
  NOT_FOUND = 5,
  ALREADY_EXISTS = 6,
  PERMISSION_DENIED = 7,
  RESOURCE_EXHAUSTED = 8,
  FAILED_PRECONDITION = 9,
  ABORTED = 10,
  OUT_OF_RANGE = 11,
  UNIMPLEMENTED = 12,
  INTERNAL = 13,
  UNAVAILABLE = 14,
  DATA_LOSS = 15,
  UNAUTHENTICATED = 16,
}


// An RPC error with a status code and an error message.
export class RPCError extends Error {
  readonly code: StatusCode;
  readonly http: number;
  readonly text: string;

  constructor(code: StatusCode, http: number, text: string) {
    super(`${StatusCode[code]} (HTTP ${http}): ${text}`);
    Object.setPrototypeOf(this, RPCError.prototype);
    this.code = code;
    this.http = http;
    this.text = text;
  }
}


// Descriptors holds all type information about RPC APIs exposed by a server.
export class Descriptors {
  // A list of RPC services exposed by a server.
  readonly services: Service[] = [];

  constructor(desc: FileDescriptorSet, services: string[]) {
    for (const fileDesc of (desc.file ?? [])) {
      resolveSourceLocation(fileDesc); // mutates `fileDesc` in-place
    }
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
  // The last component of the name, e.g. `Discovery`.
  readonly title: string;
  // Short description e.g `Describes services.`.
  readonly help: string;
  // Paragraphs with full documentation.
  readonly doc: string;
  // List of methods exposed by the service.
  readonly methods: Method[] = [];

  constructor(name: string, desc: ServiceDescriptorProto) {
    this.name = name;
    this.title = splitFullName(name)[1];
    this.help = extractHelp(desc.resolvedSourceLocation, this.title, 'service');
    this.doc = extractDoc(desc.resolvedSourceLocation);
    if (desc.method !== undefined) {
      for (const methodDesc of desc.method) {
        this.methods.push(new Method(this.name, methodDesc));
      }
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
  // Short description e.g `Returns ...`.
  readonly help: string;
  // Paragraphs with full documentation.
  readonly doc: string;

  constructor(service: string, desc: MethodDescriptorProto) {
    this.service = service;
    this.name = desc.name;
    this.help = extractHelp(desc.resolvedSourceLocation, desc.name, 'method');
    this.doc = extractDoc(desc.resolvedSourceLocation);
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
    for (const fileDesc of (desc.file ?? [])) {
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
      for (const svc of (fileDesc.service ?? [])) {
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


// resolveSourceLocation recursively populates `resolvedSourceLocation`.
//
// See https://github.com/protocolbuffers/protobuf/blob/5a5683/src/google/protobuf/descriptor.proto#L803
const resolveSourceLocation = (desc: FileDescriptorProto) => {
  if (!desc.sourceCodeInfo) {
    return;
  }

  // A helper to convert a location path to a string key.
  const locationKey = (path: number[]): string => {
    return path.map((n) => n.toString()).join('.');
  };

  // Build a map {path -> SourceCodeInfoLocation}. See the link above for
  // what a path is.
  const locationMap = new Map<string, SourceCodeInfoLocation>();
  for (const location of (desc.sourceCodeInfo.location ?? [])) {
    if (location.path) {
      locationMap.set(locationKey(location.path), location);
    }
  }

  // Now recursively traverse the tree of definitions in the `desc` and
  // for each visited path see if there's a location for it in `locationMap`.
  //
  // Various magical numbers below are message field numbers defined in
  // descriptor.proto (again, see the link above, it is a pretty confusing
  // mechanism and comments in descriptor.proto are the best explanation).

  // Helper for recursively diving into a list of definitions of some kind.
  const resolveList = <T extends HasResolved>(
    path: number[],
    field: number,
    list: T[] | undefined,
    dive?: (path: number[], val: T) => void,
  ) => {
    if (!list) {
      return;
    }
    path.push(field);
    for (const [idx, val] of list.entries()) {
      path.push(idx);
      val.resolvedSourceLocation = locationMap.get(locationKey(path));
      if (dive) {
        dive(path, val);
      }
      path.pop();
    }
    path.pop();
  };

  const resolveService = (path: number[], msg: ServiceDescriptorProto) => {
    resolveList(path, 2, msg.method);
  };

  const resolveEnum = (path: number[], msg: EnumDescriptorProto) => {
    resolveList(path, 2, msg.value);
  };

  const resolveMessage = (path: number[], msg: DescriptorProto) => {
    resolveList(path, 2, msg.field);
    resolveList(path, 3, msg.nestedType, resolveMessage);
    resolveList(path, 4, msg.enumType, resolveEnum);
  };

  // Root lists.
  resolveList([], 4, desc.messageType, resolveMessage);
  resolveList([], 5, desc.enumType, resolveEnum);
  resolveList([], 6, desc.service, resolveService);
};


// extractHelp extracts a first paragraph of the leading comment and trims it
// a bit if necessary.
//
// A paragraph is delimited by '\n\n' or '.  '.
const extractHelp = (
    loc: SourceCodeInfoLocation | undefined,
    subject: string,
    type: string,
): string => {
  let comment = (loc ? loc.leadingComments ?? '' : '').trim();
  if (!comment) {
    return '';
  }

  // Get the first paragraph.
  comment = splitParagraph(comment)[0];

  // Go-based services very often have leading comments that start with
  // "ServiceName service does blah". We convert this to just "Does blah",
  // since we already show the service name prominently and this duplication
  // looks bad.
  const [trimmed, yes] = trimWord(comment, subject);
  if (yes) {
    const evenMore = trimWord(trimmed, type)[0];
    if (evenMore) {
      comment = evenMore.charAt(0).toUpperCase() + evenMore.substring(1);
    }
  }

  return comment;
};


// extractDoc converts comments into a list of paragraphs separated by '\n\n'.
const extractDoc = (loc: SourceCodeInfoLocation | undefined): string => {
  let text = (loc ? loc.leadingComments ?? '' : '').trim();
  const paragraphs: string[] = [];
  while (text) {
    const [p, remains] = splitParagraph(text);
    if (p) {
      paragraphs.push(p);
    }
    text = remains;
  }
  return paragraphs.join('\n\n');
};


// splitParagraph returns the first paragraph and what's left.
const splitParagraph = (s: string): [string, string] => {
  let idx = s.indexOf('\n\n');
  if (idx == -1) {
    idx = s.indexOf('.  ');
    if (idx != -1) {
      idx += 1; // include the dot
    }
  }
  if (idx != -1) {
    return [s.substring(0, idx).trim(), s.substring(idx).trim()];
  }
  return [s, ''];
};


// trimWord removes a leading word from a sentence, kind of.
const trimWord = (s: string, word: string): [string, boolean] => {
  if (s.startsWith(word + ' ')) {
    return [s.substring(word.length + 1).trimLeft(), true];
  }
  return [s, false];
};


// ".google.protobuf.Empty" => ["google.protobuf", "Empty"].
const splitFullName = (fullName: string): [string, string] => {
  const lastDot = fullName.lastIndexOf('.');
  const start = fullName.startsWith('.') ? 1 : 0;
  return [
    fullName.substring(start, lastDot),
    fullName.substring(lastDot + 1),
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

  // Parse gRPC status code header if available.
  let grpcCode = StatusCode.UNKNOWN;
  const codeStr = response.headers.get('X-Prpc-Grpc-Code');
  if (codeStr) {
    const num = parseInt(codeStr);
    if (isNaN(num) || num < 0 || num > 16) {
      grpcCode = StatusCode.UNKNOWN;
    } else {
      grpcCode = num;
    }
  } else if (response.status >= 500) {
    grpcCode = StatusCode.INTERNAL;
  }

  if (grpcCode != StatusCode.OK) {
    throw new RPCError(grpcCode, response.status, responseBody.trim());
  }

  return JSON.parse(responseBody) as T;
};


// See go.chromium.org/luci/grpc/discovery/service.proto and protos it imports
// (recursively).
//
// Only fields used by RPC Explorer are exposed below, using their JSONPB names.
//
// To simplify extraction of code comments for various definitions we also add
// an extra field `resolvedSourceLocation` to some interfaces. Its value is
// derived from `sourceCodeInfo` in resolveSourceLocation.

// Implemented by descriptors that have resolve source location attached.
interface HasResolved {
  resolvedSourceLocation?: SourceCodeInfoLocation;
}

interface DescribeResponse {
  description: FileDescriptorSet;
  services: string[];
}

export interface FileDescriptorSet {
  file?: FileDescriptorProto[];
}

interface FileDescriptorProto {
  name: string;
  package: string;

  messageType?: DescriptorProto[]; // = 4
  enumType?: EnumDescriptorProto[]; // = 5;
  service?: ServiceDescriptorProto[]; // = 6

  sourceCodeInfo?: SourceCodeInfo;
}

interface DescriptorProto extends HasResolved {
  name: string;
  field?: FieldDescriptorProto[]; // = 2
  nestedType?: DescriptorProto[]; // = 3
  enumType?: EnumDescriptorProto[]; // = 4
}

interface FieldDescriptorProto extends HasResolved {
  name: string;
  jsonName?: string;
  type: string; // e.g. "TYPE_INT32"
  typeName?: string; // for message and enum types only
}

interface EnumDescriptorProto extends HasResolved {
  name: string;
  value?: EnumValueDescriptorProto[]; // = 2;
}

interface EnumValueDescriptorProto extends HasResolved {
  name: string;
  number: number;
}

interface ServiceDescriptorProto extends HasResolved {
  name: string;
  method?: MethodDescriptorProto[]; // = 2
}

interface MethodDescriptorProto extends HasResolved {
  name: string;

  inputType: string;
  outputType: string;
}

interface SourceCodeInfo {
  location?: SourceCodeInfoLocation[];
}

interface SourceCodeInfoLocation {
  path: number[];
  leadingComments?: string;
}
