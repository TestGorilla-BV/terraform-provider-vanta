terraform {
  required_providers {
    vanta = {
      source = "TestGorilla-BV/vanta"
    }
  }
}

variable "vanta_client_id" {
  type = string
}

variable "vanta_client_secret" {
  type      = string
  sensitive = true
}

provider "vanta" {
  client_id     = var.vanta_client_id
  client_secret = var.vanta_client_secret
  region        = "us" # or "gov" for FedRAMP
}
