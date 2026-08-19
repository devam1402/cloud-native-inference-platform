module "project_services" {
  source     = "../modules/project-services"
  project_id = var.project_id
}

module "network" {
  source       = "../modules/network"
  project_id   = var.project_id
  network_name = var.network_name
  subnet_name  = var.subnet_name
  region       = var.region
  depends_on   = [module.project_services]
}

module "iam" {
  source     = "../modules/iam"
  project_id = var.project_id
}

module "gke" {
  source        = "../modules/gke"
  project_id    = var.project_id
  region        = var.region
  cluster_name  = var.cluster_name
  network       = module.network.network_name
  subnetwork    = module.network.subnetwork_name
  node_sa_email = module.iam.gke_node_sa_email
  depends_on    = [module.project_services, module.network]
}

module "system_node_pool" {
  source        = "../modules/node-pool"
  project_id    = var.project_id
  cluster_name  = module.gke.cluster_name
  location      = var.region
  name          = "system"
  machine_type  = "e2-standard-4"
  node_sa_email = module.iam.gke_node_sa_email
  min_nodes     = 1
  max_nodes     = 2
}

module "platform_node_pool" {
  source        = "../modules/node-pool"
  project_id    = var.project_id
  cluster_name  = module.gke.cluster_name
  location      = var.region
  name          = "platform"
  machine_type  = "e2-standard-8"
  node_sa_email = module.iam.gke_node_sa_email
  min_nodes     = 1
  max_nodes     = 3
}

module "artifact_registry" {
  source     = "../modules/artifact-registry"
  project_id = var.project_id
  region     = var.region
}
