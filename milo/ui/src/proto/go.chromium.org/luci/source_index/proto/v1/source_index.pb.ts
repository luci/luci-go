// Code generated by protoc-gen-ts_proto. DO NOT EDIT.
// versions:
//   protoc-gen-ts_proto  v2.7.5
//   protoc               v6.31.1
// source: go.chromium.org/luci/source_index/proto/v1/source_index.proto

/* eslint-disable */
import { BinaryReader, BinaryWriter } from "@bufbuild/protobuf/wire";

export const protobufPackage = "luci.source_index.v1";

export interface QueryCommitHashRequest {
  /**
   * Required. The gitiles host. Must be a subdomain of `.googlesource.com`
   * (e.g. chromium.googlesource.com).
   */
  readonly host: string;
  /** Required. The Git project to query the commit in (e.g. chromium/src). */
  readonly repository: string;
  /**
   * Required. The name of position defined in value of git-footer git-svn-id
   * or Cr-Commit-Position (e.g. refs/heads/master,
   * svn://svn.chromium.org/chrome/trunk/src)
   */
  readonly positionRef: string;
  /**
   * Required. The sequential identifier of the commit in the given branch
   * (position_ref).
   */
  readonly positionNumber: string;
}

export interface QueryCommitHashResponse {
  /** The full git commit hash of the matched commit. */
  readonly hash: string;
}

function createBaseQueryCommitHashRequest(): QueryCommitHashRequest {
  return { host: "", repository: "", positionRef: "", positionNumber: "0" };
}

export const QueryCommitHashRequest: MessageFns<QueryCommitHashRequest> = {
  encode(message: QueryCommitHashRequest, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.host !== "") {
      writer.uint32(10).string(message.host);
    }
    if (message.repository !== "") {
      writer.uint32(18).string(message.repository);
    }
    if (message.positionRef !== "") {
      writer.uint32(26).string(message.positionRef);
    }
    if (message.positionNumber !== "0") {
      writer.uint32(32).int64(message.positionNumber);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): QueryCommitHashRequest {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryCommitHashRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.host = reader.string();
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.repository = reader.string();
          continue;
        }
        case 3: {
          if (tag !== 26) {
            break;
          }

          message.positionRef = reader.string();
          continue;
        }
        case 4: {
          if (tag !== 32) {
            break;
          }

          message.positionNumber = reader.int64().toString();
          continue;
        }
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skip(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): QueryCommitHashRequest {
    return {
      host: isSet(object.host) ? globalThis.String(object.host) : "",
      repository: isSet(object.repository) ? globalThis.String(object.repository) : "",
      positionRef: isSet(object.positionRef) ? globalThis.String(object.positionRef) : "",
      positionNumber: isSet(object.positionNumber) ? globalThis.String(object.positionNumber) : "0",
    };
  },

  toJSON(message: QueryCommitHashRequest): unknown {
    const obj: any = {};
    if (message.host !== "") {
      obj.host = message.host;
    }
    if (message.repository !== "") {
      obj.repository = message.repository;
    }
    if (message.positionRef !== "") {
      obj.positionRef = message.positionRef;
    }
    if (message.positionNumber !== "0") {
      obj.positionNumber = message.positionNumber;
    }
    return obj;
  },

  create(base?: DeepPartial<QueryCommitHashRequest>): QueryCommitHashRequest {
    return QueryCommitHashRequest.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<QueryCommitHashRequest>): QueryCommitHashRequest {
    const message = createBaseQueryCommitHashRequest() as any;
    message.host = object.host ?? "";
    message.repository = object.repository ?? "";
    message.positionRef = object.positionRef ?? "";
    message.positionNumber = object.positionNumber ?? "0";
    return message;
  },
};

function createBaseQueryCommitHashResponse(): QueryCommitHashResponse {
  return { hash: "" };
}

export const QueryCommitHashResponse: MessageFns<QueryCommitHashResponse> = {
  encode(message: QueryCommitHashResponse, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.hash !== "") {
      writer.uint32(10).string(message.hash);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): QueryCommitHashResponse {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryCommitHashResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.hash = reader.string();
          continue;
        }
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skip(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): QueryCommitHashResponse {
    return { hash: isSet(object.hash) ? globalThis.String(object.hash) : "" };
  },

  toJSON(message: QueryCommitHashResponse): unknown {
    const obj: any = {};
    if (message.hash !== "") {
      obj.hash = message.hash;
    }
    return obj;
  },

  create(base?: DeepPartial<QueryCommitHashResponse>): QueryCommitHashResponse {
    return QueryCommitHashResponse.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<QueryCommitHashResponse>): QueryCommitHashResponse {
    const message = createBaseQueryCommitHashResponse() as any;
    message.hash = object.hash ?? "";
    return message;
  },
};

/**
 * Provides methods to retrieve metadata of git commits.
 *
 * Use of LUCI is subject to the Google [Terms of Service](https://policies.google.com/terms)
 * and [Privacy Policy](https://policies.google.com/privacy).
 */
export interface SourceIndex {
  /**
   * QueryCommitHash returns commit that matches desired position of commit,
   * based on QueryCommitHashRequest parameters. Commit position is based on
   * git-footer git-svn-id or Cr-Commit-Position.
   *
   * Returns `NOT_FOUND` if the commit is not indexed by source-index.
   * When there are multiple matches (i.e. the same commit position occurs on
   * different commits somehow), the first match (determined arbitrarily) will
   * be returned.
   */
  QueryCommitHash(request: QueryCommitHashRequest): Promise<QueryCommitHashResponse>;
}

export const SourceIndexServiceName = "luci.source_index.v1.SourceIndex";
export class SourceIndexClientImpl implements SourceIndex {
  static readonly DEFAULT_SERVICE = SourceIndexServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || SourceIndexServiceName;
    this.rpc = rpc;
    this.QueryCommitHash = this.QueryCommitHash.bind(this);
  }
  QueryCommitHash(request: QueryCommitHashRequest): Promise<QueryCommitHashResponse> {
    const data = QueryCommitHashRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "QueryCommitHash", data);
    return promise.then((data) => QueryCommitHashResponse.fromJSON(data));
  }
}

interface Rpc {
  request(service: string, method: string, data: unknown): Promise<unknown>;
}

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}

export interface MessageFns<T> {
  encode(message: T, writer?: BinaryWriter): BinaryWriter;
  decode(input: BinaryReader | Uint8Array, length?: number): T;
  fromJSON(object: any): T;
  toJSON(message: T): unknown;
  create(base?: DeepPartial<T>): T;
  fromPartial(object: DeepPartial<T>): T;
}
