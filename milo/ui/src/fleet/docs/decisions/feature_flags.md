# Fleet Console Feature Flags Architecture

Fleet Console (`src/fleet`) adopts the central LUCI Milo UI Feature Flag Framework.

For full framework capabilities, evaluation rules, and dev tools controls, see 📖 [LUCI Milo UI Feature Flags Guide](../../../../docs/guides/using_feature_flags.md).

---

## Principles & Rules

1. **Location & Ownership**: All Fleet Console feature flags are defined centrally in [`src/fleet/features.ts`](../../features.ts).
2. **Environment Isolation**:
   - **Dev-to-Prod Rollout**: Use `percentage: { dev: 100, prod: 0 }` to enable features by default in `dev` while guaranteeing zero risk in `prod`.
   - **Dev-Only Flags**: Use `allowedEnvironments: ['dev']` for experimental prototypes and unreleased pre-prod features that users must not enable in production.
3. **Callsites**: Use `useFeatureFlag` inside React components and `getFeatureFlagValue` in non-hook contexts (e.g. route tables).
4. **Verification**: Safety tests in [`src/fleet/features.test.ts`](../../features.test.ts) assert `dev` vs. `prod` behavior.
5. **Cleanup**: Remove flag definitions and conditional branches when features reach 100% rollout.
