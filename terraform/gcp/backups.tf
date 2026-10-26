# What the admin panel's Google Drive backups need from this project.
#
# Two APIs and one browser key. The OAuth client and its consent screen are NOT
# here, and cannot be: the Google provider has no resource for a standard "Web
# application" client (the IAP brand and client resources are a different thing,
# and deprecated). docs/backups.md walks through creating them by hand.

resource "google_project_service" "drive" {
  project            = var.project_id
  service            = "drive.googleapis.com"
  disable_on_destroy = false
}

# Google's folder Picker, which the panel opens in the operator's browser so they
# can hand it one folder rather than their whole Drive.
resource "google_project_service" "picker" {
  project            = var.project_id
  service            = "picker.googleapis.com"
  disable_on_destroy = false
}

# Needed to manage the key below from Terraform at all.
resource "google_project_service" "api_keys" {
  project            = var.project_id
  service            = "apikeys.googleapis.com"
  disable_on_destroy = false
}

# The Picker's browser key.
#
# A browser key is public by design -- the page hands it to Google in the clear --
# so what makes it safe is the two restrictions: it is only accepted from the
# panel's own origin, and only for the Picker API. A key copied off the page is
# useless anywhere else and for anything else.
resource "google_apikeys_key" "picker" {
  # Billed to this project explicitly; see the "billed" provider for why this
  # API, and only this one, needs it.
  provider = google.billed

  project      = var.project_id
  name         = "home-lab-admin-picker"
  display_name = "home-lab admin panel: Drive folder picker"

  restrictions {
    browser_key_restrictions {
      allowed_referrers = [for origin in concat([var.panel_origin], var.picker_extra_origins) : "${origin}/*"]
    }
    api_targets {
      service = "picker.googleapis.com"
    }
  }

  depends_on = [google_project_service.api_keys, google_project_service.picker]
}
