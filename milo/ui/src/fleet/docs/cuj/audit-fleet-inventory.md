# CUJ: Audit Fleet Inventory and Ensure Accuracy

This Critical User Journey (CUJ) describes how users reconcile the logical representation of the fleet with the physical reality in the labs. This ensures that capacity planning is based on accurate data.

* **User:** Lab Manager
* **Goal:** I want to have an accurate picture of the hardware in the lab, so that I can plan capacity effectively.

## The Journey Steps

| Step | Task | Description / How it's served today |
| :--- | :--- | :--- |
| **S1** | **Compare records** | The user compares device lists across different systems (e.g., Swarming, Lab Management Systems, UFS, and Asset Management Systems) to find discrepancies. Served by the **Device List** and **Catalog** pages. |
| **S2** | **Identify discrepancies** | The user isolates hardware that exists physically but is invisible to schedulers (or vice-versa), or has mismatched metadata. |
| **S3** | **Update metadata** | The user edits device fields to align the systems. Served by direct metadata editing in the Fleet Console UI. |
| **S4** | **Decommission hardware** | The user triggers decommissioning for retired hardware to purge references. |

## Friction Points to Avoid
* Forcing users to physically walk the floor to verify database registration changes.
* Orphan assets created when auxiliary hardware references are lost.
* Tedious terminal commands or manual spreadsheet updates for basic inventory changes.
