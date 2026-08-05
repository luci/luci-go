---
name: add-editable-field
description: Orchestrates adding a new editable inventory field to Fleet Console across three distinct CLs (proto, backend handler, frontend UI) using a universal Field Archetype Matrix. Enforces copying both the Progress Checklist and the Field Technical Context block across all execution turns. Use when you need to make a UFS inventory field editable in ChromeOS Device Details cards.
---

# Add Editable Field Orchestrator Skill

## Quick Start

When invoked to make an inventory field editable in Fleet Console (e.g., `"hive"`, `"rpm"`, `"zone"`, `"serial"`), orchestrate the end-to-end development process across UFS research, Fleet Console backend RPC integration, and Milo UI card forms by breaking the work into **three separate CLs** to keep diff sizes manageable and reviews focused.

> [!IMPORTANT]
> **Primary Source Precedence Rule**: The `Field Technical Context` block generated in Step 1 and the **Field Archetype Matrix** take absolute precedence over historical session logs, transcripts, or prior conversation memory. NEVER copy UI component types or code snippets from past transcripts if they contradict the classified archetype.
>
> **Dual Copy-Paste Pattern**: At the start of executing this skill, you MUST copy the **Progress Checklist** below into your very next response. After completing Step 1, you MUST copy **BOTH** the Progress Checklist and the **Field Technical Context** block into **EVERY subsequent response** across the entire task execution so all verified symbol names, masks, and bug IDs remain visible in the chat history.

```
Progress:
- [ ] Step 1: Execute UFS field research and classify field using Field Archetype Matrix
- [ ] Step 2: Dispatch CL 1 subagent for Fleet Console Proto & Path Constants
- [ ] Step 3: Dispatch CL 2 subagent for Fleet Console Backend Handler & Validation
- [ ] Step 4: Dispatch CL 3 subagent for Milo UI Frontend Card & Engine Integration
- [ ] Step 5: Verify all CLs and format commit messages with Bug: b/...
```

## Step 1: Execute UFS Field Research & Archetype Classification

1.  Invoke a `self` subagent or execute [ufs-field-researcher](../ufs-field-researcher/SKILL.md) to investigate the target field across `infra/unifiedfleet`, `infra/cmd/shivas`, and `infra/fleetconsole`.
2.  **Sibling Field Enumeration Check**: If the target field belongs to a composite UFS protobuf struct (`OSRPM`, `Servo`, `Dolos`), verify that all editable sibling fields on that struct are enumerated and included so the struct is edited cohesively.
3.  **Archetype Classification**: Consult the **Field Archetype Matrix** at the bottom of this document to classify the researched field into one of four archetypes (**Archetype A**: Unconstrained Scalar, **Archetype B**: Dimension Autocomplete, **Archetype C**: Protobuf Enum, **Archetype D**: Composite Struct).
4.  Copy the output `field-technical-context` block into all subsequent chat responses alongside the Progress Checklist.
5.  **Primary Source Precedence**: Verify that the selected archetype strictly governs subsequent steps. Do not substitute historical transcript examples for Archetype Matrix rules.

## Step 2: CL 1 — Fleet Console Protobuf & Paths (`infra/fleetconsole`)

Invoke a subagent targeting the `infra/fleetconsole` repository to create a dedicated protobuf and path constants CL. Keep handlers out of this CL to avoid bloating the change. Instruct the subagent to:
1.  **Proto Naming Convention & Struct Flattening Rule**: Use simple, homogeneous, developer-friendly field names (`host`, `outlet`, `type`, `port`, `serial`) for composite structs (`RPMConfigEdits`, `ServoConfigEdits`) rather than verbose database column names like `powerunit_name` or `powerunit_outlet`. Ensure all editable sibling fields of a struct are added together.
2.  **Proto Definition**: Add the field (or composite struct) to `message DeviceConfigEdits` in `api/fleetconsolerpc/chromeos.proto`.
3.  **Proto Generation**: Run `PATH=$PATH:$HOME/projects/infra/infra/cipd/bin:$HOME/projects/infra/infra/go/bin go generate ./api/fleetconsolerpc/...` to regenerate bindings (absolute paths are required by Go 1.22+ to prevent `exec: cannot run executable found relative to current directory` errors).
4.  **Path Constants**: Define allowed path constants in `api/fleetconsolerpc/paths.go` (e.g., `PathHive = "hive"`, `PathRPMHost = "rpm.host"`).
5.  **CL Preparation**: Ask the user for the Buganizer ticket for adding the field, format the commit footer as `Bug: b/<ticket_id>` per the Gerrit template, and upload using [prepare-cl](../prepare-cl/SKILL.md).

## Step 3: CL 2 — Fleet Console Backend Handler & Validation (`infra/fleetconsole`)

Invoke a second subagent targeting the `infra/fleetconsole` repository to implement the handler and validation logic in a separate CL. Instruct the subagent to:
1.  **Validation**: Update `ValidateUpdateMask` and `ValidateConfigEdits` in `internal/core/chromeos/validate.go`.
2.  **UFS Mapping**: Update `MapUpdateMachineLSERequest` (`mapUpdateDUT` / `mapUpdateLabstation`) in `internal/infra/ext/ufs/map_update_machinelse.go` to map the Fleet Console edit to the verified UFS mask path.
3.  **Unit Tests**: Add new unit tests if necessary in `validate_test.go` and `map_update_machinelse_test.go` (`go test ./...`).
4.  **Integration Tests**: Run backend integration tests using the `integration-testing-backend` skill (e.g., `make test PKG=./test/integration/...`).
5.  **CL Preparation**: Format commit message footer with `Bug: b/<ticket_id>` and upload using [prepare-cl](../prepare-cl/SKILL.md).

## Step 4: CL 3 — Milo UI Frontend Card & Engine Integration (`go.chromium.org/luci/milo/ui/src/fleet`)

Invoke a subagent targeting the `go.chromium.org/luci/milo/ui/src/fleet` repository to implement the UI card inputs and Shivas command preview. Instruct the subagent to:
1.  **Prerequisite Proto Generation**: Frontend TypeScript compilation and type-checking requires local generated proto bindings from CL 1. Ensure that from the monorepo UI root (`go.chromium.org/luci/milo/ui/`), `PATH=$PATH:$HOME/projects/infra/infra/cipd/bin bash src/fleet/gen_ts_proto.sh` is executed after CL 1 proto changes are present in the workspace.
2.  **Editing Engine Modification (`inventory_editing_utils.ts`)**: Modify the four active imperative integration points in `pages/device_details_page/chromeos/utils/inventory_editing_utils.ts`:
    *   **Path Constants**: Define explicit property dictionaries (e.g., `export const <FIELD>_PATHS = { ... }`) for DUTs and Labstations.
    *   **`getEditableFields(isLabstation)`**: Push `FieldConfig` entries (`label`, `path`, `editPath`, `type`, `requiresRedeploy`). **CRITICAL DEPLOY RULE**: Only set `requiresRedeploy: true` if `ufs-field-researcher` determined in `Field Technical Context` that updating this field triggers a Shivas hardware deploy task (`Requires Redeploy: true`, per `partialUpdateDeployPaths`). Otherwise, omit or set `false`.
    *   **`translateDiffToEdits(original, updated)`**: Map UI form diffs into the Fleet Console RPC `DeviceConfigEdits` payload and mask paths, handling zero-clearing or multi-field dependency resets.
    *   **`generateShivasCommands(...)`**: Append Shivas CLI flags for changed fields, checking `update_dut.go` for irregular flag names (e.g., `-rpm` for host instead of `-rpm-host`).
3.  **UI Card Integration**: In `pages/device_details_page/chromeos/components/cards/<CardName>.tsx`, wrap layout in `<CardForm cardId="..." title="...">` and select the input component matching your Archetype from the Pattern Catalog below (`FormTextField` or `FormAutocompleteField`). For cards that support both read-only display and form-based editing (e.g., `RPMCard.tsx`), use `const form = useOptionalInventoryForm();` so the component renders read-only outside form contexts.
    *   **Mandatory Autocomplete Prompt Guardrail**: When dispatching the CL 3 subagent for a field where `Use Autocomplete: true` in the `Field Technical Context`, the orchestrator prompt MUST explicitly instruct the subagent to use `<FormAutocompleteField multiple={false}>` and explicitly BAN `<FormTextField>` for that field.
4.  **Parent Call-Site Compatibility (`chromeos_inventory_data.tsx`)**: When a card uses `useOptionalInventoryForm()`, preserve existing call-sites in `chromeos_inventory_data.tsx` (e.g., `<CardName prop={...} editable={false} />`) so fallback read-only compatibility is maintained outside `<InventoryFormProvider>`.
5.  **Testing**: Add unit tests in `inventory_editing_utils.test.ts` and `<CardName>.test.tsx` and verify with `npm test`, `npm run lint`, and `npm run type-check`.
6.  **CL Preparation**: Format commit message footer with `Bug: b/<ticket_id>` and upload using [prepare-cl](../prepare-cl/SKILL.md).
7.  **Subagent Archetype Self-Validation**: Instruct the CL 3 subagent to independently inspect the `Field Technical Context` block. If `Use Autocomplete: true` is set, the subagent MUST verify that the UI component used is `<FormAutocompleteField>` (Archetype B/C) and MUST reject and override any parent prompt instruction requesting `<FormTextField>`.

## Step 5: Verification Rules & Quality Gates

> [!IMPORTANT]
> **Mandatory Gate Rule**: No subagent may call [prepare-cl](../prepare-cl/SKILL.md) or declare its task complete until every command for its respective CL layer runs and passes with zero errors.

| Layer / CL | Failure Modes | Verification Command / Tool | Pass Criteria |
| :--- | :--- | :--- | :--- |
| **CL 1: Proto & Paths (`infra/fleetconsole`)** | • Missing field tags<br>• Mismatched field names between `.proto` and `paths.go`<br>• Uncommitted `.pb.go` drift | `PATH=$PATH:$HOME/projects/infra/infra/cipd/bin:$HOME/projects/infra/infra/go/bin go generate ./api/fleetconsolerpc/...`<br>`go vet ./api/fleetconsolerpc/...`<br>`git status --short` | Zero compilation errors; zero untracked or modified generated files after generation. |
| **CL 2: Backend Handler & Mapper (`infra/fleetconsole`)** | • UFS Mask Path mismatch (mapped path != UFS controller)<br>• Uncaught validation regressions<br>• AlloyDB schema/SQL errors | `go test ./internal/core/chromeos/...`<br>`go test ./internal/infra/ext/ufs/...`<br>`git grep -n '"<ufs_mask_path>"'` (in UFS repo)<br>`PATH=$PATH:$HOME/projects/depot_tools:$HOME/depot_tools make test PKG=./test/integration/...` | `map_update_machinelse_test.go` asserts `UpdateMask.Paths` matches literal `UFS Mask Path` from `Field Technical Context` and verified in UFS controller; integration tests pass. |
| **CL 3: Milo UI Frontend (`go.chromium.org/luci/milo/ui/src/fleet`)** | • RPC mask path mismatch (`editPath` != constant in `paths.go`)<br>• Broken diff translation (`translateDiffToEdits`)<br>• Wrong deploy task warning (`requiresRedeploy`)<br>• Wrong form component used (`FormTextField` used when `Use Autocomplete: true`)<br>• TypeScript/lint errors | `PATH=$PATH:$HOME/projects/depot_tools ../../../../../../../env.py npm test -- .../inventory_editing_utils.test.ts`<br>`PATH=$PATH:$HOME/projects/depot_tools ../../../../../../../env.py npm test -- .../<CardName>.test.tsx`<br>`git grep -n '"<editPath>"'` (in `paths.go`)<br>`git diff \| grep -E '<FormAutocompleteField\|<FormTextField'`<br>`PATH=$PATH:$HOME/projects/depot_tools ../../../../../../../env.py npm run type-check`<br>`PATH=$PATH:$HOME/projects/depot_tools ../../../../../../../env.py npm run lint` | `inventory_editing_utils.test.ts` asserts `translateDiffToEdits` outputs `editPath` matching exact literal string from `paths.go`; when `Use Autocomplete: true`, `git diff` confirms `<FormAutocompleteField` is present and `<FormTextField` is absent; 100% pass on component rendering, zero TS/lint errors. |
| **Pre-CL Upload Gate (All CLs)** | • Submodule leakage<br>• Missing Buganizer ticket formatting | [preventing-workspace-leakage](../preventing-workspace-leakage/SKILL.md)<br>`git cl issue` | Clean root git status; commit footer formatted as `Bug: b/...`. |

### Contract Invariant Verification (Cross-CL Gates)
1.  **CL 2 ↔ UFS Mask Path Gate**: Verify that the literal string appended to `ufsPaths` in `mapUpdateDUT` / `mapUpdateLabstation` is an exact character-for-character match of the `UFS Mask Path` documented in the `Field Technical Context` block (verified against `infra/unifiedfleet/app/controller/`).
2.  **CL 3 ↔ CL 1 RPC Path Constant Gate**: Verify that the `editPath` property assigned in `getEditableFields(isLabstation)` in `inventory_editing_utils.ts` is an exact character-for-character match of the string constant value defined in `infra/fleetconsole/api/fleetconsolerpc/paths.go`.
3.  **CL 3 Archetype Component Verification Gate**: When `Use Autocomplete: true` in `Field Technical Context`, assert via `git diff` on `<CardName>.tsx` that `<FormAutocompleteField` is used and `<FormTextField` is not used for that field.

---

# Field Archetype Matrix & Pattern Catalog

Use the table below to select the exact frontend implementation pattern for your researched field based on its data type and inventory constraints.

| Archetype | Matching Criteria | UI Component | Engine Configuration (`inventory_editing_utils.ts`) | Reference Snippet |
| :--- | :--- | :--- | :--- | :--- |
| **Archetype A: Unconstrained Scalar** | `string` or `number` with `Use Autocomplete: false` (e.g., `hive`, `serial`, `port`) | `<FormTextField>` | Define `<FIELD>_PATHS`, push simple scalar to `getEditableFields`, map in `translateDiffToEdits` | See **Pattern A** |
| **Archetype B: Dimension Autocomplete** | `string` with `Use Autocomplete: true` (e.g., `zone`, `pool`, `hostname`) | `<FormAutocompleteField>` + `useDeviceDimensions` | Define `<FIELD>_PATHS`, push string to `getEditableFields`, map in `translateDiffToEdits` | See **Pattern B** |
| **Archetype C: Protobuf Enum** | `enum (<EnumName>)` (e.g., `rpm.type`, `power_state`) | `<FormAutocompleteField>` + token normalization | Convert numeric enum integer (0, 1, 2) to string token for display; map string token back to enum integer in `translateDiffToEdits` | See **Pattern C** |
| **Archetype D: Composite Struct** | Message with multiple sibling fields (e.g., `rpm`, `servo`, `dolos`) | Multiple card inputs grouped under `<CardForm>` | Export struct path constants, register sibling fields in `getEditableFields`, construct struct in `translateDiffToEdits` | See **Pattern D** |

### Pattern A: Unconstrained Scalar (`Hive` example)
```tsx
// 1. In inventory_editing_utils.ts:
export const HIVE_PATHS = {
  hive: 'chromeosMachineLse.deviceLse.dut.hive',
};

// In getEditableFields(isLabstation):
if (!isLabstation) {
  fields.push({
    label: 'Hive',
    path: HIVE_PATHS.hive,
    editPath: 'hive',
    type: 'string',
    requiresRedeploy: false,
  });
}

// 2. In HiveCard.tsx:
<CardForm cardId="hive" title="Hive">
  <Grid container spacing={2}>
    <FormTextField
      label="Hive"
      path={HIVE_PATHS.hive}
      gridSm={6}
    />
  </Grid>
</CardForm>
```

### Pattern B: Dimension-Validated Autocomplete (`Zone` example)
```tsx
// 1. In PhysicalLocationCard.tsx:
const dimensionsQuery = useDeviceDimensions({ platform: Platform.CHROMEOS });
const zoneOptions = dimensionsQuery.data?.labels?.['label-zone']?.values || [];

<CardForm cardId="location" title="Location">
  <Grid container spacing={2}>
    <FormAutocompleteField
      label="Zone"
      path={LOCATION_PATHS.zone}
      options={zoneOptions as string[]}
      multiple={false} // CRITICAL: defaults to true; required for single-valued fields
      gridSm={6}
    />
  </Grid>
</CardForm>
```

### Pattern C: Protobuf Enum (`RPM Type` example)
```tsx
// 1. In inventory_editing_utils.ts — Simple string-backed enums/types:
// For string-backed enum fields like 'RPM Type', production code maps strings directly without numeric integer conversion.
// If converting numeric enum integers (0, 1, 2), convert UI string token back to integer in translateDiffToEdits:
if (label === 'RPM Type' && typeof updatedVal === 'string') {
  const enumVal = oSRPM_TypeFromJSON(`TYPE_${updatedVal}`);
  edits.rpm = { ...edits.rpm, powerunitType: enumVal };
  paths.push('rpm.type');
}

// 2. In RPMCard.tsx — Strip enum prefix for autocomplete tokens and specify multiple={false}:
const rpmTypeOptions = useMemo(
  () =>
    Object.keys(OSRPM_Type)
      .filter((key) => isNaN(Number(key)) && key !== 'TYPE_UNKNOWN')
      .map((key) => key.replace('TYPE_', '')),
  [],
);

const paths = isLabstation ? RPM_PATHS.labstation : RPM_PATHS.dut;

<FormAutocompleteField
  label="RPM Type"
  path={paths.type}
  options={rpmTypeOptions}
  multiple={false} // CRITICAL: defaults to true; required for single-valued selection
  gridSm={6}
/>
```

### Pattern D: Composite Struct (`RPM` complete example)
```tsx
// 1. In inventory_editing_utils.ts — Exported paths & sibling registration:
export const RPM_PATHS = {
  dut: {
    host: 'chromeosMachineLse.deviceLse.dut.peripherals.rpm.powerunitName',
    outlet: 'chromeosMachineLse.deviceLse.dut.peripherals.rpm.powerunitOutlet',
    type: 'chromeosMachineLse.deviceLse.dut.peripherals.rpm.powerunitType',
  },
  labstation: {
    host: 'chromeosMachineLse.deviceLse.labstation.rpm.powerunitName',
    outlet: 'chromeosMachineLse.deviceLse.labstation.rpm.powerunitOutlet',
    type: 'chromeosMachineLse.deviceLse.labstation.rpm.powerunitType',
  },
};

// In getEditableFields(isLabstation):
const rpmPaths = isLabstation ? RPM_PATHS.labstation : RPM_PATHS.dut;
fields.push(
  {
    label: 'RPM Hostname',
    path: rpmPaths.host,
    editPath: 'rpm.host',
    type: 'string',
    requiresRedeploy: true, // Shivas flag -rpm triggers redeploy
  },
  {
    label: 'RPM Outlet',
    path: rpmPaths.outlet,
    editPath: 'rpm.outlet',
    type: 'string',
    requiresRedeploy: true, // Shivas flag -rpm-outlet triggers redeploy
  },
);

// 2. In RPMCard.tsx — Use useOptionalInventoryForm() for read-only fallback compatibility:
const form = useOptionalInventoryForm();
const paths = isLabstation ? RPM_PATHS.labstation : RPM_PATHS.dut;

<CardForm cardId="rpm" title="RPM">
  <Grid container spacing={2}>
    <FormTextField
      label="RPM Hostname"
      path={paths.host}
      gridSm={6}
    />
    <FormTextField
      label="RPM Outlet"
      path={paths.outlet}
      gridSm={6}
    />
  </Grid>
</CardForm>

// 3. In chromeos_inventory_data.tsx — Preserve read-only fallback call-site:
// Keep passing read-only props (<RPMCard rpm={rpm} editable={false} />) so read-only views outside form contexts function without error.
<RPMCard rpm={rpm} editable={false} />
```

### Mandatory Unit Test Pattern (`<CardName>.test.tsx`)
```tsx
// CRITICAL: Cards using unconditional useInventoryForm() (e.g., ServoHardwareCard) throw outside <InventoryFormProvider>.
// For those cards, wrap ALL test render calls in <InventoryFormProvider>:
render(
  <InventoryFormProvider originalLse={mockLse} draftLse={mockLse} editable={true}>
    <TargetCard />
  </InventoryFormProvider>
);

// Note: Cards using useOptionalInventoryForm() (e.g., RPMCard.tsx) can be tested in read-only mode without wrapping.
// Query for edit confirmation button in edit mode:
const confirmBtn = screen.getByRole('button', { name: 'Confirm' }); // Do NOT query for { name: 'Save' }
```

