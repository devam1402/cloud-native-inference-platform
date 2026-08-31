terraform {
  backend "gcs" {
    bucket = "cnip-tfstate-v2"
    prefix = "gke"
  }
}
