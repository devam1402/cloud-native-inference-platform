variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "repo_id" {
  type    = string
  default = "cnip-images"
}

resource "google_artifact_registry_repository" "images" {
  project       = var.project_id
  location      = var.region
  repository_id = var.repo_id
  format        = "DOCKER"
}
