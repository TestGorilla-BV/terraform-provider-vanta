resource "vanta_vendor" "acme" {
  name = "Acme Inc"
}

resource "vanta_vendor_finding" "pentest" {
  vendor_id   = vanta_vendor.acme.id
  content     = "Vendor has not performed a penetration test in the past 15 months."
  risk_status = "REMEDIATE"

  remediation = {
    state             = "OPEN"
    requirement_notes = "Request an updated penetration test report."
  }
}
