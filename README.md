# Goemailsender

A Google Cloud Run function written in Go that sends emails via the Gmail API using **keyless authentication** (no JSON key files). It impersonates a Google Workspace user through Domain-Wide Delegation and sends from a private alias, keeping the base admin address hidden from recipients.

---

## What it does

- Exposes a single HTTP endpoint (`POST /`) that accepts an email payload
- Authenticates using **Workload Identity + Service Account Impersonation** — no stored credentials
- Impersonates a Google Workspace mailbox (`admin@your-domain.com`) via Domain-Wide Delegation
- Sends all emails **from an alias** (`noreply@your-domain.com`) so the admin address is never exposed
- Supports both plain text and HTML email bodies
- Returns a JSON response indicating success or failure

---

## Authentication chain

```
Cloud Run runtime SA (ADC)
    → Token Creator role
        → DWD Service Account (email-sender-dwd)
            → Domain-Wide Delegation
                → admin@your-domain.com (Workspace mailbox)
                    → From: noreply@your-domain.com (alias)
```

No JSON key file is stored anywhere. Google's metadata server provides the base identity automatically at runtime.

---

## API

### Request

```
POST https://<your-cloud-run-url>
Content-Type: application/json
```

```json
{
  "to": "recipient@example.com",
  "subject": "Hello from our contact form",
  "text_content": "Plain text body",
  "html_content": "<p>Or an HTML body</p>"
}
```

| Field | Required | Description |
|---|---|---|
| `to` | ✅ | Recipient email address |
| `subject` | ✅ | Email subject line |
| `text_content` | ⚠️ | Plain text body — required if `html_content` is absent |
| `html_content` | ⚠️ | HTML body — required if `text_content` is absent |
| `from` | ❌ | Ignored — sender is always hardcoded to your configured alias |

### Response

**Success (`200`)**
```json
{
  "success": true,
  "message": "Email sent successfully"
}
```

**Error (`400 / 405 / 500`)**
```json
{
  "success": false,
  "message": "Failed to send email",
  "error": "description of what went wrong"
}
```

---

## Environment variables

These must be set on the Cloud Run service:

| Variable | Value | Description |
|---|---|---|
| `GMAIL_DWD_SERVICE_ACCOUNT` | `email-sender-dwd@<your-gcp-project>.iam.gserviceaccount.com` | The service account that has Domain-Wide Delegation enabled |
| `GMAIL_IMPERSONATE_USER` | `admin@your-domain.com` | The Workspace mailbox to impersonate — must own the alias |

---

## Project structure

```
emailSender/
├── function.go     # Cloud Run function entrypoint and all logic
├── go.mod
└── go.sum
```

---

## Dependencies

```go
google.golang.org/api/gmail/v1
google.golang.org/api/impersonate
google.golang.org/api/option
github.com/GoogleCloudPlatform/functions-framework-go/functions
```

Install / update:
```bash
go get google.golang.org/api/impersonate
go mod tidy
```

---

## One-time GCP & Workspace setup

### 1. Create the alias in Google Workspace Admin

1. Go to [Google Admin Console](https://admin.google.com) → **Directory → Users**
2. Click `admin@your-domain.com` → **User information → Alternate emails**
3. Add `noreply@your-domain.com` (or your chosen alias) and save

> Without this step the Gmail API will reject sends from the alias with a `403` error.

---

### 2. Create the DWD service account (no keys)

```bash
gcloud iam service-accounts create email-sender-dwd \
  --display-name="Email Sender DWD" \
  --project=<your-gcp-project>
```

Then in [Cloud Console → IAM & Admin → Service Accounts](https://console.cloud.google.com/iam-admin/serviceaccounts):

1. Click **Email Sender DWD** → **Details** tab → **Edit**
2. Enable **"Google Workspace Domain-wide Delegation"**
3. Save and copy the generated **Client ID**

---

### 3. Grant Token Creator to the Cloud Run runtime SA

```bash
gcloud iam service-accounts add-iam-policy-binding \
  email-sender-dwd@<your-gcp-project>.iam.gserviceaccount.com \
  --member="serviceAccount:<your-cloud-run-sa>@<your-gcp-project>.iam.gserviceaccount.com" \
  --role="roles/iam.serviceAccountTokenCreator"
```

> The Cloud Run runtime SA is the service account assigned to your Cloud Run function. If you haven't set a custom one it defaults to `<project-number>-compute@developer.gserviceaccount.com`.

---

### 4. Authorize the Gmail scope in Google Workspace Admin

1. Go to [Google Admin Console](https://admin.google.com) → **Security → Access and data control → API controls**
2. Click **Manage Domain-Wide Delegation → Add new**
3. Fill in:
   - **Client ID**: the number copied in step 2
   - **OAuth Scopes**: `https://www.googleapis.com/auth/gmail.send`
4. Click **Authorize**

> You must be a **Super Admin** on the Google Workspace account to complete this step.

---

## Deployment

### Deploy or update the function

```bash
gcloud run functions deploy goemailsender \
  --region=<your-region> \
  --project=<your-gcp-project> \
  --runtime=go122 \
  --entry-point=SendEmail \
  --trigger-http \
  --allow-unauthenticated \
  --service-account=<your-cloud-run-sa>@<your-gcp-project>.iam.gserviceaccount.com
```

### Set environment variables

```bash
gcloud run services update goemailsender \
  --region=<your-region> \
  --project=<your-gcp-project> \
  --set-env-vars="GMAIL_DWD_SERVICE_ACCOUNT=email-sender-dwd@<your-gcp-project>.iam.gserviceaccount.com,GMAIL_IMPERSONATE_USER=admin@your-domain.com"
```

### Confirm the runtime service account

```bash
gcloud run services describe goemailsender \
  --region=<your-region> \
  --project=<your-gcp-project> \
  --format="value(spec.template.spec.serviceAccountName)"
```

---

## Testing

```bash
curl -X POST https://<your-cloud-run-url> \
  -H "Content-Type: application/json" \
  -d '{
    "to": "test@example.com",
    "subject": "Test email",
    "html_content": "<p>Hello from the contact form.</p>"
  }'
```

---

## Security notes

- **No JSON key files** are used or stored anywhere — authentication is handled entirely by Google's IAM infrastructure via Workload Identity
- The `From` address is **hardcoded** in the function — callers cannot spoof the sender address via the request body
- The base admin address is **never exposed** to email recipients — only the alias is visible
- CORS headers are set to `*` — restrict `Access-Control-Allow-Origin` to your domain in production
