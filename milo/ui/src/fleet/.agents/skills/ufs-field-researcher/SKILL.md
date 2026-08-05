---
name: ufs-field-researcher
description: Researches a Unified Fleet System (UFS) and Shivas inventory field by searching exact controller functions, protobuf messages, and CLI flag registrations. Outputs a formatted Field Technical Context block for use in chat. Use when adding a new editable inventory field to Fleet Console or when investigating UFS field mechanics and history.
---

# UFS Field Researcher Skill

## Quick Start

When invoked to research an inventory field (e.g., `"hive"`, `"rpm"`, `"zone"`), follow this structured workflow to locate live symbol definitions by function and message names across `infra/unifiedfleet`, `infra/cmd/shivas`, and `infra/fleetconsole`, and output an authoritative **Field Technical Context** block.

> [!IMPORTANT]
> **At the start of executing this skill**, you MUST copy the progress checklist below into your very next response to the user, and check off the steps sequentially as you complete them to ensure structured progress visibility.

```
Progress:
- [ ] Step 1: Locate protobuf message definition and enumerate sibling fields (including fallbacks)
- [ ] Step 2: Search exact UFS controller mask validation, update processing, and autocomplete rules
- [ ] Step 3: Extract introducing Gerrit CL URL and Buganizer ID via git blame/log in UFS repo
- [ ] Step 4: Search Shivas CLI flag registrations (including machine/asset update fallbacks)
- [ ] Step 5: Check existing Fleet Console backend proto, path constants, and UFS mappers
- [ ] Step 6: Output verified Field Technical Context block in chat response
```

## Step 1: Locate UFS Protobuf Definition & Enumerate Sibling Fields

In the `infra/unifiedfleet` repository:
1.  Search `api/v1/models/chromeos/lab/` and `api/v1/models/` for the target protobuf message defining the field:
    *   **DUT Model**: `message DeviceUnderTest` in `device.proto`
    *   **Labstation Model**: `message Labstation` in `device.proto`
    *   **Peripheral Models**: `peripherals.proto` (both in `api/v1/models/` and `api/v1/models/chromeos/lab/`), `servo.proto`, `rpm.proto`, `dolos.proto`, `chameleon.proto`
2.  **Inheritance Fallback Search**: If the field is not found on `DeviceUnderTest` or `Labstation` (e.g., `zone`, `rack`, `realm`), it is inherited from the physical machine or asset record. Search these fallback protobuf models in `api/v1/models/`:
    *   **MachineLSE Model**: `machine_lse.proto` (`message MachineLSE`)
    *   **Machine & Location Models**: `machine.proto`, `location.proto` (`message Location`)
    *   **Asset Model**: `asset.proto`
3.  **Sibling Field Enumeration Rule**: If the target field belongs to a composite protobuf message or struct (`OSRPM`, `Servo`, `Dolos`, `Chameleon`), you MUST enumerate all sibling fields defined on that struct.
    *   Classify each sibling field by data type (`string`, `int32`, `repeated string`, `enum`) and editability.
    *   If a field is a protobuf enum, explicitly note its generated enum type name (e.g., `OSRPM_Type`).
    *   Ensure all editable sibling fields are documented so the struct is edited cohesively.

## Step 2: Search Exact UFS Controller Functions & Autocomplete Rules

Locate live logic by searching for these exact function signatures in `infra/unifiedfleet`:
1.  **DUT Controller (`app/controller/dut.go`)**:
    *   `func UpdateDUT(ctx context.Context, machinelse *ufspb.MachineLSE, mask *field_mask.FieldMask)`
    *   `func validateUpdateMachineLSEDUTMask(ctx context.Context, mask *field_mask.FieldMask, oldMachinelse, machinelse *ufspb.MachineLSE, machine *ufspb.Machine)`
    *   `func processUpdateMachineLSEUpdateMask(ctx context.Context, oldMachineLse, newMachineLse *ufspb.MachineLSE, mask *field_mask.FieldMask)`
    *   `func processUpdateMachineLSEDUTMask(oldDut, newDut *chromeosLab.DeviceUnderTest, path string)`
    *   **Peripheral Helpers**: `processUpdateMachineLSEServoMask`, `processUpdateMachineLSERPMMask`, `processUpdateMachineLSEDolosMask`
2.  **Labstation Controller (`app/controller/labstation.go`)**:
    *   `func UpdateLabstation(ctx context.Context, machinelse *ufspb.MachineLSE, mask *field_mask.FieldMask)`
    *   `func validateUpdateLabstationMask(ctx context.Context, mask *field_mask.FieldMask, oldMachinelse, machinelse *ufspb.MachineLSE, machine *ufspb.Machine)`
    *   `func processUpdateLabstationMask(ctx context.Context, oldMachineLSE, newMachineLSE *ufspb.MachineLSE, mask *field_mask.FieldMask)`
    *   `func validateUpdateLabstation(ctx context.Context, oldLabstation, labstation *ufspb.MachineLSE, mask *field_mask.FieldMask)`
3.  **General LSE & Machine Validation (`app/controller/machine_lse.go`, `machine.go`)**:
    *   `func validateUpdateMachineLSE(...)`
    *   `func validateUpdateMachine(...)`

**Autocomplete Relevance Check**: Inspect UFS validation logic to determine if the field enforces membership in an existing inventory entity or device dimension set (e.g., must be a known existing zone, pool, or hostname).
*   If membership is enforced or options can be enumerated from device dimensions, record `Use Autocomplete: true (dimension key: e.g., 'label-zone' or 'label-pool')`.
*   If UFS accepts arbitrary unconstrained strings/numbers, record `Use Autocomplete: false (FormTextField)`.

## Step 3: Extract Introducing Gerrit CL & Buganizer ID

In `infra/unifiedfleet`:
1.  Run `git log -S "<field_name>" --oneline -- api/v1/models/` to find the commit that introduced the protobuf field.
2.  Run `git show <sha>` on the introducing commit to inspect the full commit message.
3.  Extract:
    *   **Gerrit CL URL** from the `Reviewed-on: https://chromium-review.googlesource.com/...` footer.
    *   **Buganizer Ticket ID** from the `Bug: b/...` or `BUG=b:...` footer.

## Step 4: Search Shivas CLI Flags & Deploy Rules

In `infra/cmd/shivas`:
1.  Search `internal/ufs/subcmds/dut/update_dut.go` for CLI flag registrations:
    *   Look for `c.Flags.StringVar(...)` matching the field name.
    *   Locate the field's path constant (e.g., `rpmHostPath = "dut.rpm.host"`).
    *   **Irregular Flag Name Exception**: Do not assume flags mechanically follow `-<field>-<subfield>`. Check `update_dut.go` explicitly for irregular flag names (e.g., `-rpm` is used for `powerunit_name` rather than `-rpm-host`) and record all flags accurately in the context block.
2.  **Inheritance Fallback Search**: If the flag is not registered under `update_dut.go`, search `internal/ufs/subcmds/machine/update_machine.go` and `internal/ufs/subcmds/asset/update_asset.go` for CLI flag registrations.
3.  Search for `var partialUpdateDeployPaths` in `update_dut.go`.
4.  Determine whether updating this field triggers a hardware verification deploy task (`requiresRedeploy: true` if present in `partialUpdateDeployPaths`, `false` otherwise).

## Step 5: Check Fleet Console Backend State

In `infra/fleetconsole`:
1.  Search `api/fleetconsolerpc/chromeos.proto` for `message DeviceConfigEdits` to check if the field is already exposed.
2.  Search `api/fleetconsolerpc/paths.go` for existing allowed field path constants (`PathPools`, `PathServoHostname`, etc.).
3.  Search `internal/core/chromeos/validate.go` for `ValidateUpdateMask` and `ValidateConfigEdits` to check existing validation rules.
4.  Search `internal/infra/ext/ufs/map_update_machinelse.go` for `MapUpdateMachineLSERequest`, `mapUpdateDUT`, and `mapUpdateLabstation` to check existing UFS mapping logic.

## Step 6: Output Field Technical Context Block

Output the verified findings in chat using the exact markdown block format below so the orchestrator or calling agent can copy and paste it across execution turns. For composite structs, list all sub-field mask paths separately for DUT and Labstation targets:

```field-technical-context
Field Name: <Name>
Requires Redeploy: <true | false>
Use Autocomplete: <true (dimension key: e.g., 'label-zone') | false (FormTextField)>
Data Type: <string | int32 | repeated string | enum (EnumName)>
Sibling Fields: <List all sibling fields on target struct, their types, and editability>
UFS Proto Message: <e.g., DeviceUnderTest.hive in device.proto or Location.zone in location.proto>
DUT Mask Paths: <list of mask paths for DUT, e.g., dut.rpm.host, dut.rpm.outlet, dut.rpm.type>
Labstation Mask Paths: <list of mask paths for Labstation or N/A, e.g., labstation.rpm.host, labstation.rpm.outlet>
UFS Mask Validator: <e.g., validateUpdateMachineLSEDUTMask>
UFS Mask Processor: <e.g., processUpdateMachineLSERPMMask>
Introducing Commit SHA: <SHA>
Gerrit CL URL: <https://chromium-review.googlesource.com/...>
Buganizer Ticket ID: <b/...>
Shivas CLI Flag: <e.g., -hive or -rpm, -rpm-outlet>
Fleet Console Proto Field: <e.g., DeviceConfigEdits.hive or missing>
Fleet Console Path Constant: <e.g., PathHive = "hive" or missing>
```

