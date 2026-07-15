# Current employees.
data "vanta_people" "current" {
  employment_status = "CURRENT"
}

output "current_emails" {
  value = [for p in data.vanta_people.current.people : p.email]
}
