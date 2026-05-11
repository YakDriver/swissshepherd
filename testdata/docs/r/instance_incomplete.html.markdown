---
subcategory: "Test"
page_title: "Test: test_instance"
description: |-
  Manages a Test Instance.
---
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

# Resource: test_instance

Manages a Test Instance.

## Example Usage

```terraform
resource "test_instance" "example" {
  name = "example"
}
```

## Argument Reference

The following arguments are required:

* `name` - (Required) Name of the instance.

The following arguments are optional:

* `tags` - (Optional) Map of tags to assign to the resource.

## Attribute Reference

This resource exports the following attributes in addition to the arguments above:

* `arn` - ARN of the instance.
