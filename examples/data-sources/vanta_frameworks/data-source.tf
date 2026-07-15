data "vanta_frameworks" "all" {}

output "framework_progress" {
  value = {
    for f in data.vanta_frameworks.all.frameworks :
    f.shorthand_name => "${f.num_controls_completed}/${f.num_controls_total} controls"
  }
}
