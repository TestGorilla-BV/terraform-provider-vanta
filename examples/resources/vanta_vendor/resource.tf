resource "vanta_vendor" "acme" {
  name                = "Acme Inc"
  website_url         = "https://www.acme.com"
  services_provided   = "Cloud infrastructure"
  account_manager_name  = "Jane Doe"
  account_manager_email = "jane@acme.com"
  status              = "MANAGED"
  inherent_risk_level = "HIGH"
  vendor_headquarters = "USA"
}
