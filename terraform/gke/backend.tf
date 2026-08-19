terraform {
  backend "gcs" {
    bucket = "cnip-tfstate-gold-courage"
    prefix = "gke"
  }
}
