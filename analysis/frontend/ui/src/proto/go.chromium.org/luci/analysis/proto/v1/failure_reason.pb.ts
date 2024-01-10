/* eslint-disable */
import _m0 from "protobufjs/minimal";

export const protobufPackage = "luci.analysis.v1";

/** Information about why a test failed. */
export interface FailureReason {
  /**
   * The error message that ultimately caused the test to fail. This should
   * only be the error message and should not include any stack traces.
   * An example would be the message from an Exception in a Java test.
   * In the case that a test failed due to multiple expectation failures, any
   * immediately fatal failure should be chosen, or otherwise the first
   * expectation failure.
   * If this field is empty, other fields may be used to cluster the failure
   * instead.
   *
   * The size of the message must be equal to or smaller than 1024 bytes in
   * UTF-8.
   */
  readonly primaryErrorMessage: string;
}

function createBaseFailureReason(): FailureReason {
  return { primaryErrorMessage: "" };
}

export const FailureReason = {
  encode(message: FailureReason, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.primaryErrorMessage !== "") {
      writer.uint32(10).string(message.primaryErrorMessage);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FailureReason {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFailureReason() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.primaryErrorMessage = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): FailureReason {
    return {
      primaryErrorMessage: isSet(object.primaryErrorMessage) ? globalThis.String(object.primaryErrorMessage) : "",
    };
  },

  toJSON(message: FailureReason): unknown {
    const obj: any = {};
    if (message.primaryErrorMessage !== "") {
      obj.primaryErrorMessage = message.primaryErrorMessage;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<FailureReason>, I>>(base?: I): FailureReason {
    return FailureReason.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<FailureReason>, I>>(object: I): FailureReason {
    const message = createBaseFailureReason() as any;
    message.primaryErrorMessage = object.primaryErrorMessage ?? "";
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
