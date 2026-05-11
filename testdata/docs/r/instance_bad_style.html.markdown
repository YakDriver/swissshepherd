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

* `name` - (Required) The name of the instance.
* `description` - (Optional) A description of the instance.
* `mode` - (Optional) Specifies the mode to use.
* `type` - (Optional) Indicates the type of instance.

## Attribute Reference

This resource exports the following attributes in addition to the arguments above:

* `arn` - An ARN identifying the instance.
