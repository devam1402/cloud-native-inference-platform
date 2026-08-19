resource "google_service_account_iam_member" "controller_wi" {
  service_account_id = google_service_account.platform_controller.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[flowplane-system/controller-manager]"
}

resource "google_project_iam_member" "controller_artifact_reader" {
  project = var.project_id
  role    = "roles/artifactregistry.reader"
  member  = "serviceAccount:${google_service_account.platform_controller.email}"
}

resource "google_project_iam_member" "gke_node_minimal" {
  project = var.project_id
  role    = "roles/container.nodeServiceAccount"
  member  = "serviceAccount:${google_service_account.gke_node.email}"
}
