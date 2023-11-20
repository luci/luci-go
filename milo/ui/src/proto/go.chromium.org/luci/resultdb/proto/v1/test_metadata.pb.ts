/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";
import { Struct } from "../../../../../google/protobuf/struct.pb";
import { SourceRef } from "./common.pb";

export const protobufPackage = "luci.resultdb.v1";

/** Information about a test metadata. */
export interface TestMetadataDetail {
  /**
   * Can be used to refer to a test metadata, e.g. in ResultDB.QueryTestMetadata
   * RPC.
   * Format:
   * "projects/{PROJECT}/refs/{REF_HASH}/tests/{URL_ESCAPED_TEST_ID}".
   * where URL_ESCAPED_TEST_ID is test_id escaped with
   * https://golang.org/pkg/net/url/#PathEscape. See also https://aip.dev/122.
   *
   * Output only.
   */
  readonly name: string;
  /** The LUCI project. */
  readonly project: string;
  /**
   * A unique identifier of a test in a LUCI project.
   * Refer to TestResult.test_id for details.
   */
  readonly testId: string;
  /**
   * Hexadecimal encoded hash string of the source_ref.
   * A given source_ref always hashes to the same ref_hash value.
   */
  readonly refHash: string;
  /** A reference in the source control system where the test metadata comes from. */
  readonly sourceRef:
    | SourceRef
    | undefined;
  /** Test metadata content. */
  readonly testMetadata: TestMetadata | undefined;
}

/** Information about a test. */
export interface TestMetadata {
  /** The original test name. */
  readonly name: string;
  /**
   * Where the test is defined, e.g. the file name.
   * location.repo MUST be specified.
   */
  readonly location:
    | TestLocation
    | undefined;
  /**
   * The issue tracker component associated with the test, if any.
   * Bugs related to the test may be filed here.
   */
  readonly bugComponent:
    | BugComponent
    | undefined;
  /**
   * Identifies the schema of the JSON object in the properties field.
   * Use the fully-qualified name of the source protocol buffer.
   * eg. chromiumos.test.api.TestCaseInfo
   * ResultDB will *not* validate the properties field with respect to this
   * schema. Downstream systems may however use this field to inform how the
   * properties field is interpreted.
   */
  readonly propertiesSchema: string;
  /**
   * Arbitrary JSON object that contains structured, domain-specific properties
   * of the test.
   *
   * The serialized size must be <= 4096 bytes.
   *
   * If this field is specified, properties_schema must also be specified.
   */
  readonly properties: { readonly [key: string]: any } | undefined;
}

/** Location of the test definition. */
export interface TestLocation {
  /**
   * Gitiles URL as the identifier for a repo.
   * Format for Gitiles URL: https://<host>/<project>
   * For example "https://chromium.googlesource.com/chromium/src"
   * Must not end with ".git".
   * SHOULD be specified.
   */
  readonly repo: string;
  /**
   * Name of the file where the test is defined.
   * For files in a repository, must start with "//"
   * Example: "//components/payments/core/payment_request_data_util_unittest.cc"
   * Max length: 512.
   * MUST not use backslashes.
   * Required.
   */
  readonly fileName: string;
  /** One-based line number where the test is defined. */
  readonly line: number;
}

/**
 * Represents a component in an issue tracker. A component is
 * a container for issues.
 */
export interface BugComponent {
  /** The Google Issue Tracker component. */
  readonly issueTracker?:
    | IssueTrackerComponent
    | undefined;
  /** The monorail component. */
  readonly monorail?: MonorailComponent | undefined;
}

/**
 * A component in Google Issue Tracker, sometimes known as Buganizer,
 * available at https://issuetracker.google.com.
 */
export interface IssueTrackerComponent {
  /** The Google Issue Tracker component ID. */
  readonly componentId: string;
}

/**
 * A component in monorail issue tracker, available at
 * https://bugs.chromium.org.
 */
export interface MonorailComponent {
  /** The monorail project name. */
  readonly project: string;
  /** The monorail component value. E.g. "Blink>Accessibility". */
  readonly value: string;
}

function createBaseTestMetadataDetail(): TestMetadataDetail {
  return { name: "", project: "", testId: "", refHash: "", sourceRef: undefined, testMetadata: undefined };
}

export const TestMetadataDetail = {
  encode(message: TestMetadataDetail, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.project !== "") {
      writer.uint32(18).string(message.project);
    }
    if (message.testId !== "") {
      writer.uint32(26).string(message.testId);
    }
    if (message.refHash !== "") {
      writer.uint32(98).string(message.refHash);
    }
    if (message.sourceRef !== undefined) {
      SourceRef.encode(message.sourceRef, writer.uint32(34).fork()).ldelim();
    }
    if (message.testMetadata !== undefined) {
      TestMetadata.encode(message.testMetadata, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestMetadataDetail {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestMetadataDetail() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.project = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.testId = reader.string();
          continue;
        case 12:
          if (tag !== 98) {
            break;
          }

          message.refHash = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.sourceRef = SourceRef.decode(reader, reader.uint32());
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.testMetadata = TestMetadata.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestMetadataDetail {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      testId: isSet(object.testId) ? globalThis.String(object.testId) : "",
      refHash: isSet(object.refHash) ? globalThis.String(object.refHash) : "",
      sourceRef: isSet(object.sourceRef) ? SourceRef.fromJSON(object.sourceRef) : undefined,
      testMetadata: isSet(object.testMetadata) ? TestMetadata.fromJSON(object.testMetadata) : undefined,
    };
  },

  toJSON(message: TestMetadataDetail): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.testId !== "") {
      obj.testId = message.testId;
    }
    if (message.refHash !== "") {
      obj.refHash = message.refHash;
    }
    if (message.sourceRef !== undefined) {
      obj.sourceRef = SourceRef.toJSON(message.sourceRef);
    }
    if (message.testMetadata !== undefined) {
      obj.testMetadata = TestMetadata.toJSON(message.testMetadata);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestMetadataDetail>, I>>(base?: I): TestMetadataDetail {
    return TestMetadataDetail.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestMetadataDetail>, I>>(object: I): TestMetadataDetail {
    const message = createBaseTestMetadataDetail() as any;
    message.name = object.name ?? "";
    message.project = object.project ?? "";
    message.testId = object.testId ?? "";
    message.refHash = object.refHash ?? "";
    message.sourceRef = (object.sourceRef !== undefined && object.sourceRef !== null)
      ? SourceRef.fromPartial(object.sourceRef)
      : undefined;
    message.testMetadata = (object.testMetadata !== undefined && object.testMetadata !== null)
      ? TestMetadata.fromPartial(object.testMetadata)
      : undefined;
    return message;
  },
};

function createBaseTestMetadata(): TestMetadata {
  return { name: "", location: undefined, bugComponent: undefined, propertiesSchema: "", properties: undefined };
}

export const TestMetadata = {
  encode(message: TestMetadata, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.location !== undefined) {
      TestLocation.encode(message.location, writer.uint32(18).fork()).ldelim();
    }
    if (message.bugComponent !== undefined) {
      BugComponent.encode(message.bugComponent, writer.uint32(26).fork()).ldelim();
    }
    if (message.propertiesSchema !== "") {
      writer.uint32(34).string(message.propertiesSchema);
    }
    if (message.properties !== undefined) {
      Struct.encode(Struct.wrap(message.properties), writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestMetadata {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestMetadata() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.location = TestLocation.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.bugComponent = BugComponent.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.propertiesSchema = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.properties = Struct.unwrap(Struct.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestMetadata {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      location: isSet(object.location) ? TestLocation.fromJSON(object.location) : undefined,
      bugComponent: isSet(object.bugComponent) ? BugComponent.fromJSON(object.bugComponent) : undefined,
      propertiesSchema: isSet(object.propertiesSchema) ? globalThis.String(object.propertiesSchema) : "",
      properties: isObject(object.properties) ? object.properties : undefined,
    };
  },

  toJSON(message: TestMetadata): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.location !== undefined) {
      obj.location = TestLocation.toJSON(message.location);
    }
    if (message.bugComponent !== undefined) {
      obj.bugComponent = BugComponent.toJSON(message.bugComponent);
    }
    if (message.propertiesSchema !== "") {
      obj.propertiesSchema = message.propertiesSchema;
    }
    if (message.properties !== undefined) {
      obj.properties = message.properties;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestMetadata>, I>>(base?: I): TestMetadata {
    return TestMetadata.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestMetadata>, I>>(object: I): TestMetadata {
    const message = createBaseTestMetadata() as any;
    message.name = object.name ?? "";
    message.location = (object.location !== undefined && object.location !== null)
      ? TestLocation.fromPartial(object.location)
      : undefined;
    message.bugComponent = (object.bugComponent !== undefined && object.bugComponent !== null)
      ? BugComponent.fromPartial(object.bugComponent)
      : undefined;
    message.propertiesSchema = object.propertiesSchema ?? "";
    message.properties = object.properties ?? undefined;
    return message;
  },
};

function createBaseTestLocation(): TestLocation {
  return { repo: "", fileName: "", line: 0 };
}

export const TestLocation = {
  encode(message: TestLocation, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.repo !== "") {
      writer.uint32(10).string(message.repo);
    }
    if (message.fileName !== "") {
      writer.uint32(18).string(message.fileName);
    }
    if (message.line !== 0) {
      writer.uint32(24).int32(message.line);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestLocation {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestLocation() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.repo = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.fileName = reader.string();
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.line = reader.int32();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestLocation {
    return {
      repo: isSet(object.repo) ? globalThis.String(object.repo) : "",
      fileName: isSet(object.fileName) ? globalThis.String(object.fileName) : "",
      line: isSet(object.line) ? globalThis.Number(object.line) : 0,
    };
  },

  toJSON(message: TestLocation): unknown {
    const obj: any = {};
    if (message.repo !== "") {
      obj.repo = message.repo;
    }
    if (message.fileName !== "") {
      obj.fileName = message.fileName;
    }
    if (message.line !== 0) {
      obj.line = Math.round(message.line);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestLocation>, I>>(base?: I): TestLocation {
    return TestLocation.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestLocation>, I>>(object: I): TestLocation {
    const message = createBaseTestLocation() as any;
    message.repo = object.repo ?? "";
    message.fileName = object.fileName ?? "";
    message.line = object.line ?? 0;
    return message;
  },
};

function createBaseBugComponent(): BugComponent {
  return { issueTracker: undefined, monorail: undefined };
}

export const BugComponent = {
  encode(message: BugComponent, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.issueTracker !== undefined) {
      IssueTrackerComponent.encode(message.issueTracker, writer.uint32(10).fork()).ldelim();
    }
    if (message.monorail !== undefined) {
      MonorailComponent.encode(message.monorail, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BugComponent {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBugComponent() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.issueTracker = IssueTrackerComponent.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.monorail = MonorailComponent.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BugComponent {
    return {
      issueTracker: isSet(object.issueTracker) ? IssueTrackerComponent.fromJSON(object.issueTracker) : undefined,
      monorail: isSet(object.monorail) ? MonorailComponent.fromJSON(object.monorail) : undefined,
    };
  },

  toJSON(message: BugComponent): unknown {
    const obj: any = {};
    if (message.issueTracker !== undefined) {
      obj.issueTracker = IssueTrackerComponent.toJSON(message.issueTracker);
    }
    if (message.monorail !== undefined) {
      obj.monorail = MonorailComponent.toJSON(message.monorail);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BugComponent>, I>>(base?: I): BugComponent {
    return BugComponent.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BugComponent>, I>>(object: I): BugComponent {
    const message = createBaseBugComponent() as any;
    message.issueTracker = (object.issueTracker !== undefined && object.issueTracker !== null)
      ? IssueTrackerComponent.fromPartial(object.issueTracker)
      : undefined;
    message.monorail = (object.monorail !== undefined && object.monorail !== null)
      ? MonorailComponent.fromPartial(object.monorail)
      : undefined;
    return message;
  },
};

function createBaseIssueTrackerComponent(): IssueTrackerComponent {
  return { componentId: "0" };
}

export const IssueTrackerComponent = {
  encode(message: IssueTrackerComponent, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.componentId !== "0") {
      writer.uint32(8).int64(message.componentId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): IssueTrackerComponent {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseIssueTrackerComponent() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.componentId = longToString(reader.int64() as Long);
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): IssueTrackerComponent {
    return { componentId: isSet(object.componentId) ? globalThis.String(object.componentId) : "0" };
  },

  toJSON(message: IssueTrackerComponent): unknown {
    const obj: any = {};
    if (message.componentId !== "0") {
      obj.componentId = message.componentId;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<IssueTrackerComponent>, I>>(base?: I): IssueTrackerComponent {
    return IssueTrackerComponent.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<IssueTrackerComponent>, I>>(object: I): IssueTrackerComponent {
    const message = createBaseIssueTrackerComponent() as any;
    message.componentId = object.componentId ?? "0";
    return message;
  },
};

function createBaseMonorailComponent(): MonorailComponent {
  return { project: "", value: "" };
}

export const MonorailComponent = {
  encode(message: MonorailComponent, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.project !== "") {
      writer.uint32(10).string(message.project);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MonorailComponent {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseMonorailComponent() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.project = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.value = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): MonorailComponent {
    return {
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      value: isSet(object.value) ? globalThis.String(object.value) : "",
    };
  },

  toJSON(message: MonorailComponent): unknown {
    const obj: any = {};
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.value !== "") {
      obj.value = message.value;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<MonorailComponent>, I>>(base?: I): MonorailComponent {
    return MonorailComponent.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<MonorailComponent>, I>>(object: I): MonorailComponent {
    const message = createBaseMonorailComponent() as any;
    message.project = object.project ?? "";
    message.value = object.value ?? "";
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

type KeysOfUnion<T> = T extends T ? keyof T : never;
export type Exact<P, I extends P> = P extends Builtin ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & { [K in Exclude<keyof I, KeysOfUnion<P>>]: never };

function longToString(long: Long) {
  return long.toString();
}

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any;
  _m0.configure();
}

function isObject(value: any): boolean {
  return typeof value === "object" && value !== null;
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
