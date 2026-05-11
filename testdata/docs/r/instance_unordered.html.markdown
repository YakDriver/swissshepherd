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
* `description` - (Optional) Description of the instance.
* `arn_prefix` - (Optional) Prefix for the ARN.

## Attribute Reference

This resource exports the following attributes in addition to the arguments above:

* `zebra` - Zebra attribute.
* `arn` - ARN of the instance.
