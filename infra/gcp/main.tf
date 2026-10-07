terraform {
  required_version = ">= 1.6.0"
  required_providers {
    google = { source = "hashicorp/google", version = "~> 7.0" }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
  zone    = var.zone
}

variable "project_id" { type = string }
variable "region" {
  type    = string
  default = "us-central1"
}
variable "zone" {
  type    = string
  default = "us-central1-a"
}
variable "name" {
  type    = string
  default = "artex-english"
}
variable "tier" {
  type    = string
  default = "standard"
  validation {
    condition     = contains(["free", "small", "standard", "work"], var.tier)
    error_message = "tier must be free, small, standard, or work."
  }
}
variable "machine_type" {
  type        = string
  default     = ""
  description = "Optional. Overrides the machine selected by tier. Disk size, swap, and the Node build heap still come from tier."
}
variable "ssh_public_key" { type = string }
variable "admin_cidr" { type = string }
variable "web_cidrs" {
  type    = list(string)
  default = []
}
variable "domain" {
  type    = string
  default = ""
  validation {
    condition     = var.domain == "" || can(regex("^[A-Za-z0-9.-]+$", var.domain))
    error_message = "domain must be a plain DNS hostname."
  }
}
variable "dns_managed_zone" {
  type    = string
  default = ""
}
variable "git_ref" {
  type    = string
  default = "main"
  validation {
    condition     = can(regex("^[A-Za-z0-9._/-]+$", var.git_ref))
    error_message = "git_ref must contain only Git branch or tag characters."
  }
}

locals {
  tiers = {
    free     = { machine = "e2-micro", disk_gb = 30, disk_type = "pd-standard", node_heap_mb = 1536, swap_mb = 4096 }
    small    = { machine = "e2-small", disk_gb = 40, disk_type = "pd-balanced", node_heap_mb = 1536, swap_mb = 2048 }
    standard = { machine = "e2-standard-2", disk_gb = 100, disk_type = "pd-balanced", node_heap_mb = 3072, swap_mb = 0 }
    work     = { machine = "e2-standard-4", disk_gb = 128, disk_type = "pd-balanced", node_heap_mb = 3072, swap_mb = 0 }
  }
  spec         = local.tiers[var.tier]
  machine_type = var.machine_type != "" ? var.machine_type : local.spec.machine
  permitted_web_cidrs = length(var.web_cidrs) == 0 ? [var.admin_cidr] : var.web_cidrs
  bootstrap = replace(replace(replace(replace(replace(
    file("${path.module}/../bootstrap.sh"),
    "__REPO_URL__", "https://github.com/skilfoy/ARTEX-English.git"),
    "__GIT_REF__", var.git_ref),
    "__APP_DOMAIN__", var.domain == "" ? ":80" : var.domain),
    "__SWAP_MB__", tostring(local.spec.swap_mb)),
    "__NODE_HEAP_MB__", tostring(local.spec.node_heap_mb),
  )
}

resource "google_compute_network" "app" {
  name                    = "${var.name}-network"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "app" {
  name          = "${var.name}-subnet"
  region        = var.region
  network       = google_compute_network.app.id
  ip_cidr_range = "10.43.1.0/24"
}

resource "google_compute_firewall" "ssh" {
  name          = "${var.name}-ssh"
  network       = google_compute_network.app.name
  source_ranges = [var.admin_cidr]
  target_tags   = [var.name]
  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}

resource "google_compute_firewall" "web" {
  name          = "${var.name}-web"
  network       = google_compute_network.app.name
  source_ranges = local.permitted_web_cidrs
  target_tags   = [var.name]
  allow {
    protocol = "tcp"
    ports    = ["80", "443"]
  }
}

resource "google_compute_address" "app" {
  name   = "${var.name}-ip"
  region = var.region
}

resource "google_compute_instance" "app" {
  name = var.name

  lifecycle {
    precondition {
      condition = var.tier != "free" || (
        contains(["us-west1", "us-central1", "us-east1"], var.region) &&
        startswith(var.zone, "${var.region}-")
      )
      error_message = "GCP tier=free is the always-free e2-micro only in us-west1, us-central1, or us-east1, and zone must be in that region. The default us-central1-a qualifies."
    }
  }
  machine_type              = local.machine_type
  zone                      = var.zone
  tags                      = [var.name]
  metadata_startup_script   = local.bootstrap
  metadata                  = { ssh-keys = "ubuntu:${var.ssh_public_key}" }
  allow_stopping_for_update = true

  boot_disk {
    initialize_params {
      image = "ubuntu-os-cloud/ubuntu-2404-lts-amd64"
      size  = local.spec.disk_gb
      type  = local.spec.disk_type
    }
  }

  network_interface {
    subnetwork = google_compute_subnetwork.app.id
    access_config {
      nat_ip = google_compute_address.app.address
    }
  }
}

resource "google_dns_record_set" "app" {
  count        = var.domain != "" && var.dns_managed_zone != "" ? 1 : 0
  managed_zone = var.dns_managed_zone
  name         = "${trimsuffix(var.domain, ".")}."
  type         = "A"
  ttl          = 60
  rrdatas      = [google_compute_address.app.address]
}

output "public_ip" { value = google_compute_address.app.address }
output "app_url" { value = var.domain == "" ? "http://${google_compute_address.app.address}" : "https://${var.domain}" }
output "ssh_command" { value = "ssh ubuntu@${google_compute_address.app.address}" }
output "tier" { value = var.tier }
output "machine_type" { value = local.machine_type }
