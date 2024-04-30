/* eslint-disable */
import _m0 from "protobufjs/minimal";
import { Empty } from "../../../../../google/protobuf/empty.pb";
import { FieldMask } from "../../../../../google/protobuf/field_mask.pb";
import { Timestamp } from "../../../../../google/protobuf/timestamp.pb";

export const protobufPackage = "auth.service";

/** PrincipalKind denotes the type of principal of a specific entity. */
export enum PrincipalKind {
  UNSPECIFIED = 0,
  /** IDENTITY - A single individual identity, e.g. "user:someone@example.com". */
  IDENTITY = 1,
  /** GROUP - A group name, e.g. "some-group". */
  GROUP = 2,
  /** GLOB - An identity glob, e.g. "user*@example.com". */
  GLOB = 3,
}

export function principalKindFromJSON(object: any): PrincipalKind {
  switch (object) {
    case 0:
    case "PRINCIPAL_KIND_UNSPECIFIED":
      return PrincipalKind.UNSPECIFIED;
    case 1:
    case "IDENTITY":
      return PrincipalKind.IDENTITY;
    case 2:
    case "GROUP":
      return PrincipalKind.GROUP;
    case 3:
    case "GLOB":
      return PrincipalKind.GLOB;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum PrincipalKind");
  }
}

export function principalKindToJSON(object: PrincipalKind): string {
  switch (object) {
    case PrincipalKind.UNSPECIFIED:
      return "PRINCIPAL_KIND_UNSPECIFIED";
    case PrincipalKind.IDENTITY:
      return "IDENTITY";
    case PrincipalKind.GROUP:
      return "GROUP";
    case PrincipalKind.GLOB:
      return "GLOB";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum PrincipalKind");
  }
}

/** ListGroupsResponse is all the groups listed in LUCI Auth Service. */
export interface ListGroupsResponse {
  /**
   * List of all groups. In order to keep the response lightweight, each
   * AuthGroup will contain only metadata, i.e. the membership list fields will
   * be left empty.
   */
  readonly groups: readonly AuthGroup[];
}

/** GetGroupRequest is to specify an individual group. */
export interface GetGroupRequest {
  /** e.g: "administrators" */
  readonly name: string;
}

/** CreateGroupRequest requests the creation of a new group. */
export interface CreateGroupRequest {
  /**
   * Details of the group to create. Not all fields will be written to the new
   * group: if the request specifies fields that should be automatically
   * generated (e.g. created/modified timestamps), these will be ignored.
   */
  readonly group: AuthGroup | undefined;
}

/** UpdateGroupRequest requests an update to an existing group. */
export interface UpdateGroupRequest {
  /**
   * Details of the group to update. The group's 'name' field is used to
   * identify the group to update.
   */
  readonly group:
    | AuthGroup
    | undefined;
  /** The list of fields to be updated. */
  readonly updateMask: readonly string[] | undefined;
}

/** DeleteGroupRequest requests the deletion of a group. */
export interface DeleteGroupRequest {
  /** Name of the group to delete. */
  readonly name: string;
  /**
   * The current etag of the group.
   * If an etag is provided and does not match the current etag of the group,
   * deletion will be blocked and an ABORTED error will be returned.
   */
  readonly etag: string;
}

/** AuthGroup defines an individual group. */
export interface AuthGroup {
  /** e.g: "auth-service-access" */
  readonly name: string;
  /** e.g: ["user:t@example.com"] */
  readonly members: readonly string[];
  /** e.g: ["user:*@example.com" */
  readonly globs: readonly string[];
  /** e.g: ["another-group-0", "another-group-1"] */
  readonly nested: readonly string[];
  /** e.g: "This group is used for ..." */
  readonly description: string;
  /** e.g: "administrators" */
  readonly owners: string;
  /** e.g: "1972-01-01T10:00:20.021Z" */
  readonly createdTs:
    | string
    | undefined;
  /** e.g: "user:test@example.com" */
  readonly createdBy: string;
  /** Output only. Whether the caller can modify this group. */
  readonly callerCanModify: boolean;
  /**
   * An opaque string that indicates the version of the group being edited.
   * This will be sent to the client in responses, and should be sent back
   * to the server for update and delete requests in order to protect against
   * concurrent modification errors. See https://google.aip.dev/154.
   * Technically this is a "weak etag", meaning that if two AuthGroups have the
   * same etag, they are not guaranteed to be byte-for-byte identical. This is
   * because under the hood we generate it based on the last-modified time
   * (though this should not be relied on as it may change in future).
   */
  readonly etag: string;
}

/**
 * GetSubgraphRequest contains the Principal that is the basis of the search
 * for inclusion and is the root of the output subgraph.
 */
export interface GetSubgraphRequest {
  readonly principal: Principal | undefined;
}

/**
 * The Subgraph returned by GetSubgraph RPC.
 *
 * The node representing a principal passed to GetSubgraph is always the
 * first in the list.
 */
export interface Subgraph {
  readonly nodes: readonly Node[];
}

/**
 * Principal is an entity that can be found in the Subgraph. A Principal can
 * represent a group, identity, or glob. See PrincipalKind for clarification on
 * how each Principal kind is represented.
 */
export interface Principal {
  /** e.g. IDENTITY, GROUP, GLOB */
  readonly kind: PrincipalKind;
  /** e.g. "user*@example.com", "some-group", "user:m0@example.com" */
  readonly name: string;
}

/**
 * Each Node is a representation of a Principal.
 * Each subgraph will only contain one Node per principal; in other words,
 * a principal uniquely identifies a Node.
 */
export interface Node {
  /** The principal represented by this node. */
  readonly principal:
    | Principal
    | undefined;
  /**
   * Nodes that directly include this principal.
   *
   * Each item is an index of a Node in Subgraph's `nodes` list.
   */
  readonly includedBy: readonly number[];
}

function createBaseListGroupsResponse(): ListGroupsResponse {
  return { groups: [] };
}

export const ListGroupsResponse = {
  encode(message: ListGroupsResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.groups) {
      AuthGroup.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListGroupsResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListGroupsResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.groups.push(AuthGroup.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ListGroupsResponse {
    return {
      groups: globalThis.Array.isArray(object?.groups) ? object.groups.map((e: any) => AuthGroup.fromJSON(e)) : [],
    };
  },

  toJSON(message: ListGroupsResponse): unknown {
    const obj: any = {};
    if (message.groups?.length) {
      obj.groups = message.groups.map((e) => AuthGroup.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListGroupsResponse>, I>>(base?: I): ListGroupsResponse {
    return ListGroupsResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListGroupsResponse>, I>>(object: I): ListGroupsResponse {
    const message = createBaseListGroupsResponse() as any;
    message.groups = object.groups?.map((e) => AuthGroup.fromPartial(e)) || [];
    return message;
  },
};

function createBaseGetGroupRequest(): GetGroupRequest {
  return { name: "" };
}

export const GetGroupRequest = {
  encode(message: GetGroupRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetGroupRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetGroupRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): GetGroupRequest {
    return { name: isSet(object.name) ? globalThis.String(object.name) : "" };
  },

  toJSON(message: GetGroupRequest): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<GetGroupRequest>, I>>(base?: I): GetGroupRequest {
    return GetGroupRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<GetGroupRequest>, I>>(object: I): GetGroupRequest {
    const message = createBaseGetGroupRequest() as any;
    message.name = object.name ?? "";
    return message;
  },
};

function createBaseCreateGroupRequest(): CreateGroupRequest {
  return { group: undefined };
}

export const CreateGroupRequest = {
  encode(message: CreateGroupRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.group !== undefined) {
      AuthGroup.encode(message.group, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): CreateGroupRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseCreateGroupRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.group = AuthGroup.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): CreateGroupRequest {
    return { group: isSet(object.group) ? AuthGroup.fromJSON(object.group) : undefined };
  },

  toJSON(message: CreateGroupRequest): unknown {
    const obj: any = {};
    if (message.group !== undefined) {
      obj.group = AuthGroup.toJSON(message.group);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<CreateGroupRequest>, I>>(base?: I): CreateGroupRequest {
    return CreateGroupRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<CreateGroupRequest>, I>>(object: I): CreateGroupRequest {
    const message = createBaseCreateGroupRequest() as any;
    message.group = (object.group !== undefined && object.group !== null)
      ? AuthGroup.fromPartial(object.group)
      : undefined;
    return message;
  },
};

function createBaseUpdateGroupRequest(): UpdateGroupRequest {
  return { group: undefined, updateMask: undefined };
}

export const UpdateGroupRequest = {
  encode(message: UpdateGroupRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.group !== undefined) {
      AuthGroup.encode(message.group, writer.uint32(10).fork()).ldelim();
    }
    if (message.updateMask !== undefined) {
      FieldMask.encode(FieldMask.wrap(message.updateMask), writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): UpdateGroupRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseUpdateGroupRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.group = AuthGroup.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.updateMask = FieldMask.unwrap(FieldMask.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): UpdateGroupRequest {
    return {
      group: isSet(object.group) ? AuthGroup.fromJSON(object.group) : undefined,
      updateMask: isSet(object.updateMask) ? FieldMask.unwrap(FieldMask.fromJSON(object.updateMask)) : undefined,
    };
  },

  toJSON(message: UpdateGroupRequest): unknown {
    const obj: any = {};
    if (message.group !== undefined) {
      obj.group = AuthGroup.toJSON(message.group);
    }
    if (message.updateMask !== undefined) {
      obj.updateMask = FieldMask.toJSON(FieldMask.wrap(message.updateMask));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<UpdateGroupRequest>, I>>(base?: I): UpdateGroupRequest {
    return UpdateGroupRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<UpdateGroupRequest>, I>>(object: I): UpdateGroupRequest {
    const message = createBaseUpdateGroupRequest() as any;
    message.group = (object.group !== undefined && object.group !== null)
      ? AuthGroup.fromPartial(object.group)
      : undefined;
    message.updateMask = object.updateMask ?? undefined;
    return message;
  },
};

function createBaseDeleteGroupRequest(): DeleteGroupRequest {
  return { name: "", etag: "" };
}

export const DeleteGroupRequest = {
  encode(message: DeleteGroupRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.etag !== "") {
      writer.uint32(18).string(message.etag);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DeleteGroupRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseDeleteGroupRequest() as any;
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

          message.etag = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): DeleteGroupRequest {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      etag: isSet(object.etag) ? globalThis.String(object.etag) : "",
    };
  },

  toJSON(message: DeleteGroupRequest): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.etag !== "") {
      obj.etag = message.etag;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<DeleteGroupRequest>, I>>(base?: I): DeleteGroupRequest {
    return DeleteGroupRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<DeleteGroupRequest>, I>>(object: I): DeleteGroupRequest {
    const message = createBaseDeleteGroupRequest() as any;
    message.name = object.name ?? "";
    message.etag = object.etag ?? "";
    return message;
  },
};

function createBaseAuthGroup(): AuthGroup {
  return {
    name: "",
    members: [],
    globs: [],
    nested: [],
    description: "",
    owners: "",
    createdTs: undefined,
    createdBy: "",
    callerCanModify: false,
    etag: "",
  };
}

export const AuthGroup = {
  encode(message: AuthGroup, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    for (const v of message.members) {
      writer.uint32(18).string(v!);
    }
    for (const v of message.globs) {
      writer.uint32(26).string(v!);
    }
    for (const v of message.nested) {
      writer.uint32(34).string(v!);
    }
    if (message.description !== "") {
      writer.uint32(42).string(message.description);
    }
    if (message.owners !== "") {
      writer.uint32(50).string(message.owners);
    }
    if (message.createdTs !== undefined) {
      Timestamp.encode(toTimestamp(message.createdTs), writer.uint32(58).fork()).ldelim();
    }
    if (message.createdBy !== "") {
      writer.uint32(66).string(message.createdBy);
    }
    if (message.callerCanModify === true) {
      writer.uint32(72).bool(message.callerCanModify);
    }
    if (message.etag !== "") {
      writer.uint32(794).string(message.etag);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): AuthGroup {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseAuthGroup() as any;
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

          message.members.push(reader.string());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.globs.push(reader.string());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.nested.push(reader.string());
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.description = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.owners = reader.string();
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.createdTs = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.createdBy = reader.string();
          continue;
        case 9:
          if (tag !== 72) {
            break;
          }

          message.callerCanModify = reader.bool();
          continue;
        case 99:
          if (tag !== 794) {
            break;
          }

          message.etag = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): AuthGroup {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      members: globalThis.Array.isArray(object?.members) ? object.members.map((e: any) => globalThis.String(e)) : [],
      globs: globalThis.Array.isArray(object?.globs) ? object.globs.map((e: any) => globalThis.String(e)) : [],
      nested: globalThis.Array.isArray(object?.nested) ? object.nested.map((e: any) => globalThis.String(e)) : [],
      description: isSet(object.description) ? globalThis.String(object.description) : "",
      owners: isSet(object.owners) ? globalThis.String(object.owners) : "",
      createdTs: isSet(object.createdTs) ? globalThis.String(object.createdTs) : undefined,
      createdBy: isSet(object.createdBy) ? globalThis.String(object.createdBy) : "",
      callerCanModify: isSet(object.callerCanModify) ? globalThis.Boolean(object.callerCanModify) : false,
      etag: isSet(object.etag) ? globalThis.String(object.etag) : "",
    };
  },

  toJSON(message: AuthGroup): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.members?.length) {
      obj.members = message.members;
    }
    if (message.globs?.length) {
      obj.globs = message.globs;
    }
    if (message.nested?.length) {
      obj.nested = message.nested;
    }
    if (message.description !== "") {
      obj.description = message.description;
    }
    if (message.owners !== "") {
      obj.owners = message.owners;
    }
    if (message.createdTs !== undefined) {
      obj.createdTs = message.createdTs;
    }
    if (message.createdBy !== "") {
      obj.createdBy = message.createdBy;
    }
    if (message.callerCanModify === true) {
      obj.callerCanModify = message.callerCanModify;
    }
    if (message.etag !== "") {
      obj.etag = message.etag;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<AuthGroup>, I>>(base?: I): AuthGroup {
    return AuthGroup.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<AuthGroup>, I>>(object: I): AuthGroup {
    const message = createBaseAuthGroup() as any;
    message.name = object.name ?? "";
    message.members = object.members?.map((e) => e) || [];
    message.globs = object.globs?.map((e) => e) || [];
    message.nested = object.nested?.map((e) => e) || [];
    message.description = object.description ?? "";
    message.owners = object.owners ?? "";
    message.createdTs = object.createdTs ?? undefined;
    message.createdBy = object.createdBy ?? "";
    message.callerCanModify = object.callerCanModify ?? false;
    message.etag = object.etag ?? "";
    return message;
  },
};

function createBaseGetSubgraphRequest(): GetSubgraphRequest {
  return { principal: undefined };
}

export const GetSubgraphRequest = {
  encode(message: GetSubgraphRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.principal !== undefined) {
      Principal.encode(message.principal, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetSubgraphRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetSubgraphRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.principal = Principal.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): GetSubgraphRequest {
    return { principal: isSet(object.principal) ? Principal.fromJSON(object.principal) : undefined };
  },

  toJSON(message: GetSubgraphRequest): unknown {
    const obj: any = {};
    if (message.principal !== undefined) {
      obj.principal = Principal.toJSON(message.principal);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<GetSubgraphRequest>, I>>(base?: I): GetSubgraphRequest {
    return GetSubgraphRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<GetSubgraphRequest>, I>>(object: I): GetSubgraphRequest {
    const message = createBaseGetSubgraphRequest() as any;
    message.principal = (object.principal !== undefined && object.principal !== null)
      ? Principal.fromPartial(object.principal)
      : undefined;
    return message;
  },
};

function createBaseSubgraph(): Subgraph {
  return { nodes: [] };
}

export const Subgraph = {
  encode(message: Subgraph, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.nodes) {
      Node.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Subgraph {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseSubgraph() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.nodes.push(Node.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Subgraph {
    return { nodes: globalThis.Array.isArray(object?.nodes) ? object.nodes.map((e: any) => Node.fromJSON(e)) : [] };
  },

  toJSON(message: Subgraph): unknown {
    const obj: any = {};
    if (message.nodes?.length) {
      obj.nodes = message.nodes.map((e) => Node.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Subgraph>, I>>(base?: I): Subgraph {
    return Subgraph.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Subgraph>, I>>(object: I): Subgraph {
    const message = createBaseSubgraph() as any;
    message.nodes = object.nodes?.map((e) => Node.fromPartial(e)) || [];
    return message;
  },
};

function createBasePrincipal(): Principal {
  return { kind: 0, name: "" };
}

export const Principal = {
  encode(message: Principal, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.kind !== 0) {
      writer.uint32(8).int32(message.kind);
    }
    if (message.name !== "") {
      writer.uint32(18).string(message.name);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Principal {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePrincipal() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.kind = reader.int32() as any;
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.name = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Principal {
    return {
      kind: isSet(object.kind) ? principalKindFromJSON(object.kind) : 0,
      name: isSet(object.name) ? globalThis.String(object.name) : "",
    };
  },

  toJSON(message: Principal): unknown {
    const obj: any = {};
    if (message.kind !== 0) {
      obj.kind = principalKindToJSON(message.kind);
    }
    if (message.name !== "") {
      obj.name = message.name;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Principal>, I>>(base?: I): Principal {
    return Principal.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Principal>, I>>(object: I): Principal {
    const message = createBasePrincipal() as any;
    message.kind = object.kind ?? 0;
    message.name = object.name ?? "";
    return message;
  },
};

function createBaseNode(): Node {
  return { principal: undefined, includedBy: [] };
}

export const Node = {
  encode(message: Node, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.principal !== undefined) {
      Principal.encode(message.principal, writer.uint32(10).fork()).ldelim();
    }
    writer.uint32(18).fork();
    for (const v of message.includedBy) {
      writer.int32(v);
    }
    writer.ldelim();
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Node {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseNode() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.principal = Principal.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag === 16) {
            message.includedBy.push(reader.int32());

            continue;
          }

          if (tag === 18) {
            const end2 = reader.uint32() + reader.pos;
            while (reader.pos < end2) {
              message.includedBy.push(reader.int32());
            }

            continue;
          }

          break;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Node {
    return {
      principal: isSet(object.principal) ? Principal.fromJSON(object.principal) : undefined,
      includedBy: globalThis.Array.isArray(object?.includedBy)
        ? object.includedBy.map((e: any) => globalThis.Number(e))
        : [],
    };
  },

  toJSON(message: Node): unknown {
    const obj: any = {};
    if (message.principal !== undefined) {
      obj.principal = Principal.toJSON(message.principal);
    }
    if (message.includedBy?.length) {
      obj.includedBy = message.includedBy.map((e) => Math.round(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Node>, I>>(base?: I): Node {
    return Node.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Node>, I>>(object: I): Node {
    const message = createBaseNode() as any;
    message.principal = (object.principal !== undefined && object.principal !== null)
      ? Principal.fromPartial(object.principal)
      : undefined;
    message.includedBy = object.includedBy?.map((e) => e) || [];
    return message;
  },
};

/** Groups service contains methods to examine groups. */
export interface Groups {
  /**
   * ListGroups returns all the groups currently stored in LUCI Auth service
   * datastore. The groups will be returned in alphabetical order based on their
   * ID.
   */
  ListGroups(request: Empty): Promise<ListGroupsResponse>;
  /** GetGroup returns information about an individual group, given the name. */
  GetGroup(request: GetGroupRequest): Promise<AuthGroup>;
  /** CreateGroup creates a new group. */
  CreateGroup(request: CreateGroupRequest): Promise<AuthGroup>;
  /** UpdateGroup updates an existing group. */
  UpdateGroup(request: UpdateGroupRequest): Promise<AuthGroup>;
  /** DeleteGroup deletes a group. */
  DeleteGroup(request: DeleteGroupRequest): Promise<Empty>;
  /**
   * GetSubgraph returns a Subgraph without information about groups that
   * include a principal (perhaps indirectly or via globs). Here a principal is
   * either an identity, a group or a glob (see PrincipalKind enum).
   */
  GetSubgraph(request: GetSubgraphRequest): Promise<Subgraph>;
}

export const GroupsServiceName = "auth.service.Groups";
export class GroupsClientImpl implements Groups {
  static readonly DEFAULT_SERVICE = GroupsServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || GroupsServiceName;
    this.rpc = rpc;
    this.ListGroups = this.ListGroups.bind(this);
    this.GetGroup = this.GetGroup.bind(this);
    this.CreateGroup = this.CreateGroup.bind(this);
    this.UpdateGroup = this.UpdateGroup.bind(this);
    this.DeleteGroup = this.DeleteGroup.bind(this);
    this.GetSubgraph = this.GetSubgraph.bind(this);
  }
  ListGroups(request: Empty): Promise<ListGroupsResponse> {
    const data = Empty.toJSON(request);
    const promise = this.rpc.request(this.service, "ListGroups", data);
    return promise.then((data) => ListGroupsResponse.fromJSON(data));
  }

  GetGroup(request: GetGroupRequest): Promise<AuthGroup> {
    const data = GetGroupRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "GetGroup", data);
    return promise.then((data) => AuthGroup.fromJSON(data));
  }

  CreateGroup(request: CreateGroupRequest): Promise<AuthGroup> {
    const data = CreateGroupRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "CreateGroup", data);
    return promise.then((data) => AuthGroup.fromJSON(data));
  }

  UpdateGroup(request: UpdateGroupRequest): Promise<AuthGroup> {
    const data = UpdateGroupRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "UpdateGroup", data);
    return promise.then((data) => AuthGroup.fromJSON(data));
  }

  DeleteGroup(request: DeleteGroupRequest): Promise<Empty> {
    const data = DeleteGroupRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "DeleteGroup", data);
    return promise.then((data) => Empty.fromJSON(data));
  }

  GetSubgraph(request: GetSubgraphRequest): Promise<Subgraph> {
    const data = GetSubgraphRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "GetSubgraph", data);
    return promise.then((data) => Subgraph.fromJSON(data));
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

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
