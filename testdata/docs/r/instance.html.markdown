---
subcategory: "Test"
page_title: "Test: test_instance"
description: |-
  Manages a Test Instance.
---

# Resource: test_instance

Manages a Test Instance.

## Example Usage

### Basic Usage

```terraform
resource "test_instance" "example" {
  name = "example"

  network {
    subnet_id = "subnet-123"
  }
}
```

## Argument Reference

The following arguments are required:

* `name` - (Required) Name of the instance.

The following arguments are optional:

* `description` - (Optional) Description of the instance.
* `network` - (Optional) Network configuration. See [`network`](#network-block) below for details.
* `tags` - (Optional) Map of tags to assign to the resource.

### `network` Block

The `network` block supports the following arguments:

* `security_groups` - (Optional) List of security group IDs.
* `subnet_id` - (Required) ID of the subnet.

## Attribute Reference

This resource exports the following attributes in addition to the arguments above:

* `arn` - ARN of the instance.
* `network[*].private_ip` - Private IP address.

## Timeouts

* `create` - (Default `30m`)
* `delete` - (Default `30m`)

## Import

In Terraform v1.5.0 and later, use an [`import` block](https://developer.hashicorp.com/terraform/language/import) to import Test Instances using the `id`. For example:

```terraform
import {
  to = test_instance.example
  id = "i-1234567890"
}
```
