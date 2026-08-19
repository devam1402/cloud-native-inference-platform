variable "project_id" {
  type = string
}

variable "cluster_name" {
  type = string
}

variable "location" {
  type = string
}

variable "name" {
  type = string
}

variable "machine_type" {
  type = string
}

variable "node_sa_email" {
  type = string
}

variable "min_nodes" {
  type    = number
  default = 1
}

variable "max_nodes" {
  type    = number
  default = 3
}

variable "disk_size_gb" {
  type    = number
  default = 50
}

resource "google_container_node_pool" "pool" {
  name     = var.name
  project  = var.project_id
  cluster  = var.cluster_name
  location = var.location

  autoscaling {
    min_node_count = var.min_nodes
    max_node_count = var.max_nodes
  }

  node_config {
    machine_type    = var.machine_type
    disk_type       = "pd-standard"
    disk_size_gb    = var.disk_size_gb
    service_account = var.node_sa_email
    oauth_scopes    = ["https://www.googleapis.com/auth/cloud-platform"]
    labels          = { "flowplane.dev/pool" = var.name }
  }

  management {
    auto_repair  = true
    auto_upgrade = true
  }
}
