
variable "required" {
}

variable "optional" {
  default = ["optional var default"]
}

variable "not_set" {
}

locals {
  foo       = "local foo"
  undefined = "(must be artificially excluded from state in tests)"
}

resource "test" "single" {
  arg = "hello from single resource"

  provisioner "foo" {
    arg = "hello from provisioner"
  }
}

resource "test" "counted_one" {
  arg = "hello from count = 1 resource"
  count = 1
}

resource "test" "counted_two" {
  arg = "hello from count = 2 resource"
  count = 2
}

data "test" "single" {
  arg = "hello from single data source"
}

module "child" {
  source = "./child"
}
