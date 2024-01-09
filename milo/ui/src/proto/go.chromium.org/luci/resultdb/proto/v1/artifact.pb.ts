/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";
import { Timestamp } from "../../../../../google/protobuf/timestamp.pb";

export const protobufPackage = "luci.resultdb.v1";

/**
 * A file produced during a build/test, typically a test artifact.
 * The parent resource is either a TestResult or an Invocation.
 *
 * An invocation-level artifact might be related to tests, or it might not, for
 * example it may be used to store build step logs when streaming support is
 * added.
 * Next id: 9.
 */
export interface Artifact {
  /**
   * Can be used to refer to this artifact.
   * Format:
   * - For invocation-level artifacts:
   *   "invocations/{INVOCATION_ID}/artifacts/{ARTIFACT_ID}".
   * - For test-result-level artifacts:
   *   "invocations/{INVOCATION_ID}/tests/{URL_ESCAPED_TEST_ID}/results/{RESULT_ID}/artifacts/{ARTIFACT_ID}".
   * where URL_ESCAPED_TEST_ID is the test_id escaped with
   * https://golang.org/pkg/net/url/#PathEscape (see also https://aip.dev/122),
   * and ARTIFACT_ID is documented below.
   * Examples: "screenshot.png", "traces/a.txt".
   */
  readonly name: string;
  /**
   * A local identifier of the artifact, unique within the parent resource.
   * MAY have slashes, but MUST NOT start with a slash.
   * SHOULD not use backslashes.
   * Regex: ^[[:word:]]([[:print:]]{0,254}[[:word:]])?$
   */
  readonly artifactId: string;
  /**
   * A signed short-lived URL to fetch the contents of the artifact.
   * See also fetch_url_expiration.
   */
  readonly fetchUrl: string;
  /** When fetch_url expires. If expired, re-request this Artifact. */
  readonly fetchUrlExpiration:
    | string
    | undefined;
  /**
   * Media type of the artifact.
   * Logs are typically "text/plain" and screenshots are typically "image/png".
   * Optional.
   */
  readonly contentType: string;
  /**
   * Size of the file.
   * Can be used in UI to decide between displaying the artifact inline or only
   * showing a link if it is too large.
   * If you are using the gcs_uri, this field is not verified, but only treated as a hint.
   */
  readonly sizeBytes: string;
  /**
   * Contents of the artifact.
   * This is INPUT_ONLY, and taken by BatchCreateArtifacts().
   * All getter RPCs, such as ListArtifacts(), do not populate values into
   * the field in the response.
   * If specified, `gcs_uri` must be empty.
   */
  readonly contents: Uint8Array;
  /** The GCS URI of the artifact if it's stored in GCS.  If specified, `contents` must be empty. */
  readonly gcsUri: string;
}

function createBaseArtifact(): Artifact {
  return {
    name: "",
    artifactId: "",
    fetchUrl: "",
    fetchUrlExpiration: undefined,
    contentType: "",
    sizeBytes: "0",
    contents: new Uint8Array(0),
    gcsUri: "",
  };
}

export const Artifact = {
  encode(message: Artifact, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.artifactId !== "") {
      writer.uint32(18).string(message.artifactId);
    }
    if (message.fetchUrl !== "") {
      writer.uint32(26).string(message.fetchUrl);
    }
    if (message.fetchUrlExpiration !== undefined) {
      Timestamp.encode(toTimestamp(message.fetchUrlExpiration), writer.uint32(34).fork()).ldelim();
    }
    if (message.contentType !== "") {
      writer.uint32(42).string(message.contentType);
    }
    if (message.sizeBytes !== "0") {
      writer.uint32(48).int64(message.sizeBytes);
    }
    if (message.contents.length !== 0) {
      writer.uint32(58).bytes(message.contents);
    }
    if (message.gcsUri !== "") {
      writer.uint32(66).string(message.gcsUri);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Artifact {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseArtifact() as any;
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

          message.artifactId = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.fetchUrl = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.fetchUrlExpiration = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.contentType = reader.string();
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.sizeBytes = longToString(reader.int64() as Long);
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.contents = reader.bytes();
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.gcsUri = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Artifact {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      artifactId: isSet(object.artifactId) ? globalThis.String(object.artifactId) : "",
      fetchUrl: isSet(object.fetchUrl) ? globalThis.String(object.fetchUrl) : "",
      fetchUrlExpiration: isSet(object.fetchUrlExpiration) ? globalThis.String(object.fetchUrlExpiration) : undefined,
      contentType: isSet(object.contentType) ? globalThis.String(object.contentType) : "",
      sizeBytes: isSet(object.sizeBytes) ? globalThis.String(object.sizeBytes) : "0",
      contents: isSet(object.contents) ? bytesFromBase64(object.contents) : new Uint8Array(0),
      gcsUri: isSet(object.gcsUri) ? globalThis.String(object.gcsUri) : "",
    };
  },

  toJSON(message: Artifact): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.artifactId !== "") {
      obj.artifactId = message.artifactId;
    }
    if (message.fetchUrl !== "") {
      obj.fetchUrl = message.fetchUrl;
    }
    if (message.fetchUrlExpiration !== undefined) {
      obj.fetchUrlExpiration = message.fetchUrlExpiration;
    }
    if (message.contentType !== "") {
      obj.contentType = message.contentType;
    }
    if (message.sizeBytes !== "0") {
      obj.sizeBytes = message.sizeBytes;
    }
    if (message.contents.length !== 0) {
      obj.contents = base64FromBytes(message.contents);
    }
    if (message.gcsUri !== "") {
      obj.gcsUri = message.gcsUri;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Artifact>, I>>(base?: I): Artifact {
    return Artifact.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Artifact>, I>>(object: I): Artifact {
    const message = createBaseArtifact() as any;
    message.name = object.name ?? "";
    message.artifactId = object.artifactId ?? "";
    message.fetchUrl = object.fetchUrl ?? "";
    message.fetchUrlExpiration = object.fetchUrlExpiration ?? undefined;
    message.contentType = object.contentType ?? "";
    message.sizeBytes = object.sizeBytes ?? "0";
    message.contents = object.contents ?? new Uint8Array(0);
    message.gcsUri = object.gcsUri ?? "";
    return message;
  },
};

function bytesFromBase64(b64: string): Uint8Array {
  if (globalThis.Buffer) {
    return Uint8Array.from(globalThis.Buffer.from(b64, "base64"));
  } else {
    const bin = globalThis.atob(b64);
    const arr = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i);
    }
    return arr;
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if (globalThis.Buffer) {
    return globalThis.Buffer.from(arr).toString("base64");
  } else {
    const bin: string[] = [];
    arr.forEach((byte) => {
      bin.push(globalThis.String.fromCharCode(byte));
    });
    return globalThis.btoa(bin.join(""));
  }
}

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

type KeysOfUnion<T> = T extends T ? keyof T : never;
export type Exact<P, I extends P> = P extends Builtin ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & { [K in Exclude<keyof I, KeysOfUnion<P>>]: never };

function toTimestamp(dateStr: string): Timestamp {
  const date = new globalThis.Date(dateStr);
  const seconds = Math.trunc(date.getTime() / 1_000).toString();
  const nanos = (date.getTime() % 1_000) * 1_000_000;
  return { seconds, nanos };
}

function fromTimestamp(t: Timestamp): string {
  let millis = (globalThis.Number(t.seconds) || 0) * 1_000;
  millis += (t.nanos || 0) / 1_000_000;
  return new globalThis.Date(millis).toISOString();
}

function longToString(long: Long) {
  return long.toString();
}

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any;
  _m0.configure();
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
