// Code generated by protoc-gen-ts_proto. DO NOT EDIT.
// versions:
//   protoc-gen-ts_proto  v2.7.5
//   protoc               v6.31.1
// source: go.chromium.org/luci/auth_service/api/rpcpb/changelogs.proto

/* eslint-disable */
import { BinaryReader, BinaryWriter } from "@bufbuild/protobuf/wire";
import { Timestamp } from "../../../../../google/protobuf/timestamp.pb";

export const protobufPackage = "auth.service";

/**
 * ListChangeLogsRequest is a request to get a list of change logs, which can
 * be filtered by auth_db_rev and/or target.
 */
export interface ListChangeLogsRequest {
  /** AuthDB revision that the change log was made. */
  readonly authDbRev: string;
  /** Entity that was changed in the change log. */
  readonly target: string;
  /**
   * The value of next_page_token received in a ListChangeLogsResponse. Used
   * to get the next page of change logs. If empty, gets the first page.
   */
  readonly pageToken: string;
  /** The maximum number of change logs to include in the response. */
  readonly pageSize: number;
}

/** ListChangeLogsResponse contains a list of change logs that matched the query. */
export interface ListChangeLogsResponse {
  /** A list of change logs. */
  readonly changes: readonly AuthDBChange[];
  /**
   * The value to use as the page_token in a ListChangeLogsRequest to get the
   * next page of change logs. If empty, there are no more change logs.
   */
  readonly nextPageToken: string;
}

/** AuthDBChange refers to a change log entry. */
export interface AuthDBChange {
  /** Fields common across all change types. */
  readonly changeType: string;
  readonly target: string;
  readonly authDbRev: string;
  readonly who: string;
  readonly when: string | undefined;
  readonly comment: string;
  readonly appVersion: string;
  readonly description: string;
  readonly oldDescription: string;
  /** Fields specific to AuthDBGroupChange. */
  readonly owners: string;
  readonly oldOwners: string;
  readonly members: readonly string[];
  readonly globs: readonly string[];
  readonly nested: readonly string[];
  /** Fields specific to AuthDBIPAllowlistChange. */
  readonly subnets: readonly string[];
  /** Fields specific to AuthDBIPAllowlistAssignmentChange. */
  readonly identity: string;
  readonly ipAllowList: string;
  /** Fields specific to AuthDBConfigChange. */
  readonly oauthClientId: string;
  readonly oauthClientSecret: string;
  readonly oauthAdditionalClientIds: readonly string[];
  readonly tokenServerUrlOld: string;
  readonly tokenServerUrlNew: string;
  readonly securityConfigOld: string;
  readonly securityConfigNew: string;
  /** Fields specific to AuthRealmsGlobalsChange. */
  readonly permissionsAdded: readonly string[];
  readonly permissionsChanged: readonly string[];
  readonly permissionsRemoved: readonly string[];
  /** Fields specific to AuthProjectRealmsChange. */
  readonly configRevOld: string;
  readonly configRevNew: string;
  readonly permsRevOld: string;
  readonly permsRevNew: string;
}

function createBaseListChangeLogsRequest(): ListChangeLogsRequest {
  return { authDbRev: "0", target: "", pageToken: "", pageSize: 0 };
}

export const ListChangeLogsRequest: MessageFns<ListChangeLogsRequest> = {
  encode(message: ListChangeLogsRequest, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.authDbRev !== "0") {
      writer.uint32(8).int64(message.authDbRev);
    }
    if (message.target !== "") {
      writer.uint32(18).string(message.target);
    }
    if (message.pageToken !== "") {
      writer.uint32(26).string(message.pageToken);
    }
    if (message.pageSize !== 0) {
      writer.uint32(32).int32(message.pageSize);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): ListChangeLogsRequest {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListChangeLogsRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 8) {
            break;
          }

          message.authDbRev = reader.int64().toString();
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.target = reader.string();
          continue;
        }
        case 3: {
          if (tag !== 26) {
            break;
          }

          message.pageToken = reader.string();
          continue;
        }
        case 4: {
          if (tag !== 32) {
            break;
          }

          message.pageSize = reader.int32();
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

  fromJSON(object: any): ListChangeLogsRequest {
    return {
      authDbRev: isSet(object.authDbRev) ? globalThis.String(object.authDbRev) : "0",
      target: isSet(object.target) ? globalThis.String(object.target) : "",
      pageToken: isSet(object.pageToken) ? globalThis.String(object.pageToken) : "",
      pageSize: isSet(object.pageSize) ? globalThis.Number(object.pageSize) : 0,
    };
  },

  toJSON(message: ListChangeLogsRequest): unknown {
    const obj: any = {};
    if (message.authDbRev !== "0") {
      obj.authDbRev = message.authDbRev;
    }
    if (message.target !== "") {
      obj.target = message.target;
    }
    if (message.pageToken !== "") {
      obj.pageToken = message.pageToken;
    }
    if (message.pageSize !== 0) {
      obj.pageSize = Math.round(message.pageSize);
    }
    return obj;
  },

  create(base?: DeepPartial<ListChangeLogsRequest>): ListChangeLogsRequest {
    return ListChangeLogsRequest.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<ListChangeLogsRequest>): ListChangeLogsRequest {
    const message = createBaseListChangeLogsRequest() as any;
    message.authDbRev = object.authDbRev ?? "0";
    message.target = object.target ?? "";
    message.pageToken = object.pageToken ?? "";
    message.pageSize = object.pageSize ?? 0;
    return message;
  },
};

function createBaseListChangeLogsResponse(): ListChangeLogsResponse {
  return { changes: [], nextPageToken: "" };
}

export const ListChangeLogsResponse: MessageFns<ListChangeLogsResponse> = {
  encode(message: ListChangeLogsResponse, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    for (const v of message.changes) {
      AuthDBChange.encode(v!, writer.uint32(10).fork()).join();
    }
    if (message.nextPageToken !== "") {
      writer.uint32(18).string(message.nextPageToken);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): ListChangeLogsResponse {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListChangeLogsResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.changes.push(AuthDBChange.decode(reader, reader.uint32()));
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.nextPageToken = reader.string();
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

  fromJSON(object: any): ListChangeLogsResponse {
    return {
      changes: globalThis.Array.isArray(object?.changes)
        ? object.changes.map((e: any) => AuthDBChange.fromJSON(e))
        : [],
      nextPageToken: isSet(object.nextPageToken) ? globalThis.String(object.nextPageToken) : "",
    };
  },

  toJSON(message: ListChangeLogsResponse): unknown {
    const obj: any = {};
    if (message.changes?.length) {
      obj.changes = message.changes.map((e) => AuthDBChange.toJSON(e));
    }
    if (message.nextPageToken !== "") {
      obj.nextPageToken = message.nextPageToken;
    }
    return obj;
  },

  create(base?: DeepPartial<ListChangeLogsResponse>): ListChangeLogsResponse {
    return ListChangeLogsResponse.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<ListChangeLogsResponse>): ListChangeLogsResponse {
    const message = createBaseListChangeLogsResponse() as any;
    message.changes = object.changes?.map((e) => AuthDBChange.fromPartial(e)) || [];
    message.nextPageToken = object.nextPageToken ?? "";
    return message;
  },
};

function createBaseAuthDBChange(): AuthDBChange {
  return {
    changeType: "",
    target: "",
    authDbRev: "0",
    who: "",
    when: undefined,
    comment: "",
    appVersion: "",
    description: "",
    oldDescription: "",
    owners: "",
    oldOwners: "",
    members: [],
    globs: [],
    nested: [],
    subnets: [],
    identity: "",
    ipAllowList: "",
    oauthClientId: "",
    oauthClientSecret: "",
    oauthAdditionalClientIds: [],
    tokenServerUrlOld: "",
    tokenServerUrlNew: "",
    securityConfigOld: "",
    securityConfigNew: "",
    permissionsAdded: [],
    permissionsChanged: [],
    permissionsRemoved: [],
    configRevOld: "",
    configRevNew: "",
    permsRevOld: "",
    permsRevNew: "",
  };
}

export const AuthDBChange: MessageFns<AuthDBChange> = {
  encode(message: AuthDBChange, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.changeType !== "") {
      writer.uint32(10).string(message.changeType);
    }
    if (message.target !== "") {
      writer.uint32(18).string(message.target);
    }
    if (message.authDbRev !== "0") {
      writer.uint32(24).int64(message.authDbRev);
    }
    if (message.who !== "") {
      writer.uint32(34).string(message.who);
    }
    if (message.when !== undefined) {
      Timestamp.encode(toTimestamp(message.when), writer.uint32(42).fork()).join();
    }
    if (message.comment !== "") {
      writer.uint32(50).string(message.comment);
    }
    if (message.appVersion !== "") {
      writer.uint32(58).string(message.appVersion);
    }
    if (message.description !== "") {
      writer.uint32(66).string(message.description);
    }
    if (message.oldDescription !== "") {
      writer.uint32(74).string(message.oldDescription);
    }
    if (message.owners !== "") {
      writer.uint32(82).string(message.owners);
    }
    if (message.oldOwners !== "") {
      writer.uint32(90).string(message.oldOwners);
    }
    for (const v of message.members) {
      writer.uint32(98).string(v!);
    }
    for (const v of message.globs) {
      writer.uint32(106).string(v!);
    }
    for (const v of message.nested) {
      writer.uint32(114).string(v!);
    }
    for (const v of message.subnets) {
      writer.uint32(122).string(v!);
    }
    if (message.identity !== "") {
      writer.uint32(130).string(message.identity);
    }
    if (message.ipAllowList !== "") {
      writer.uint32(138).string(message.ipAllowList);
    }
    if (message.oauthClientId !== "") {
      writer.uint32(146).string(message.oauthClientId);
    }
    if (message.oauthClientSecret !== "") {
      writer.uint32(154).string(message.oauthClientSecret);
    }
    for (const v of message.oauthAdditionalClientIds) {
      writer.uint32(162).string(v!);
    }
    if (message.tokenServerUrlOld !== "") {
      writer.uint32(170).string(message.tokenServerUrlOld);
    }
    if (message.tokenServerUrlNew !== "") {
      writer.uint32(178).string(message.tokenServerUrlNew);
    }
    if (message.securityConfigOld !== "") {
      writer.uint32(186).string(message.securityConfigOld);
    }
    if (message.securityConfigNew !== "") {
      writer.uint32(194).string(message.securityConfigNew);
    }
    for (const v of message.permissionsAdded) {
      writer.uint32(202).string(v!);
    }
    for (const v of message.permissionsChanged) {
      writer.uint32(210).string(v!);
    }
    for (const v of message.permissionsRemoved) {
      writer.uint32(218).string(v!);
    }
    if (message.configRevOld !== "") {
      writer.uint32(226).string(message.configRevOld);
    }
    if (message.configRevNew !== "") {
      writer.uint32(234).string(message.configRevNew);
    }
    if (message.permsRevOld !== "") {
      writer.uint32(242).string(message.permsRevOld);
    }
    if (message.permsRevNew !== "") {
      writer.uint32(250).string(message.permsRevNew);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): AuthDBChange {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseAuthDBChange() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.changeType = reader.string();
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.target = reader.string();
          continue;
        }
        case 3: {
          if (tag !== 24) {
            break;
          }

          message.authDbRev = reader.int64().toString();
          continue;
        }
        case 4: {
          if (tag !== 34) {
            break;
          }

          message.who = reader.string();
          continue;
        }
        case 5: {
          if (tag !== 42) {
            break;
          }

          message.when = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        }
        case 6: {
          if (tag !== 50) {
            break;
          }

          message.comment = reader.string();
          continue;
        }
        case 7: {
          if (tag !== 58) {
            break;
          }

          message.appVersion = reader.string();
          continue;
        }
        case 8: {
          if (tag !== 66) {
            break;
          }

          message.description = reader.string();
          continue;
        }
        case 9: {
          if (tag !== 74) {
            break;
          }

          message.oldDescription = reader.string();
          continue;
        }
        case 10: {
          if (tag !== 82) {
            break;
          }

          message.owners = reader.string();
          continue;
        }
        case 11: {
          if (tag !== 90) {
            break;
          }

          message.oldOwners = reader.string();
          continue;
        }
        case 12: {
          if (tag !== 98) {
            break;
          }

          message.members.push(reader.string());
          continue;
        }
        case 13: {
          if (tag !== 106) {
            break;
          }

          message.globs.push(reader.string());
          continue;
        }
        case 14: {
          if (tag !== 114) {
            break;
          }

          message.nested.push(reader.string());
          continue;
        }
        case 15: {
          if (tag !== 122) {
            break;
          }

          message.subnets.push(reader.string());
          continue;
        }
        case 16: {
          if (tag !== 130) {
            break;
          }

          message.identity = reader.string();
          continue;
        }
        case 17: {
          if (tag !== 138) {
            break;
          }

          message.ipAllowList = reader.string();
          continue;
        }
        case 18: {
          if (tag !== 146) {
            break;
          }

          message.oauthClientId = reader.string();
          continue;
        }
        case 19: {
          if (tag !== 154) {
            break;
          }

          message.oauthClientSecret = reader.string();
          continue;
        }
        case 20: {
          if (tag !== 162) {
            break;
          }

          message.oauthAdditionalClientIds.push(reader.string());
          continue;
        }
        case 21: {
          if (tag !== 170) {
            break;
          }

          message.tokenServerUrlOld = reader.string();
          continue;
        }
        case 22: {
          if (tag !== 178) {
            break;
          }

          message.tokenServerUrlNew = reader.string();
          continue;
        }
        case 23: {
          if (tag !== 186) {
            break;
          }

          message.securityConfigOld = reader.string();
          continue;
        }
        case 24: {
          if (tag !== 194) {
            break;
          }

          message.securityConfigNew = reader.string();
          continue;
        }
        case 25: {
          if (tag !== 202) {
            break;
          }

          message.permissionsAdded.push(reader.string());
          continue;
        }
        case 26: {
          if (tag !== 210) {
            break;
          }

          message.permissionsChanged.push(reader.string());
          continue;
        }
        case 27: {
          if (tag !== 218) {
            break;
          }

          message.permissionsRemoved.push(reader.string());
          continue;
        }
        case 28: {
          if (tag !== 226) {
            break;
          }

          message.configRevOld = reader.string();
          continue;
        }
        case 29: {
          if (tag !== 234) {
            break;
          }

          message.configRevNew = reader.string();
          continue;
        }
        case 30: {
          if (tag !== 242) {
            break;
          }

          message.permsRevOld = reader.string();
          continue;
        }
        case 31: {
          if (tag !== 250) {
            break;
          }

          message.permsRevNew = reader.string();
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

  fromJSON(object: any): AuthDBChange {
    return {
      changeType: isSet(object.changeType) ? globalThis.String(object.changeType) : "",
      target: isSet(object.target) ? globalThis.String(object.target) : "",
      authDbRev: isSet(object.authDbRev) ? globalThis.String(object.authDbRev) : "0",
      who: isSet(object.who) ? globalThis.String(object.who) : "",
      when: isSet(object.when) ? globalThis.String(object.when) : undefined,
      comment: isSet(object.comment) ? globalThis.String(object.comment) : "",
      appVersion: isSet(object.appVersion) ? globalThis.String(object.appVersion) : "",
      description: isSet(object.description) ? globalThis.String(object.description) : "",
      oldDescription: isSet(object.oldDescription) ? globalThis.String(object.oldDescription) : "",
      owners: isSet(object.owners) ? globalThis.String(object.owners) : "",
      oldOwners: isSet(object.oldOwners) ? globalThis.String(object.oldOwners) : "",
      members: globalThis.Array.isArray(object?.members) ? object.members.map((e: any) => globalThis.String(e)) : [],
      globs: globalThis.Array.isArray(object?.globs) ? object.globs.map((e: any) => globalThis.String(e)) : [],
      nested: globalThis.Array.isArray(object?.nested) ? object.nested.map((e: any) => globalThis.String(e)) : [],
      subnets: globalThis.Array.isArray(object?.subnets) ? object.subnets.map((e: any) => globalThis.String(e)) : [],
      identity: isSet(object.identity) ? globalThis.String(object.identity) : "",
      ipAllowList: isSet(object.ipAllowList) ? globalThis.String(object.ipAllowList) : "",
      oauthClientId: isSet(object.oauthClientId) ? globalThis.String(object.oauthClientId) : "",
      oauthClientSecret: isSet(object.oauthClientSecret) ? globalThis.String(object.oauthClientSecret) : "",
      oauthAdditionalClientIds: globalThis.Array.isArray(object?.oauthAdditionalClientIds)
        ? object.oauthAdditionalClientIds.map((e: any) => globalThis.String(e))
        : [],
      tokenServerUrlOld: isSet(object.tokenServerUrlOld) ? globalThis.String(object.tokenServerUrlOld) : "",
      tokenServerUrlNew: isSet(object.tokenServerUrlNew) ? globalThis.String(object.tokenServerUrlNew) : "",
      securityConfigOld: isSet(object.securityConfigOld) ? globalThis.String(object.securityConfigOld) : "",
      securityConfigNew: isSet(object.securityConfigNew) ? globalThis.String(object.securityConfigNew) : "",
      permissionsAdded: globalThis.Array.isArray(object?.permissionsAdded)
        ? object.permissionsAdded.map((e: any) => globalThis.String(e))
        : [],
      permissionsChanged: globalThis.Array.isArray(object?.permissionsChanged)
        ? object.permissionsChanged.map((e: any) => globalThis.String(e))
        : [],
      permissionsRemoved: globalThis.Array.isArray(object?.permissionsRemoved)
        ? object.permissionsRemoved.map((e: any) => globalThis.String(e))
        : [],
      configRevOld: isSet(object.configRevOld) ? globalThis.String(object.configRevOld) : "",
      configRevNew: isSet(object.configRevNew) ? globalThis.String(object.configRevNew) : "",
      permsRevOld: isSet(object.permsRevOld) ? globalThis.String(object.permsRevOld) : "",
      permsRevNew: isSet(object.permsRevNew) ? globalThis.String(object.permsRevNew) : "",
    };
  },

  toJSON(message: AuthDBChange): unknown {
    const obj: any = {};
    if (message.changeType !== "") {
      obj.changeType = message.changeType;
    }
    if (message.target !== "") {
      obj.target = message.target;
    }
    if (message.authDbRev !== "0") {
      obj.authDbRev = message.authDbRev;
    }
    if (message.who !== "") {
      obj.who = message.who;
    }
    if (message.when !== undefined) {
      obj.when = message.when;
    }
    if (message.comment !== "") {
      obj.comment = message.comment;
    }
    if (message.appVersion !== "") {
      obj.appVersion = message.appVersion;
    }
    if (message.description !== "") {
      obj.description = message.description;
    }
    if (message.oldDescription !== "") {
      obj.oldDescription = message.oldDescription;
    }
    if (message.owners !== "") {
      obj.owners = message.owners;
    }
    if (message.oldOwners !== "") {
      obj.oldOwners = message.oldOwners;
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
    if (message.subnets?.length) {
      obj.subnets = message.subnets;
    }
    if (message.identity !== "") {
      obj.identity = message.identity;
    }
    if (message.ipAllowList !== "") {
      obj.ipAllowList = message.ipAllowList;
    }
    if (message.oauthClientId !== "") {
      obj.oauthClientId = message.oauthClientId;
    }
    if (message.oauthClientSecret !== "") {
      obj.oauthClientSecret = message.oauthClientSecret;
    }
    if (message.oauthAdditionalClientIds?.length) {
      obj.oauthAdditionalClientIds = message.oauthAdditionalClientIds;
    }
    if (message.tokenServerUrlOld !== "") {
      obj.tokenServerUrlOld = message.tokenServerUrlOld;
    }
    if (message.tokenServerUrlNew !== "") {
      obj.tokenServerUrlNew = message.tokenServerUrlNew;
    }
    if (message.securityConfigOld !== "") {
      obj.securityConfigOld = message.securityConfigOld;
    }
    if (message.securityConfigNew !== "") {
      obj.securityConfigNew = message.securityConfigNew;
    }
    if (message.permissionsAdded?.length) {
      obj.permissionsAdded = message.permissionsAdded;
    }
    if (message.permissionsChanged?.length) {
      obj.permissionsChanged = message.permissionsChanged;
    }
    if (message.permissionsRemoved?.length) {
      obj.permissionsRemoved = message.permissionsRemoved;
    }
    if (message.configRevOld !== "") {
      obj.configRevOld = message.configRevOld;
    }
    if (message.configRevNew !== "") {
      obj.configRevNew = message.configRevNew;
    }
    if (message.permsRevOld !== "") {
      obj.permsRevOld = message.permsRevOld;
    }
    if (message.permsRevNew !== "") {
      obj.permsRevNew = message.permsRevNew;
    }
    return obj;
  },

  create(base?: DeepPartial<AuthDBChange>): AuthDBChange {
    return AuthDBChange.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<AuthDBChange>): AuthDBChange {
    const message = createBaseAuthDBChange() as any;
    message.changeType = object.changeType ?? "";
    message.target = object.target ?? "";
    message.authDbRev = object.authDbRev ?? "0";
    message.who = object.who ?? "";
    message.when = object.when ?? undefined;
    message.comment = object.comment ?? "";
    message.appVersion = object.appVersion ?? "";
    message.description = object.description ?? "";
    message.oldDescription = object.oldDescription ?? "";
    message.owners = object.owners ?? "";
    message.oldOwners = object.oldOwners ?? "";
    message.members = object.members?.map((e) => e) || [];
    message.globs = object.globs?.map((e) => e) || [];
    message.nested = object.nested?.map((e) => e) || [];
    message.subnets = object.subnets?.map((e) => e) || [];
    message.identity = object.identity ?? "";
    message.ipAllowList = object.ipAllowList ?? "";
    message.oauthClientId = object.oauthClientId ?? "";
    message.oauthClientSecret = object.oauthClientSecret ?? "";
    message.oauthAdditionalClientIds = object.oauthAdditionalClientIds?.map((e) => e) || [];
    message.tokenServerUrlOld = object.tokenServerUrlOld ?? "";
    message.tokenServerUrlNew = object.tokenServerUrlNew ?? "";
    message.securityConfigOld = object.securityConfigOld ?? "";
    message.securityConfigNew = object.securityConfigNew ?? "";
    message.permissionsAdded = object.permissionsAdded?.map((e) => e) || [];
    message.permissionsChanged = object.permissionsChanged?.map((e) => e) || [];
    message.permissionsRemoved = object.permissionsRemoved?.map((e) => e) || [];
    message.configRevOld = object.configRevOld ?? "";
    message.configRevNew = object.configRevNew ?? "";
    message.permsRevOld = object.permsRevOld ?? "";
    message.permsRevNew = object.permsRevNew ?? "";
    return message;
  },
};

/** ChangeLogs service contains methods to examine change logs. */
export interface ChangeLogs {
  /** ListChangeLogs returns all the change logs in Datastore. */
  ListChangeLogs(request: ListChangeLogsRequest): Promise<ListChangeLogsResponse>;
}

export const ChangeLogsServiceName = "auth.service.ChangeLogs";
export class ChangeLogsClientImpl implements ChangeLogs {
  static readonly DEFAULT_SERVICE = ChangeLogsServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || ChangeLogsServiceName;
    this.rpc = rpc;
    this.ListChangeLogs = this.ListChangeLogs.bind(this);
  }
  ListChangeLogs(request: ListChangeLogsRequest): Promise<ListChangeLogsResponse> {
    const data = ListChangeLogsRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "ListChangeLogs", data);
    return promise.then((data) => ListChangeLogsResponse.fromJSON(data));
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

export interface MessageFns<T> {
  encode(message: T, writer?: BinaryWriter): BinaryWriter;
  decode(input: BinaryReader | Uint8Array, length?: number): T;
  fromJSON(object: any): T;
  toJSON(message: T): unknown;
  create(base?: DeepPartial<T>): T;
  fromPartial(object: DeepPartial<T>): T;
}
