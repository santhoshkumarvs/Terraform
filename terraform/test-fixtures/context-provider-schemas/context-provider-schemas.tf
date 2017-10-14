
provider "foo" {

}

provider "bar" {

}

resource "bar_bar" "n" {

}

resource "foo_bar" "n" {
  provider = "bar"
}

data "baz_bar" "n" {

}

// this provider doesn't support schema at all
provider "old" {

}
