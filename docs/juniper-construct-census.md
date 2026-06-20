# Juniper Junos 25.4R1 construct census

What the Juniper corpus actually contains, censused once as the design reference for the owned fixture catalog. The corpus itself is not a dependency.

| Construct | Count | Notes |
|---|---|---|
| module | 2657 | Top-level namespace containers. native: 2507, ietf: 10, openconfig: 140 |
| submodule | 38 | Submodule definitions. native: 0, ietf: 0, openconfig: 38. Only in openconfig modular designs. |
| import | 2645 | External module imports. native: 2461, ietf: 7, openconfig: 177. Heavily used in native Junos models. |
| include | 33 | Submodule includes. native: 0, ietf: 0, openconfig: 33. Used exclusively with openconfig submodules. |
| belongs-to | 38 | Submodule parent declaration. native: 0, ietf: 0, openconfig: 38. Paired 1:1 with submodules. |
| container | 2439 | Data tree containers. native: 2316, ietf: 4, openconfig: 119. Core YANG data modeling construct. |
| list | 2048 | List instances. native: 1956, ietf: 3, openconfig: 89. Widely used for collections. |
| leaf | 2590 | Leaf nodes (scalars). native: 2453, ietf: 4, openconfig: 133. Ubiquitous data nodes. |
| leaf-list | 882 | List of scalar values. native: 838, ietf: 2, openconfig: 42. Less common than leaves/containers. |
| choice | 1839 | Choice constructs. native: 1836, ietf: 2, openconfig: 1. Heavily skewed to native models. |
| case | 1822 | Case branches of choice. native: 1817, ietf: 1, openconfig: 4. Nearly 1:1 with choices. |
| uses | 2467 | Grouping reuse. native: 2330, ietf: 0, openconfig: 137. Very common for code reuse. |
| grouping | 2615 | Reusable groups. native: 2475, ietf: 1, openconfig: 139. Heavy use in both native and openconfig. |
| augment | 890 | Tree augmentations. native: 830, ietf: 0, openconfig: 60. Significant in native Junos extensions. |
| deviation | 3 | Deviation statements. native: 0, ietf: 2, openconfig: 1. Extremely rare; used for vendor/compliance variance. |
| deviate | 3 | Deviation payload (not-supported/replace/etc). native: 0, ietf: 2, openconfig: 1. Paired with deviation statements. |
| rpc | 1585 | RPC operations. native: 1584, ietf: 1, openconfig: 0. Heavily native-centric; openconfig uses data models, not RPCs. |
| action | 5 | YANG 1.1 RPC actions within data tree. native: 0, ietf: 0, openconfig: 5. Extremely rare; newer RFC7950 feature. |
| notification | 1 | Notification events. native: 1, ietf: 0, openconfig: 0. Extremely rare in this corpus. |
| identity | 57 | Abstract type identities. native: 13, ietf: 3, openconfig: 41. Used for enumeration-like extensible types. |
| typedef | 85 | Type definitions. native: 34, ietf: 6, openconfig: 45. More prevalent in openconfig. |
| feature | 2 | Feature conditional flags. native: 0, ietf: 2, openconfig: 0. Extremely rare; mostly IETF usage. |
| if-feature | 2 | Feature conditionals (statements). native: 0, ietf: 2, openconfig: 0. Paired with feature; minimal corpus coverage. |
| extension | 9 | Custom extensions. native: 5, ietf: 3, openconfig: 1. Openconfig defines openconfig-* extensions. |
| anydata | 0 | YANG 1.1 untyped data container. native: 0, ietf: 0, openconfig: 0. Not present in this 25.4 corpus. |
| anyxml | 325 | Untyped XML data (legacy). native: 324, ietf: 1, openconfig: 0. Present in netconf operations; native-heavy. |
| refine | 2 | Refine grouping members. native: 0, ietf: 0, openconfig: 2. Structural refinement; very rare. |
| when | 51 | Conditional presence. native: 9, ietf: 1, openconfig: 41. Strong openconfig usage for feature-gating. |
| must | 13 | Constraint assertions. native: 1, ietf: 0, openconfig: 12. Openconfig-dominant for validation logic. |
| key | 756 | List key specification. native: 662, ietf: 3, openconfig: 91. Required for list uniqueness. |
| unique | 7 | Uniqueness constraints. native: 3, ietf: 1, openconfig: 3. Rare; most lists use keys instead. |
| presence | 238 | Container presence indication. native: 238, ietf: 0, openconfig: 0. Exclusively native; junos-specific semantic. |
| mandatory | 731 | Mandatory node flag. native: 716, ietf: 4, openconfig: 11. Heavily native. |
| default | 1258 | Default values. native: 1180, ietf: 4, openconfig: 74. Very common; primarily native. |
| config | 172 | Config/state split flag. native: 62, ietf: 2, openconfig: 108. Openconfig-dominant for explicit separation. |
| ordered-by | 387 | List ordering mode (user/system). native: 382, ietf: 1, openconfig: 4. Heavy native usage. |
| min-elements | 0 | Minimum element count. native: 0, ietf: 0, openconfig: 0. Not present in this 25.4 corpus. |
| max-elements | 200 | Maximum element count. native: 199, ietf: 0, openconfig: 1. Almost exclusively native. |
| yang-version | 759 | YANG version identifier. native: 614, ietf: 2, openconfig: 143. Both 1.0 and 1.1 present; version distribution not analyzed. |
