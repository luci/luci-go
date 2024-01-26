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
  // Helpers for visiting descriptors.
  private readonly types: Types;
  // A cache of visited message types.
  private readonly messages: Cache<Message>;
  // A cache of visited enum types.
  private readonly enums: Cache<Enum>;

  constructor(desc: FileDescriptorSet, services: string[]) {
    for (const fileDesc of (desc.file ?? [])) {
      resolveSourceLocation(fileDesc); // mutates `fileDesc` in-place
    }
    this.types = new Types(desc);
    for (const serviceName of services) {
      const desc = this.types.service(serviceName);
      if (desc) {
        this.services.push(new Service(serviceName, desc));
      }
    }
    this.messages = new Cache();
    this.enums = new Cache();
  }

  // Returns a service by its name or undefined if no such service.
  service(serviceName: string): Service | undefined {
    return this.services.find((service) => service.name == serviceName);
  }

  // Returns a message descriptor given its name or undefined if not found.
  message(messageName: string): Message | undefined {
    return this.messages.get(messageName, () => {
      const desc = this.types.message(messageName);
      return desc ? new Message(desc, this) : undefined;
    });
  }

  // Returns an enum descriptor given its name or undefined if not found.
  enum(enumName: string): Enum | undefined {
    return this.enums.get(enumName, () => {
      const desc = this.types.enum(enumName);
      return desc ? new Enum(desc) : undefined;
    });
  }
}


// Describes a structure of a message.
export class Message {
  // True if this message represents some map<K,V> entry.
  readonly mapEntry: boolean;
  // The list of fields of this message.
  readonly fields: Field[];

  constructor(desc: DescriptorProto, root: Descriptors) {
    this.mapEntry = desc.options?.mapEntry ?? false;
    this.fields = [];
    for (const field of (desc.field ?? [])) {
      this.fields.push(new Field(field, root));
    }
  }

  // Returns a field given its JSON name.
  fieldByJsonName(name: string): Field | undefined {
    for (const field of this.fields) {
      if (field.jsonName == name) {
        return field;
      }
    }
    return undefined;
  }
}


// Describes possible values of an enum.
export class Enum {
  // Possible values of an enum.
  readonly values: EnumValue[];

  constructor(desc: EnumDescriptorProto) {
    this.values = [];
    for (const val of (desc.value ?? [])) {
      this.values.push(new EnumValue(val));
    }
  }
}


// A single possible value of an enum
export class EnumValue {
  // E.g. "SOME_VALUE";
  readonly name: string;

  private readonly desc: EnumValueDescriptorProto;
  private cachedDoc: string | null;

  constructor(desc: EnumValueDescriptorProto) {
    this.name = desc.name;
    this.desc = desc;
    this.cachedDoc = null;
  }

  // Paragraphs with documentation extracted from comments in the proto file.
  get doc(): string {
    if (this.cachedDoc == null) {
      this.cachedDoc = extractDoc(this.desc.resolvedSourceLocation);
    }
    return this.cachedDoc;
  }
}


// How field value is encoded in JSON.
export enum JSONType {
  Object,
  List,
  String,
  Scalar, // numbers, booleans, null
}


// Describes a structure of a single field.
export class Field {
  // Fields JSON name, e.g. `someField`.
  readonly jsonName: string;
  // True if this is a repeated field or a map field.
  readonly repeated: boolean;

  private readonly desc: FieldDescriptorProto;
  private readonly root: Descriptors;
  private cachedDoc: string | null;

  static readonly scalarTypeNames = new Map([
    ['TYPE_DOUBLE', 'double'],
    ['TYPE_FLOAT', 'float'],
    ['TYPE_INT64', 'int64'],
    ['TYPE_UINT64', 'uint64'],
    ['TYPE_INT32', 'int32'],
    ['TYPE_FIXED64', 'fixed64'],
    ['TYPE_FIXED32', 'fixed32'],
    ['TYPE_BOOL', 'bool'],
    ['TYPE_STRING', 'string'],
    ['TYPE_BYTES', 'bytes'],
    ['TYPE_UINT32', 'uint32'],
    ['TYPE_SFIXED32', 'sfixed32'],
    ['TYPE_SFIXED64', 'sfixed64'],
    ['TYPE_SINT32', 'sint32'],
    ['TYPE_SINT64', 'sint64'],
  ]);

  constructor(desc: FieldDescriptorProto, root: Descriptors) {
    this.desc = desc;
    this.root = root;
    this.jsonName = desc.jsonName ?? '';
    this.repeated = desc.label == 'LABEL_REPEATED';
    this.cachedDoc = null;
  }

  // For message-valued fields, the type of the field value.
  //
  // Works for both singular and repeated fields.
  get message(): Message | undefined {
    if (this.desc.type == 'TYPE_MESSAGE') {
      return this.root.message(this.desc.typeName ?? '');
    }
    return undefined;
  }

  // For enum-valued fields, the type of the field value.
  //
  // Works for both singular and repeated fields.
  get enum(): Enum | undefined {
    if (this.desc.type == 'TYPE_ENUM') {
      return this.root.enum(this.desc.typeName ?? '');
    }
    return undefined;
  }

  // Field type name, approximately as it appears in the proto file.
  //
  // Recognizes repeated fields and maps.
  get type(): string {
    const pfx = this.repeated ? 'repeated ' : '';
    const scalar = Field.scalarTypeNames.get(this.desc.type);
    if (scalar) {
      return pfx + scalar;
    }
    const message = this.message;
    if (message && message.mapEntry) {
      // Note: mapEntry fields are always implicitly repeated, omit `pfx`.
      return `map<${message.fields[0].type}, ${message.fields[1].type}>`;
    }
    return pfx + trimDot(this.desc.typeName ?? 'unknown');
  }

  // Type of the field value in JSON representation.
  //
  // Recognizes repeated fields and maps.
  get jsonType(): JSONType {
    if (this.repeated) {
      if (this.message?.mapEntry) {
        return JSONType.Object;
      }
      return JSONType.List;
    }
    return this.jsonElementType;
  }

  // JSON type of the base element.
  //
  // For repeated fields it is the type inside the list.
  get jsonElementType(): JSONType {
    switch (this.desc.type) {
      case 'TYPE_MESSAGE':
        return JSONType.Object;
      case 'TYPE_ENUM':
      case 'TYPE_STRING':
      case 'TYPE_BYTES':
        return JSONType.String;
      default:
        return JSONType.Scalar;
    }
  }

  // Paragraphs with documentation extracted from comments in the proto file.
  get doc(): string {
    if (this.cachedDoc == null) {
      this.cachedDoc = extractDoc(this.desc.resolvedSourceLocation);
    }
    return this.cachedDoc;
  }
}


// Descriptions of a single RPC service.
export class Service {
  // Name of the service, e.g. `discovery.Discovery`.
  readonly name: string;
  // The last component of the name, e.g. `Discovery`.
  readonly title: string;
  // Short description e.g. `Describes services.`.
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
  // Short description e.g. `Returns ...`.
  readonly help: string;
  // Paragraphs with full documentation.
  readonly doc: string;
  // Name of the protobuf message type of the request.
  readonly requestType: string;

  constructor(service: string, desc: MethodDescriptorProto) {
    this.service = service;
    this.name = desc.name;
    this.help = extractHelp(desc.resolvedSourceLocation, desc.name, 'method');
    this.doc = extractDoc(desc.resolvedSourceLocation);
    this.requestType = desc.inputType;
  }

  // Invokes this method, returns prettified JSON response as a string.
  //
  // `authorization` will be used as a value of Authorization header. `traceID`
  // is used to populate X-Cloud-Trace-Context header.
  async invoke(
      request: string,
      authorization: string,
      traceID?: string,
  ): Promise<string> {
    const resp: object = await invokeMethod(
        this.service, this.name, request, authorization, traceID,
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


// Generates a tracing ID for Cloud Trace.
//
// See https://cloud.google.com/trace/docs/setup#force-trace.
export const generateTraceID = (): string => {
  let output = '';
  for (let i = 0; i < 32; ++i) {
    output += (Math.floor(Math.random() * 16)).toString(16);
  }
  return output;
};


// Private guts.


// A helper to cache visited types.
class Cache<V> {
  private cache: Map<string, V | 'none'>;

  constructor() {
    this.cache = new Map();
  }

  get(key: string, val: () => V | undefined): V | undefined {
    let cached = this.cache.get(key);
    switch (cached) {
      case 'none':
      // Cached "absence".
        return undefined;
      case undefined:
      // Not in the cache yet.
        cached = val();
        this.cache.set(key, cached == undefined ? 'none' : cached);
        return cached;
      default:
        return cached;
    }
  }
}


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

  // Given a full message name returns its descriptor or undefined.
  message(fullName: string): DescriptorProto | undefined {
    // Assume this is a top-level type defined in some package.
    const [pkg, name] = splitFullName(fullName);
    const found = this.visitPackage(pkg, (fileDesc) => {
      for (const desc of (fileDesc.messageType ?? [])) {
        if (desc.name == name) {
          return desc;
        }
      }
      return undefined;
    });
    if (found) {
      return found;
    }
    // It might be a reference to a nested type, in which case `pkg` is actually
    // some message name itself. Try to find it.
    const parent = this.message(pkg);
    if (parent) {
      for (const desc of (parent.nestedType ?? [])) {
        if (desc.name == name) {
          return desc;
        }
      }
    }
    return undefined;
  }

  // Given a full enum name returns its descriptor or undefined.
  enum(fullName: string): EnumDescriptorProto | undefined {
    // Assume this is a top-level type defined in some package.
    const [pkg, name] = splitFullName(fullName);
    const found = this.visitPackage(pkg, (fileDesc) => {
      for (const msg of (fileDesc.enumType ?? [])) {
        if (msg.name == name) {
          return msg;
        }
      }
      return undefined;
    });
    if (found) {
      return found;
    }
    // It might be a reference to a nested type, in which case `pkg` is actually
    // some message name. Try to find it.
    const parent = this.message(pkg);
    if (parent) {
      for (const msg of (parent.enumType ?? [])) {
        if (msg.name == name) {
          return msg;
        }
      }
    }
    return undefined;
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

  return deindent(comment);
};


// extractDoc converts comments into a list of paragraphs separated by '\n\n'.
const extractDoc = (loc: SourceCodeInfoLocation | undefined): string => {
  let text = (loc ? loc.leadingComments ?? '' : '').trim();
  const paragraphs: string[] = [];
  while (text) {
    const [p, remains] = splitParagraph(text);
    if (p) {
      paragraphs.push(deindent(p));
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


// deindent removes a leading space from all lines that have it.
//
// Protobuf docs are generated from comments not unlike this one. `protoc`
// strips `//` but does nothing to spaces that follow `//`. As a result docs are
// intended by one space.
const deindent = (s: string): string => {
  const lines = s.split('\n');
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].startsWith(' ')) {
      lines[i] = lines[i].substring(1);
    }
  }
  return lines.join('\n');
};


// trimWord removes a leading word from a sentence, kind of.
const trimWord = (s: string, word: string): [string, boolean] => {
  if (s.startsWith(word + ' ')) {
    return [s.substring(word.length + 1).trimLeft(), true];
  }
  return [s, false];
};


// ".google.protobuf.Empty" => "google.protobuf.Empty";
const trimDot = (name: string): string => {
  if (name.startsWith('.')) {
    return name.substring(1);
  }
  return name;
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
  traceID?: string,
): Promise<T> => {
  const headers = new Headers();
  headers.set('Accept', 'application/json; charset=utf-8');
  headers.set('Content-Type', 'application/json; charset=utf-8');
  if (authorization) {
    headers.set('Authorization', authorization);
  }
  if (traceID) {
    headers.set('X-Cloud-Trace-Context', `${traceID}/1;o=1`);
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
  options?: MessageOptions;
}

interface FieldDescriptorProto extends HasResolved {
  name: string;
  jsonName?: string;
  type: string; // e.g. "TYPE_INT32"
  typeName?: string; // for message and enum types only
  label: string; // e.g. "LABEL_REPEATED"
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

interface MessageOptions {
  mapEntry?: boolean;
}

interface SourceCodeInfo {
  location?: SourceCodeInfoLocation[];
}

interface SourceCodeInfoLocation {
  path: number[];
  leadingComments?: string;
}
