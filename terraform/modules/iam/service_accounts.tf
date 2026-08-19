variable "project_id" { type = string }

resource "google_service_account" "terraform" {
  account_id   = "cnip-terraform"
  display_name = "Terraform apply identity"
}

resource "google_service_account" "gke_node" {
  account_id   = "cnip-gke-node"
  display_name = "GKE node default identity"
}

resource "google_service_account" "platform_controller" {
  account_id   = "cnip-controller"
  display_name = "Platform controller workload identity"
}
