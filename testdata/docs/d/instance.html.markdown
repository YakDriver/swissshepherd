---
subcategory: "Test"
page_title: "Test: test_instance"
description: |-
  Provides details about a Test Instance.
---

# Data Source: test_instance

Provides details about a Test Instance.

## Example Usage

```terraform
data "test_instance" "example" {
  name = "example"
}
```

## Argument Reference

This data source supports the following arguments:

* `name` - (Required) Name of the instance.

## Attribute Reference

This data source exports the following attributes in addition to the arguments above:

* `arn` - ARN of the instance.
