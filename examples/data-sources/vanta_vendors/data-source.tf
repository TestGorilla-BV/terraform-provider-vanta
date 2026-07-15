# All managed vendors.
data "vanta_vendors" "managed" {
  status_matches_any = ["MANAGED"]
}

output "managed_vendor_names" {
  value = [for v in data.vanta_vendors.managed.vendors : v.name]
}
