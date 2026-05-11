---
subcategory: "Test"
page_title: "Test: test_instance"
description: |-
  Manages a Test Instance.
---

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

* `arn` - (Required) ARN of the instance.
* `name` - (Required) Name of the instance.

## Attribute Reference

This resource exports no additional attributes.
