variable "project_id" {
  type = string
}

variable "project_name" {
  type    = string
  default = "cloud-native-inference-platform"
}

variable "environment" {
  type    = string
  default = "platform"
}

variable "region" {
  type = string
}

variable "zone" {
  type = string
}

variable "cluster_name" {
  type = string
}

variable "network_name" {
  type    = string
  default = "cnip-vpc"
}

variable "subnet_name" {
  type    = string
  default = "cnip-gke-subnet"
}
