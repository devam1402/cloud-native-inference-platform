output "cluster_name" { value = module.gke.cluster_name }
output "artifact_registry" { value = module.artifact_registry.repository_url }
output "controller_sa" { value = module.iam.controller_sa_email }
