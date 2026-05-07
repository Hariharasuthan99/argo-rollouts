---
description: Use this instruction when writing/modifying tests related to RolloutPlugin.
---

# Instruction Title

* RolloutPlugin for now is only intedended to be used for basic canary use case, so the test cases should focus on that use case and cover all the edge cases related to it.
* The test cases should cover all fields in the RolloutPlugin CRD
* The test cases should cover all the conditions and status updates related to RolloutPlugin.
* The test cases should test all the functionality of RolloutPlugin.
* As much as possible, use inline YAML like Rollout tests, don't create separate YAML files for each test case.