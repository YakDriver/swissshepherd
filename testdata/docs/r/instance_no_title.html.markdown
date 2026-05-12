---
subcategory: "Test"
page_title: "Test: test_instance"
description: |-
  Manages a Test Instance.
---
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

## Example Usage

Title fixture: the file has no level-1 heading at all — a common pre-migration
state for docs generated from templates. The title rule must flag the missing
section.

```terraform
resource "test_instance" "example" {
  name = "example"
}
```

## Argument Reference

* `name` - (Required) Name.

## Attribute Reference

This resource exports no additional attributes.
