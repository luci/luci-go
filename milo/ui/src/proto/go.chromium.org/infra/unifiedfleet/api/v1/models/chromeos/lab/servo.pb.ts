// Code generated by protoc-gen-ts_proto. DO NOT EDIT.
// versions:
//   protoc-gen-ts_proto  v2.7.5
//   protoc               v6.31.1
// source: go.chromium.org/infra/unifiedfleet/api/v1/models/chromeos/lab/servo.proto

/* eslint-disable */
import { BinaryReader, BinaryWriter } from "@bufbuild/protobuf/wire";
import { UsbDrive } from "../../../../../../../chromiumos/config/proto/chromiumos/test/lab/api/usb_drive.pb";

export const protobufPackage = "unifiedfleet.api.v1.models.chromeos.lab";

/**
 * Servo Setup Type describes the capabilities of servos.
 * Next Tag : 3
 */
export enum ServoSetupType {
  SERVO_SETUP_REGULAR = 0,
  SERVO_SETUP_DUAL_V4 = 1,
  /** SERVO_SETUP_INVALID - SERVO_SETUP_INVALID explicitly marks errors in servo setup. */
  SERVO_SETUP_INVALID = 2,
}

export function servoSetupTypeFromJSON(object: any): ServoSetupType {
  switch (object) {
    case 0:
    case "SERVO_SETUP_REGULAR":
      return ServoSetupType.SERVO_SETUP_REGULAR;
    case 1:
    case "SERVO_SETUP_DUAL_V4":
      return ServoSetupType.SERVO_SETUP_DUAL_V4;
    case 2:
    case "SERVO_SETUP_INVALID":
      return ServoSetupType.SERVO_SETUP_INVALID;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum ServoSetupType");
  }
}

export function servoSetupTypeToJSON(object: ServoSetupType): string {
  switch (object) {
    case ServoSetupType.SERVO_SETUP_REGULAR:
      return "SERVO_SETUP_REGULAR";
    case ServoSetupType.SERVO_SETUP_DUAL_V4:
      return "SERVO_SETUP_DUAL_V4";
    case ServoSetupType.SERVO_SETUP_INVALID:
      return "SERVO_SETUP_INVALID";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum ServoSetupType");
  }
}

/**
 * Servo Firmware Channel describes the firmware expected to have on servos.
 * Next Tag : 4
 */
export enum ServoFwChannel {
  /** SERVO_FW_STABLE - Servo firmware from Stable channel. */
  SERVO_FW_STABLE = 0,
  /** SERVO_FW_PREV - The previous Servo firmware from Stable channel. */
  SERVO_FW_PREV = 1,
  /** SERVO_FW_DEV - Servo firmware from Dev channel. */
  SERVO_FW_DEV = 2,
  /** SERVO_FW_ALPHA - Servo firmware from Alpha channel. */
  SERVO_FW_ALPHA = 3,
}

export function servoFwChannelFromJSON(object: any): ServoFwChannel {
  switch (object) {
    case 0:
    case "SERVO_FW_STABLE":
      return ServoFwChannel.SERVO_FW_STABLE;
    case 1:
    case "SERVO_FW_PREV":
      return ServoFwChannel.SERVO_FW_PREV;
    case 2:
    case "SERVO_FW_DEV":
      return ServoFwChannel.SERVO_FW_DEV;
    case 3:
    case "SERVO_FW_ALPHA":
      return ServoFwChannel.SERVO_FW_ALPHA;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum ServoFwChannel");
  }
}

export function servoFwChannelToJSON(object: ServoFwChannel): string {
  switch (object) {
    case ServoFwChannel.SERVO_FW_STABLE:
      return "SERVO_FW_STABLE";
    case ServoFwChannel.SERVO_FW_PREV:
      return "SERVO_FW_PREV";
    case ServoFwChannel.SERVO_FW_DEV:
      return "SERVO_FW_DEV";
    case ServoFwChannel.SERVO_FW_ALPHA:
      return "SERVO_FW_ALPHA";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum ServoFwChannel");
  }
}

/** NEXT TAG: 13 */
export interface Servo {
  /** Servo-specific configs */
  readonly servoHostname: string;
  readonly servoPort: number;
  readonly servoSerial: string;
  /**
   * Based on https://docs.google.com/document/d/1TPp7yp-uwFUh5xOnBLI4jPYtYD7IcdyQ1dgqFqtcJEU/edit?ts=5d8eafb7#heading=h.csdfk1i6g0l
   * servo_type will contain different setup of servos. So string is recommended than enum.
   */
  readonly servoType: string;
  readonly servoSetup: ServoSetupType;
  /** Based on http://go/fleet-servo-topology */
  readonly servoTopology: ServoTopology | undefined;
  readonly servoFwChannel: ServoFwChannel;
  readonly servoComponent: readonly string[];
  /** b/190538710 optional docker container name if servod is running in docker */
  readonly dockerContainerName: string;
  /** UsbDrive contains details of the servo's plugged USB drive. */
  readonly usbDrive: UsbDrive | undefined;
}

/**
 * Servo Topology describe connected servo devices on DUT set-up to provide Servo functionality.
 * Next Tag : 3
 */
export interface ServoTopology {
  readonly main: ServoTopologyItem | undefined;
  readonly children: readonly ServoTopologyItem[];
}

/**
 * Servo Topology Item describe details of one servo device on DUT set-up.
 * Next Tag : 8
 */
export interface ServoTopologyItem {
  /** type provides the type of servo device. Keeping as String to avoid issue with introduce new type. */
  readonly type: string;
  /** sysfs_product provides the product name of the device recorded in File System. */
  readonly sysfsProduct: string;
  /** serial provides the serial number of the device. */
  readonly serial: string;
  /**
   * usb_hub_port provides the port connection to the device.
   * e.g. '1-6.2.2' where
   *   '1-6'  - port on the labstation
   *   '2'    - port on smart-hub connected to the labstation
   *   '2'    - port on servo hub (part of servo_v4 or servo_v4.1) connected to the smart-hub
   * The same path will look '1-6.2' if connected servo_v4 directly to the labstation.
   */
  readonly usbHubPort: string;
  /** This is the firmware version of servo device. */
  readonly fwVersion: string;
  /** This is the complete path on the file system for the servo device. */
  readonly sysfsPath: string;
  /**
   * This is vendor and product ids of the servo device.
   * e.g. `18d1:5014`
   */
  readonly vidPid: string;
}

function createBaseServo(): Servo {
  return {
    servoHostname: "",
    servoPort: 0,
    servoSerial: "",
    servoType: "",
    servoSetup: 0,
    servoTopology: undefined,
    servoFwChannel: 0,
    servoComponent: [],
    dockerContainerName: "",
    usbDrive: undefined,
  };
}

export const Servo: MessageFns<Servo> = {
  encode(message: Servo, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.servoHostname !== "") {
      writer.uint32(18).string(message.servoHostname);
    }
    if (message.servoPort !== 0) {
      writer.uint32(24).int32(message.servoPort);
    }
    if (message.servoSerial !== "") {
      writer.uint32(34).string(message.servoSerial);
    }
    if (message.servoType !== "") {
      writer.uint32(42).string(message.servoType);
    }
    if (message.servoSetup !== 0) {
      writer.uint32(56).int32(message.servoSetup);
    }
    if (message.servoTopology !== undefined) {
      ServoTopology.encode(message.servoTopology, writer.uint32(66).fork()).join();
    }
    if (message.servoFwChannel !== 0) {
      writer.uint32(72).int32(message.servoFwChannel);
    }
    for (const v of message.servoComponent) {
      writer.uint32(90).string(v!);
    }
    if (message.dockerContainerName !== "") {
      writer.uint32(82).string(message.dockerContainerName);
    }
    if (message.usbDrive !== undefined) {
      UsbDrive.encode(message.usbDrive, writer.uint32(98).fork()).join();
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): Servo {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseServo() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.servoHostname = reader.string();
          continue;
        }
        case 3: {
          if (tag !== 24) {
            break;
          }

          message.servoPort = reader.int32();
          continue;
        }
        case 4: {
          if (tag !== 34) {
            break;
          }

          message.servoSerial = reader.string();
          continue;
        }
        case 5: {
          if (tag !== 42) {
            break;
          }

          message.servoType = reader.string();
          continue;
        }
        case 7: {
          if (tag !== 56) {
            break;
          }

          message.servoSetup = reader.int32() as any;
          continue;
        }
        case 8: {
          if (tag !== 66) {
            break;
          }

          message.servoTopology = ServoTopology.decode(reader, reader.uint32());
          continue;
        }
        case 9: {
          if (tag !== 72) {
            break;
          }

          message.servoFwChannel = reader.int32() as any;
          continue;
        }
        case 11: {
          if (tag !== 90) {
            break;
          }

          message.servoComponent.push(reader.string());
          continue;
        }
        case 10: {
          if (tag !== 82) {
            break;
          }

          message.dockerContainerName = reader.string();
          continue;
        }
        case 12: {
          if (tag !== 98) {
            break;
          }

          message.usbDrive = UsbDrive.decode(reader, reader.uint32());
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

  fromJSON(object: any): Servo {
    return {
      servoHostname: isSet(object.servoHostname) ? globalThis.String(object.servoHostname) : "",
      servoPort: isSet(object.servoPort) ? globalThis.Number(object.servoPort) : 0,
      servoSerial: isSet(object.servoSerial) ? globalThis.String(object.servoSerial) : "",
      servoType: isSet(object.servoType) ? globalThis.String(object.servoType) : "",
      servoSetup: isSet(object.servoSetup) ? servoSetupTypeFromJSON(object.servoSetup) : 0,
      servoTopology: isSet(object.servoTopology) ? ServoTopology.fromJSON(object.servoTopology) : undefined,
      servoFwChannel: isSet(object.servoFwChannel) ? servoFwChannelFromJSON(object.servoFwChannel) : 0,
      servoComponent: globalThis.Array.isArray(object?.servoComponent)
        ? object.servoComponent.map((e: any) => globalThis.String(e))
        : [],
      dockerContainerName: isSet(object.dockerContainerName) ? globalThis.String(object.dockerContainerName) : "",
      usbDrive: isSet(object.usbDrive) ? UsbDrive.fromJSON(object.usbDrive) : undefined,
    };
  },

  toJSON(message: Servo): unknown {
    const obj: any = {};
    if (message.servoHostname !== "") {
      obj.servoHostname = message.servoHostname;
    }
    if (message.servoPort !== 0) {
      obj.servoPort = Math.round(message.servoPort);
    }
    if (message.servoSerial !== "") {
      obj.servoSerial = message.servoSerial;
    }
    if (message.servoType !== "") {
      obj.servoType = message.servoType;
    }
    if (message.servoSetup !== 0) {
      obj.servoSetup = servoSetupTypeToJSON(message.servoSetup);
    }
    if (message.servoTopology !== undefined) {
      obj.servoTopology = ServoTopology.toJSON(message.servoTopology);
    }
    if (message.servoFwChannel !== 0) {
      obj.servoFwChannel = servoFwChannelToJSON(message.servoFwChannel);
    }
    if (message.servoComponent?.length) {
      obj.servoComponent = message.servoComponent;
    }
    if (message.dockerContainerName !== "") {
      obj.dockerContainerName = message.dockerContainerName;
    }
    if (message.usbDrive !== undefined) {
      obj.usbDrive = UsbDrive.toJSON(message.usbDrive);
    }
    return obj;
  },

  create(base?: DeepPartial<Servo>): Servo {
    return Servo.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<Servo>): Servo {
    const message = createBaseServo() as any;
    message.servoHostname = object.servoHostname ?? "";
    message.servoPort = object.servoPort ?? 0;
    message.servoSerial = object.servoSerial ?? "";
    message.servoType = object.servoType ?? "";
    message.servoSetup = object.servoSetup ?? 0;
    message.servoTopology = (object.servoTopology !== undefined && object.servoTopology !== null)
      ? ServoTopology.fromPartial(object.servoTopology)
      : undefined;
    message.servoFwChannel = object.servoFwChannel ?? 0;
    message.servoComponent = object.servoComponent?.map((e) => e) || [];
    message.dockerContainerName = object.dockerContainerName ?? "";
    message.usbDrive = (object.usbDrive !== undefined && object.usbDrive !== null)
      ? UsbDrive.fromPartial(object.usbDrive)
      : undefined;
    return message;
  },
};

function createBaseServoTopology(): ServoTopology {
  return { main: undefined, children: [] };
}

export const ServoTopology: MessageFns<ServoTopology> = {
  encode(message: ServoTopology, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.main !== undefined) {
      ServoTopologyItem.encode(message.main, writer.uint32(10).fork()).join();
    }
    for (const v of message.children) {
      ServoTopologyItem.encode(v!, writer.uint32(18).fork()).join();
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): ServoTopology {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseServoTopology() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.main = ServoTopologyItem.decode(reader, reader.uint32());
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.children.push(ServoTopologyItem.decode(reader, reader.uint32()));
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

  fromJSON(object: any): ServoTopology {
    return {
      main: isSet(object.main) ? ServoTopologyItem.fromJSON(object.main) : undefined,
      children: globalThis.Array.isArray(object?.children)
        ? object.children.map((e: any) => ServoTopologyItem.fromJSON(e))
        : [],
    };
  },

  toJSON(message: ServoTopology): unknown {
    const obj: any = {};
    if (message.main !== undefined) {
      obj.main = ServoTopologyItem.toJSON(message.main);
    }
    if (message.children?.length) {
      obj.children = message.children.map((e) => ServoTopologyItem.toJSON(e));
    }
    return obj;
  },

  create(base?: DeepPartial<ServoTopology>): ServoTopology {
    return ServoTopology.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<ServoTopology>): ServoTopology {
    const message = createBaseServoTopology() as any;
    message.main = (object.main !== undefined && object.main !== null)
      ? ServoTopologyItem.fromPartial(object.main)
      : undefined;
    message.children = object.children?.map((e) => ServoTopologyItem.fromPartial(e)) || [];
    return message;
  },
};

function createBaseServoTopologyItem(): ServoTopologyItem {
  return { type: "", sysfsProduct: "", serial: "", usbHubPort: "", fwVersion: "", sysfsPath: "", vidPid: "" };
}

export const ServoTopologyItem: MessageFns<ServoTopologyItem> = {
  encode(message: ServoTopologyItem, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.type !== "") {
      writer.uint32(10).string(message.type);
    }
    if (message.sysfsProduct !== "") {
      writer.uint32(18).string(message.sysfsProduct);
    }
    if (message.serial !== "") {
      writer.uint32(26).string(message.serial);
    }
    if (message.usbHubPort !== "") {
      writer.uint32(34).string(message.usbHubPort);
    }
    if (message.fwVersion !== "") {
      writer.uint32(42).string(message.fwVersion);
    }
    if (message.sysfsPath !== "") {
      writer.uint32(50).string(message.sysfsPath);
    }
    if (message.vidPid !== "") {
      writer.uint32(58).string(message.vidPid);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): ServoTopologyItem {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseServoTopologyItem() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.type = reader.string();
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.sysfsProduct = reader.string();
          continue;
        }
        case 3: {
          if (tag !== 26) {
            break;
          }

          message.serial = reader.string();
          continue;
        }
        case 4: {
          if (tag !== 34) {
            break;
          }

          message.usbHubPort = reader.string();
          continue;
        }
        case 5: {
          if (tag !== 42) {
            break;
          }

          message.fwVersion = reader.string();
          continue;
        }
        case 6: {
          if (tag !== 50) {
            break;
          }

          message.sysfsPath = reader.string();
          continue;
        }
        case 7: {
          if (tag !== 58) {
            break;
          }

          message.vidPid = reader.string();
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

  fromJSON(object: any): ServoTopologyItem {
    return {
      type: isSet(object.type) ? globalThis.String(object.type) : "",
      sysfsProduct: isSet(object.sysfsProduct) ? globalThis.String(object.sysfsProduct) : "",
      serial: isSet(object.serial) ? globalThis.String(object.serial) : "",
      usbHubPort: isSet(object.usbHubPort) ? globalThis.String(object.usbHubPort) : "",
      fwVersion: isSet(object.fwVersion) ? globalThis.String(object.fwVersion) : "",
      sysfsPath: isSet(object.sysfsPath) ? globalThis.String(object.sysfsPath) : "",
      vidPid: isSet(object.vidPid) ? globalThis.String(object.vidPid) : "",
    };
  },

  toJSON(message: ServoTopologyItem): unknown {
    const obj: any = {};
    if (message.type !== "") {
      obj.type = message.type;
    }
    if (message.sysfsProduct !== "") {
      obj.sysfsProduct = message.sysfsProduct;
    }
    if (message.serial !== "") {
      obj.serial = message.serial;
    }
    if (message.usbHubPort !== "") {
      obj.usbHubPort = message.usbHubPort;
    }
    if (message.fwVersion !== "") {
      obj.fwVersion = message.fwVersion;
    }
    if (message.sysfsPath !== "") {
      obj.sysfsPath = message.sysfsPath;
    }
    if (message.vidPid !== "") {
      obj.vidPid = message.vidPid;
    }
    return obj;
  },

  create(base?: DeepPartial<ServoTopologyItem>): ServoTopologyItem {
    return ServoTopologyItem.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<ServoTopologyItem>): ServoTopologyItem {
    const message = createBaseServoTopologyItem() as any;
    message.type = object.type ?? "";
    message.sysfsProduct = object.sysfsProduct ?? "";
    message.serial = object.serial ?? "";
    message.usbHubPort = object.usbHubPort ?? "";
    message.fwVersion = object.fwVersion ?? "";
    message.sysfsPath = object.sysfsPath ?? "";
    message.vidPid = object.vidPid ?? "";
    return message;
  },
};

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
