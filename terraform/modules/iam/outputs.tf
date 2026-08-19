output "gke_node_sa_email" { value = google_service_account.gke_node.email }
output "controller_sa_email" { value = google_service_account.platform_controller.email }
