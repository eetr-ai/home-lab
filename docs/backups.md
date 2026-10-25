# Database backups to Google Drive

The admin panel schedules backups of the lab's PostgreSQL and MongoDB databases
into a Google Drive folder you pick, and keeps as many of them as you ask for.
This guide covers the one-time Google setup and what the panel does with it.

## How it fits together

```text
Backups page ── connect ──▶ Google consent (drive.file) ──▶ grant kept in a Secret
      │
      └── schedule ──▶ CronJob in the panel's namespace
                         ├─ dump containers   pg_dump / mongodump into a scratch volume
                         └─ upload container  admin-api backup-upload: upload, then trash
                                              whatever is past "keep N"
```

- **The panel sees one folder.** The scope is `drive.file`: the panel reaches the
  files it uploaded and the folders you picked for it in Google's Picker, and
  nothing else in your Drive.
- **Each schedule is a CronJob.** Listing schedules means listing CronJobs, and
  listing runs means listing their Jobs. There is no other record to drift.
- **Retention only touches the panel's own files.** Every upload is tagged with
  the schedule and database it came from, and pruning only considers tagged files.
  Pruned backups go to Drive's trash, which empties itself after 30 days.

## One-time Google setup

Everything below is in the [Google Cloud console](https://console.cloud.google.com/)
for the project `terraform/gcp` manages.

### 1. The APIs and the Picker key

Apply `terraform/gcp`, which needs `panel_origin` set in its ignored
`terraform.tfvars` to the origin the panel answers on. It enables the Drive and
Picker APIs and creates a browser key that is accepted only from that origin and
only by the Picker API.

```bash
terraform -chdir=terraform/gcp apply
terraform -chdir=terraform/gcp output -raw picker_api_key
terraform -chdir=terraform/gcp output -raw picker_app_id
```

Those two outputs go in the panel's values, as
`admin.api.backups.google.picker.apiKey` and `.appId`.

### 2. The consent screen

Under **Google Auth Platform → Branding/Audience**:

1. Set the user type to **External**. A personal Gmail account cannot use Internal.
2. Add the scopes `.../auth/drive.file`, `openid` and `email`. All three are
   non-sensitive, so Google does not ask for app verification.
3. **Publish the app to "In production".** This step is the one that matters
   most. An app left in *Testing* gets refresh tokens that expire after seven
   days, so every scheduled backup would start failing a week after you
   connected.

### 3. The OAuth client

Under **Clients → Create client**, choose **Web application**:

- **Authorized JavaScript origins:** the panel's origin, for example
  `https://admin.example.invalid`.
- **Authorized redirect URIs:**
  `https://admin.example.invalid/api/backups/google/callback`. Google matches it
  exactly, with no wildcards.

Store the client in the panel's namespace. This is the only Secret the feature
needs you to create:

```bash
kubectl -n admin create secret generic admin-google-drive \
  --from-literal=clientId=YOUR_CLIENT_ID \
  --from-literal=clientSecret=YOUR_CLIENT_SECRET
```

The API reads it at startup, so restart `admin-api` after creating or changing it.
Until it exists, the Backups page says Drive is not set up.

### 4. Connect

On the Backups page, choose **Connect Google Drive** and approve the consent
screen. The API exchanges the code and keeps the grant in the Secret
`admin-google-drive-token`, which the backup pods read. **Disconnect** revokes it
at Google and deletes the Secret.

If Google ever says the panel did not get offline access, remove the panel under
your Google account's **Security → Third-party access** and connect again.
