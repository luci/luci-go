/* eslint-disable */
import _m0 from "protobufjs/minimal";
import { Variant } from "./common.pb";

export const protobufPackage = "luci.resultdb.v1";

/**
 * Represents a function TestResult -> bool.
 * Empty message matches all test results.
 *
 * Most clients would want to set expected_results to
 * VARIANTS_WITH_UNEXPECTED_RESULTS.
 */
export interface TestResultPredicate {
  /**
   * A test result must have a test id matching this regular expression
   * entirely, i.e. the expression is implicitly wrapped with ^ and $.
   */
  readonly testIdRegexp: string;
  /** A test result must have a variant satisfying this predicate. */
  readonly variant:
    | VariantPredicate
    | undefined;
  /**
   * A test result must match this predicate based on TestResult.expected field.
   * Most clients would want to override this field because the default
   * typically causes a large response size.
   */
  readonly expectancy: TestResultPredicate_Expectancy;
  /**
   * If true, filter out exonerated test variants.
   * Mutually exclusive with Expectancy.ALL.
   *
   * If false, the filter is NOT applied.
   * That is, the test result may or may not be exonerated.
   */
  readonly excludeExonerated: boolean;
}

/** Filters test results based on TestResult.expected field. */
export enum TestResultPredicate_Expectancy {
  /**
   * ALL - All test results satisfy this.
   * WARNING: using this significantly increases response size and latency.
   */
  ALL = 0,
  /**
   * VARIANTS_WITH_UNEXPECTED_RESULTS - A test result must belong to a test variant that has one or more
   * unexpected results. It can be used to fetch both unexpected and flakily
   * expected results.
   *
   * Note that the predicate is defined at the test variant level.
   * For example, if a test variant expects a PASS and has results
   * [FAIL, FAIL, PASS], then all results satisfy the predicate because
   * the variant satisfies the predicate.
   */
  VARIANTS_WITH_UNEXPECTED_RESULTS = 1,
  /**
   * VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS - Similar to VARIANTS_WITH_UNEXPECTED_RESULTS, but the test variant
   * must not have any expected results.
   */
  VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS = 2,
}

export function testResultPredicate_ExpectancyFromJSON(object: any): TestResultPredicate_Expectancy {
  switch (object) {
    case 0:
    case "ALL":
      return TestResultPredicate_Expectancy.ALL;
    case 1:
    case "VARIANTS_WITH_UNEXPECTED_RESULTS":
      return TestResultPredicate_Expectancy.VARIANTS_WITH_UNEXPECTED_RESULTS;
    case 2:
    case "VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS":
      return TestResultPredicate_Expectancy.VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum TestResultPredicate_Expectancy");
  }
}

export function testResultPredicate_ExpectancyToJSON(object: TestResultPredicate_Expectancy): string {
  switch (object) {
    case TestResultPredicate_Expectancy.ALL:
      return "ALL";
    case TestResultPredicate_Expectancy.VARIANTS_WITH_UNEXPECTED_RESULTS:
      return "VARIANTS_WITH_UNEXPECTED_RESULTS";
    case TestResultPredicate_Expectancy.VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS:
      return "VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum TestResultPredicate_Expectancy");
  }
}

/**
 * Represents a function TestExoneration -> bool.
 * Empty message matches all test exonerations.
 */
export interface TestExonerationPredicate {
  /**
   * A test exoneration must have a test id matching this regular expression
   * entirely, i.e. the expression is implicitly wrapped with ^ and $.
   */
  readonly testIdRegexp: string;
  /** A test exoneration must have a variant satisfying this predicate. */
  readonly variant: VariantPredicate | undefined;
}

/** Represents a function Variant -> bool. */
export interface VariantPredicate {
  /** A variant must be equal this definition exactly. */
  readonly equals?:
    | Variant
    | undefined;
  /** A variant's key-value pairs must contain those in this one. */
  readonly contains?: Variant | undefined;
}

/** Represents a function Artifact -> bool. */
export interface ArtifactPredicate {
  /**
   * Specifies which edges to follow when retrieving directly/indirectly
   * included artifacts.
   * For example,
   * - to retrieve only invocation-level artifacts, use
   *   {included_invocations: true}.
   * - to retrieve only test-result-level artifacts, use {test_results: true}.
   *
   * By default, follows all edges.
   */
  readonly followEdges:
    | ArtifactPredicate_EdgeTypeSet
    | undefined;
  /**
   * If an Artifact belongs to a TestResult, then the test result must satisfy
   * this predicate.
   * Note: this predicate does NOT apply to invocation-level artifacts.
   * To exclude them from the response, use follow_edges.
   */
  readonly testResultPredicate:
    | TestResultPredicate
    | undefined;
  /**
   * An artifact must have a content type matching this regular expression
   * entirely, i.e. the expression is implicitly wrapped with ^ and $.
   * Defaults to ".*".
   */
  readonly contentTypeRegexp: string;
  /**
   * An artifact must have an ID matching this regular expression entirely, i.e.
   * the expression is implicitly wrapped with ^ and $.  Defaults to ".*".
   */
  readonly artifactIdRegexp: string;
}

/** A set of Invocation's outgoing edge types. */
export interface ArtifactPredicate_EdgeTypeSet {
  /** The edges represented by Invocation.included_invocations field. */
  readonly includedInvocations: boolean;
  /** The parent-child relationship between Invocation and TestResult. */
  readonly testResults: boolean;
}

/**
 * Represents a function TestMetadata -> bool.
 * Empty message matches all test metadata.
 */
export interface TestMetadataPredicate {
  /** A test metadata must have the test id in this list. */
  readonly testIds: readonly string[];
}

function createBaseTestResultPredicate(): TestResultPredicate {
  return { testIdRegexp: "", variant: undefined, expectancy: 0, excludeExonerated: false };
}

export const TestResultPredicate = {
  encode(message: TestResultPredicate, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.testIdRegexp !== "") {
      writer.uint32(10).string(message.testIdRegexp);
    }
    if (message.variant !== undefined) {
      VariantPredicate.encode(message.variant, writer.uint32(18).fork()).ldelim();
    }
    if (message.expectancy !== 0) {
      writer.uint32(24).int32(message.expectancy);
    }
    if (message.excludeExonerated === true) {
      writer.uint32(32).bool(message.excludeExonerated);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestResultPredicate {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestResultPredicate() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.testIdRegexp = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.variant = VariantPredicate.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.expectancy = reader.int32() as any;
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.excludeExonerated = reader.bool();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestResultPredicate {
    return {
      testIdRegexp: isSet(object.testIdRegexp) ? globalThis.String(object.testIdRegexp) : "",
      variant: isSet(object.variant) ? VariantPredicate.fromJSON(object.variant) : undefined,
      expectancy: isSet(object.expectancy) ? testResultPredicate_ExpectancyFromJSON(object.expectancy) : 0,
      excludeExonerated: isSet(object.excludeExonerated) ? globalThis.Boolean(object.excludeExonerated) : false,
    };
  },

  toJSON(message: TestResultPredicate): unknown {
    const obj: any = {};
    if (message.testIdRegexp !== "") {
      obj.testIdRegexp = message.testIdRegexp;
    }
    if (message.variant !== undefined) {
      obj.variant = VariantPredicate.toJSON(message.variant);
    }
    if (message.expectancy !== 0) {
      obj.expectancy = testResultPredicate_ExpectancyToJSON(message.expectancy);
    }
    if (message.excludeExonerated === true) {
      obj.excludeExonerated = message.excludeExonerated;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestResultPredicate>, I>>(base?: I): TestResultPredicate {
    return TestResultPredicate.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestResultPredicate>, I>>(object: I): TestResultPredicate {
    const message = createBaseTestResultPredicate() as any;
    message.testIdRegexp = object.testIdRegexp ?? "";
    message.variant = (object.variant !== undefined && object.variant !== null)
      ? VariantPredicate.fromPartial(object.variant)
      : undefined;
    message.expectancy = object.expectancy ?? 0;
    message.excludeExonerated = object.excludeExonerated ?? false;
    return message;
  },
};

function createBaseTestExonerationPredicate(): TestExonerationPredicate {
  return { testIdRegexp: "", variant: undefined };
}

export const TestExonerationPredicate = {
  encode(message: TestExonerationPredicate, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.testIdRegexp !== "") {
      writer.uint32(10).string(message.testIdRegexp);
    }
    if (message.variant !== undefined) {
      VariantPredicate.encode(message.variant, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestExonerationPredicate {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestExonerationPredicate() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.testIdRegexp = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.variant = VariantPredicate.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestExonerationPredicate {
    return {
      testIdRegexp: isSet(object.testIdRegexp) ? globalThis.String(object.testIdRegexp) : "",
      variant: isSet(object.variant) ? VariantPredicate.fromJSON(object.variant) : undefined,
    };
  },

  toJSON(message: TestExonerationPredicate): unknown {
    const obj: any = {};
    if (message.testIdRegexp !== "") {
      obj.testIdRegexp = message.testIdRegexp;
    }
    if (message.variant !== undefined) {
      obj.variant = VariantPredicate.toJSON(message.variant);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestExonerationPredicate>, I>>(base?: I): TestExonerationPredicate {
    return TestExonerationPredicate.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestExonerationPredicate>, I>>(object: I): TestExonerationPredicate {
    const message = createBaseTestExonerationPredicate() as any;
    message.testIdRegexp = object.testIdRegexp ?? "";
    message.variant = (object.variant !== undefined && object.variant !== null)
      ? VariantPredicate.fromPartial(object.variant)
      : undefined;
    return message;
  },
};

function createBaseVariantPredicate(): VariantPredicate {
  return { equals: undefined, contains: undefined };
}

export const VariantPredicate = {
  encode(message: VariantPredicate, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.equals !== undefined) {
      Variant.encode(message.equals, writer.uint32(10).fork()).ldelim();
    }
    if (message.contains !== undefined) {
      Variant.encode(message.contains, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): VariantPredicate {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseVariantPredicate() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.equals = Variant.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.contains = Variant.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): VariantPredicate {
    return {
      equals: isSet(object.equals) ? Variant.fromJSON(object.equals) : undefined,
      contains: isSet(object.contains) ? Variant.fromJSON(object.contains) : undefined,
    };
  },

  toJSON(message: VariantPredicate): unknown {
    const obj: any = {};
    if (message.equals !== undefined) {
      obj.equals = Variant.toJSON(message.equals);
    }
    if (message.contains !== undefined) {
      obj.contains = Variant.toJSON(message.contains);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<VariantPredicate>, I>>(base?: I): VariantPredicate {
    return VariantPredicate.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<VariantPredicate>, I>>(object: I): VariantPredicate {
    const message = createBaseVariantPredicate() as any;
    message.equals = (object.equals !== undefined && object.equals !== null)
      ? Variant.fromPartial(object.equals)
      : undefined;
    message.contains = (object.contains !== undefined && object.contains !== null)
      ? Variant.fromPartial(object.contains)
      : undefined;
    return message;
  },
};

function createBaseArtifactPredicate(): ArtifactPredicate {
  return { followEdges: undefined, testResultPredicate: undefined, contentTypeRegexp: "", artifactIdRegexp: "" };
}

export const ArtifactPredicate = {
  encode(message: ArtifactPredicate, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.followEdges !== undefined) {
      ArtifactPredicate_EdgeTypeSet.encode(message.followEdges, writer.uint32(10).fork()).ldelim();
    }
    if (message.testResultPredicate !== undefined) {
      TestResultPredicate.encode(message.testResultPredicate, writer.uint32(18).fork()).ldelim();
    }
    if (message.contentTypeRegexp !== "") {
      writer.uint32(26).string(message.contentTypeRegexp);
    }
    if (message.artifactIdRegexp !== "") {
      writer.uint32(34).string(message.artifactIdRegexp);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ArtifactPredicate {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseArtifactPredicate() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.followEdges = ArtifactPredicate_EdgeTypeSet.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.testResultPredicate = TestResultPredicate.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.contentTypeRegexp = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.artifactIdRegexp = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ArtifactPredicate {
    return {
      followEdges: isSet(object.followEdges) ? ArtifactPredicate_EdgeTypeSet.fromJSON(object.followEdges) : undefined,
      testResultPredicate: isSet(object.testResultPredicate)
        ? TestResultPredicate.fromJSON(object.testResultPredicate)
        : undefined,
      contentTypeRegexp: isSet(object.contentTypeRegexp) ? globalThis.String(object.contentTypeRegexp) : "",
      artifactIdRegexp: isSet(object.artifactIdRegexp) ? globalThis.String(object.artifactIdRegexp) : "",
    };
  },

  toJSON(message: ArtifactPredicate): unknown {
    const obj: any = {};
    if (message.followEdges !== undefined) {
      obj.followEdges = ArtifactPredicate_EdgeTypeSet.toJSON(message.followEdges);
    }
    if (message.testResultPredicate !== undefined) {
      obj.testResultPredicate = TestResultPredicate.toJSON(message.testResultPredicate);
    }
    if (message.contentTypeRegexp !== "") {
      obj.contentTypeRegexp = message.contentTypeRegexp;
    }
    if (message.artifactIdRegexp !== "") {
      obj.artifactIdRegexp = message.artifactIdRegexp;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ArtifactPredicate>, I>>(base?: I): ArtifactPredicate {
    return ArtifactPredicate.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ArtifactPredicate>, I>>(object: I): ArtifactPredicate {
    const message = createBaseArtifactPredicate() as any;
    message.followEdges = (object.followEdges !== undefined && object.followEdges !== null)
      ? ArtifactPredicate_EdgeTypeSet.fromPartial(object.followEdges)
      : undefined;
    message.testResultPredicate = (object.testResultPredicate !== undefined && object.testResultPredicate !== null)
      ? TestResultPredicate.fromPartial(object.testResultPredicate)
      : undefined;
    message.contentTypeRegexp = object.contentTypeRegexp ?? "";
    message.artifactIdRegexp = object.artifactIdRegexp ?? "";
    return message;
  },
};

function createBaseArtifactPredicate_EdgeTypeSet(): ArtifactPredicate_EdgeTypeSet {
  return { includedInvocations: false, testResults: false };
}

export const ArtifactPredicate_EdgeTypeSet = {
  encode(message: ArtifactPredicate_EdgeTypeSet, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.includedInvocations === true) {
      writer.uint32(8).bool(message.includedInvocations);
    }
    if (message.testResults === true) {
      writer.uint32(16).bool(message.testResults);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ArtifactPredicate_EdgeTypeSet {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseArtifactPredicate_EdgeTypeSet() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.includedInvocations = reader.bool();
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.testResults = reader.bool();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ArtifactPredicate_EdgeTypeSet {
    return {
      includedInvocations: isSet(object.includedInvocations) ? globalThis.Boolean(object.includedInvocations) : false,
      testResults: isSet(object.testResults) ? globalThis.Boolean(object.testResults) : false,
    };
  },

  toJSON(message: ArtifactPredicate_EdgeTypeSet): unknown {
    const obj: any = {};
    if (message.includedInvocations === true) {
      obj.includedInvocations = message.includedInvocations;
    }
    if (message.testResults === true) {
      obj.testResults = message.testResults;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ArtifactPredicate_EdgeTypeSet>, I>>(base?: I): ArtifactPredicate_EdgeTypeSet {
    return ArtifactPredicate_EdgeTypeSet.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ArtifactPredicate_EdgeTypeSet>, I>>(
    object: I,
  ): ArtifactPredicate_EdgeTypeSet {
    const message = createBaseArtifactPredicate_EdgeTypeSet() as any;
    message.includedInvocations = object.includedInvocations ?? false;
    message.testResults = object.testResults ?? false;
    return message;
  },
};

function createBaseTestMetadataPredicate(): TestMetadataPredicate {
  return { testIds: [] };
}

export const TestMetadataPredicate = {
  encode(message: TestMetadataPredicate, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.testIds) {
      writer.uint32(10).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestMetadataPredicate {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestMetadataPredicate() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.testIds.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestMetadataPredicate {
    return {
      testIds: globalThis.Array.isArray(object?.testIds) ? object.testIds.map((e: any) => globalThis.String(e)) : [],
    };
  },

  toJSON(message: TestMetadataPredicate): unknown {
    const obj: any = {};
    if (message.testIds?.length) {
      obj.testIds = message.testIds;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestMetadataPredicate>, I>>(base?: I): TestMetadataPredicate {
    return TestMetadataPredicate.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestMetadataPredicate>, I>>(object: I): TestMetadataPredicate {
    const message = createBaseTestMetadataPredicate() as any;
    message.testIds = object.testIds?.map((e) => e) || [];
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

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}