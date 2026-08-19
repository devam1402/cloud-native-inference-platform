variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "cluster_name" {
  type = string
}

variable "network" {
  type = string
}

variable "subnetwork" {
  type = string
}

variable "node_sa_email" {
  type = string
}

resource "google_container_cluster" "primary" {
  name     = var.cluster_name
  project  = var.project_id
  location = var.region

  network    = var.network
  subnetwork = var.subnetwork

  remove_default_node_pool = true
  initial_node_count       = 1

  # portfolio project — no protection needed against terraform destroy;
  # flip to true once real workloads/data live on this cluster
  deletion_protection = false

  node_config {
    machine_type = "e2-small"
    disk_type    = "pd-standard"
    disk_size_gb = 20
  }

  release_channel {
    channel = "REGULAR"
  }

  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = false
    master_ipv4_cidr_block  = "172.16.0.0/28"
  }

  ip_allocation_policy {
    cluster_secondary_range_name  = "pods"
    services_secondary_range_name = "services"
  }

  # node_config above only applies to the transient default node pool at
  # creation time — that pool is deleted immediately after by
  # remove_default_node_pool. Ignoring changes here prevents Terraform
  # from trying to reconcile it against a pool that no longer exists,
  # which is what caused "Node pool default-pool not found on update".
  lifecycle {
    ignore_changes = [node_config]
  }
}