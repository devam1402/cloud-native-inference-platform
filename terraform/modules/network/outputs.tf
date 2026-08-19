output "network_name" { value = google_compute_network.vpc.name }
output "subnetwork_name" { value = google_compute_subnetwork.gke_subnet.name }
output "pods_range" { value = "pods" }
output "services_range" { value = "services" }
