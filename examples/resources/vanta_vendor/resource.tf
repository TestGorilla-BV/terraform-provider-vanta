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

  # Archive the vendor in Vanta on destroy instead of deleting it.
  archive_on_destroy = true
}
