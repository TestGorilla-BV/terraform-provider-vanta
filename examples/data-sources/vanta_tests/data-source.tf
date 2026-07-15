# Tests that currently need attention.
data "vanta_tests" "failing" {
  status_filter = "NEEDS_ATTENTION"
}

output "failing_test_names" {
  value = [for t in data.vanta_tests.failing.tests : t.name]
}
