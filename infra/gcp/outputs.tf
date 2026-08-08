output "raw_bucket" { value = google_storage_bucket.raw.name }
output "validation_bucket" { value = google_storage_bucket.validation.name }
output "quarantine_bucket" { value = google_storage_bucket.quarantine.name }
output "jumppi_service_account" { value = google_service_account.jumppi_uploader.email }
