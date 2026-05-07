---
# REQUIRED: Keyword-rich description for on-demand discovery by the agent.
# Use the "Use when..." pattern with specific trigger words.
# Example: "Use when writing unit tests for Go controllers. Covers testify patterns, mocks, and time utilities."
description: "Use when modifying code related to RolloutPlugin. Covers plugin interface, controller logic."
---

# Instruction Title

* Preserve existing functionality when modifying code related to RolloutPlugin.
* Align coding style and patterns with the existing codebase without breaking existing functionality of RolloutPlugin.
* RolloutPlugin use a controller runtime controller which will run in parallel with the main rollout controller which does not use controller-runtime. Both will co-exist in the same process and share the same manager. The RolloutPlugin controller will be responsible for reconciling RolloutPlugin resources and executing the plugin logic, while the main rollout controller will continue to handle Rollout resources and their associated logic.
* The current functionality of RolloutPlugin is focused only for basic canary use case.
* Exclude writing tests for the RolloutPlugin related code, as it will be covered in a separate instruction file focused on testing.
* Update existing documentation to reflect any changes made to the RolloutPlugin code.
* Only add concise comments to explain complex logic or decisions related to RolloutPlugin, without over-commenting or adding redundant comments.
* Reuse existing utility functions and patterns where applicable
* Preserve all functionality mentioned in rolloutplugin-user-guide.md under Controlling a Rollout section(doc is a bit outdated w.r.t to the CRD fields/status, the implementation might/can have differed but functionality should be preserved)
