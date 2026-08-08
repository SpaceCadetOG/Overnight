terraform {
  required_providers { google = { source = "hashicorp/google"; version = "~> 7.0" } }
}
provider "google" { project = var.project_id; region = var.region }

locals { prefix = "trade-forensics-${var.environment}" }

resource "google_storage_bucket" "raw" {
  name = "${var.project_id}-${local.prefix}-raw"
  location = var.region
  uniform_bucket_level_access = true
  versioning { enabled = true }
  soft_delete_policy { retention_duration_seconds = 604800 }
}
resource "google_storage_bucket" "validation" {
  name = "${var.project_id}-${local.prefix}-validation"
  location = var.region
  uniform_bucket_level_access = true
  versioning { enabled = true }
}
resource "google_storage_bucket" "quarantine" {
  name = "${var.project_id}-${local.prefix}-quarantine"
  location = var.region
  uniform_bucket_level_access = true
  versioning { enabled = true }
}

resource "google_service_account" "jumppi_uploader" { account_id = "jumppi-uploader"; display_name = "JumpPi immutable package uploader" }
resource "google_storage_bucket_iam_member" "jumppi_create" { bucket = google_storage_bucket.raw.name; role = "roles/storage.objectCreator"; member = "serviceAccount:${google_service_account.jumppi_uploader.email}" }
resource "google_storage_bucket_iam_member" "jumppi_validation_read" { bucket = google_storage_bucket.validation.name; role = "roles/storage.objectViewer"; member = "serviceAccount:${google_service_account.jumppi_uploader.email}" }

resource "google_service_account" "validator" { account_id = "package-validator"; display_name = "Cloud package validator" }
resource "google_storage_bucket_iam_member" "validator_raw_read" { bucket = google_storage_bucket.raw.name; role = "roles/storage.objectViewer"; member = "serviceAccount:${google_service_account.validator.email}" }
resource "google_storage_bucket_iam_member" "validator_write" { bucket = google_storage_bucket.validation.name; role = "roles/storage.objectCreator"; member = "serviceAccount:${google_service_account.validator.email}" }
resource "google_storage_bucket_iam_member" "validator_quarantine" { bucket = google_storage_bucket.quarantine.name; role = "roles/storage.objectCreator"; member = "serviceAccount:${google_service_account.validator.email}" }

resource "google_cloud_run_v2_service" "validator" {
  name = "trade-package-validator"; location = var.region
  template {
    service_account = google_service_account.validator.email
    containers {
      image = var.validator_image
      env { name = "VALIDATION_BUCKET"; value = google_storage_bucket.validation.name }
      env { name = "QUARANTINE_BUCKET"; value = google_storage_bucket.quarantine.name }
    }
  }
}

resource "google_pubsub_topic" "uploads" { name = "trade-package-uploads" }
resource "google_storage_notification" "uploads" {
  bucket = google_storage_bucket.raw.name; topic = google_pubsub_topic.uploads.id
  payload_format = "JSON_API_V1"; event_types = ["OBJECT_FINALIZE"]
  depends_on = [google_pubsub_topic_iam_member.gcs_publish]
}
data "google_storage_project_service_account" "gcs" {}
resource "google_pubsub_topic_iam_member" "gcs_publish" { topic = google_pubsub_topic.uploads.name; role = "roles/pubsub.publisher"; member = "serviceAccount:${data.google_storage_project_service_account.gcs.email_address}" }

resource "google_service_account" "pubsub_push" { account_id = "validator-push"; display_name = "Authenticated PubSub validator invoker" }
resource "google_cloud_run_v2_service_iam_member" "invoke" { name = google_cloud_run_v2_service.validator.name; location = var.region; role = "roles/run.invoker"; member = "serviceAccount:${google_service_account.pubsub_push.email}" }
resource "google_pubsub_subscription" "validator" {
  name = "trade-package-validator"; topic = google_pubsub_topic.uploads.name
  ack_deadline_seconds = 600
  retry_policy { minimum_backoff = "10s"; maximum_backoff = "600s" }
  push_config {
    push_endpoint = google_cloud_run_v2_service.validator.uri
    oidc_token { service_account_email = google_service_account.pubsub_push.email }
  }
}
