---
subcategory: "Test"
page_title: "Test: test_instance"
description: |-
  Manages a Test Instance.
---
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

# Resource: test_instance

Title fixture: an inline code block sits between the H1 and the first H2.
The title rule must flag it — example code belongs under ## Example Usage.

```terraform
resource "test_instance" "misplaced" {
  name = "should-not-be-in-title-section"
}
```

## Example Usage

```terraform
resource "test_instance" "example" {
  name = "example"
}
```

## Argument Reference

* `name` - (Required) Name.

## Attribute Reference

This resource exports no additional attributes.
