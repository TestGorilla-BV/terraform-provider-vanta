resource "vanta_vendor" "acme" {
  name                  = "Acme Inc"
  website_url           = "https://www.acme.com"
  services_provided     = "Cloud infrastructure"
  account_manager_name  = "Jane Doe"
  account_manager_email = "jane@acme.com"
  status                = "MANAGED"
  inherent_risk_level   = "HIGH"
  vendor_headquarters   = "USA"
  authentication_method = "GOOGLE_WORKSPACE"

  contract_amount = {
    amount   = 9375
    currency = "USD"
  }

  # Adopt a same-named vendor that already exists in Vanta instead of creating
  # a duplicate (handy for bulk-managing pre-existing vendors without import
  # blocks), and archive rather than delete on destroy.
  adopt_existing     = true
  archive_on_destroy = true
}
