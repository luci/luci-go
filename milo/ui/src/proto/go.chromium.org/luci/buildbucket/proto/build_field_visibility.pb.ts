/* eslint-disable */

export const protobufPackage = "buildbucket.v2";

/**
 * Can be used to indicate that a buildbucket.v2.Build field should be visible
 * to users with the specified permission. By default, buildbucket.builds.get
 * is required to see fields, but this enum allows that access to be expanded.
 *
 * Note that we assume that users with GET_LIMITED also have LIST, and users
 * with GET also have GET_LIMITED and LIST.
 *
 * IMPORTANT: this enum must be ordered such that permissions that grant more
 * access (e.g. BUILDS_GET_PERMISSION) must always have lower enum numbers than
 * permissions that grant less access (e.g. BUILDS_LIST_PERMISSION).
 */
export enum BuildFieldVisibility {
  /**
   * FIELD_VISIBILITY_UNSPECIFIED - No visibility specified. In this case the visibility defaults to
   * requiring the buildbucket.builds.get permission.
   */
  FIELD_VISIBILITY_UNSPECIFIED = 0,
  /**
   * BUILDS_GET_PERMISSION - Indicates the field will only be visible to users with the
   * buildbucket.builds.get permission.
   */
  BUILDS_GET_PERMISSION = 1,
  /**
   * BUILDS_GET_LIMITED_PERMISSION - Indicates the field will be visible to users with either the
   * buildbucket.builds.getLimited or buildbucket.builds.get permission.
   */
  BUILDS_GET_LIMITED_PERMISSION = 2,
  /**
   * BUILDS_LIST_PERMISSION - Indicates the field will be visible to users with either the
   * buildbucket.builds.list, buildbucket.builds.getLimited or
   * buildbucket.builds.get permission.
   */
  BUILDS_LIST_PERMISSION = 3,
}

export function buildFieldVisibilityFromJSON(object: any): BuildFieldVisibility {
  switch (object) {
    case 0:
    case "FIELD_VISIBILITY_UNSPECIFIED":
      return BuildFieldVisibility.FIELD_VISIBILITY_UNSPECIFIED;
    case 1:
    case "BUILDS_GET_PERMISSION":
      return BuildFieldVisibility.BUILDS_GET_PERMISSION;
    case 2:
    case "BUILDS_GET_LIMITED_PERMISSION":
      return BuildFieldVisibility.BUILDS_GET_LIMITED_PERMISSION;
    case 3:
    case "BUILDS_LIST_PERMISSION":
      return BuildFieldVisibility.BUILDS_LIST_PERMISSION;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum BuildFieldVisibility");
  }
}

export function buildFieldVisibilityToJSON(object: BuildFieldVisibility): string {
  switch (object) {
    case BuildFieldVisibility.FIELD_VISIBILITY_UNSPECIFIED:
      return "FIELD_VISIBILITY_UNSPECIFIED";
    case BuildFieldVisibility.BUILDS_GET_PERMISSION:
      return "BUILDS_GET_PERMISSION";
    case BuildFieldVisibility.BUILDS_GET_LIMITED_PERMISSION:
      return "BUILDS_GET_LIMITED_PERMISSION";
    case BuildFieldVisibility.BUILDS_LIST_PERMISSION:
      return "BUILDS_LIST_PERMISSION";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum BuildFieldVisibility");
  }
}