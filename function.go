package emailSender

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
)

func init() {
	functions.HTTP("SendEmail", SendEmail)
}

type EmailRequest struct {
	To          string `json:"to"`
	From        string `json:"from"`
	Subject     string `json:"subject"`
	TextContent string `json:"text_content,omitempty"`
	HTMLContent string `json:"html_content,omitempty"`
}

type EmailResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

func SendEmail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodPost {
		sendErrorResponse(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	var emailReq EmailRequest
	if err := json.NewDecoder(r.Body).Decode(&emailReq); err != nil {
		log.Printf("Failed to decode request body: %v", err)
		sendErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := validateEmailRequest(&emailReq); err != nil {
		log.Printf("Validation failed: %v", err)
		sendErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := sendEmailViaGmailAPI(&emailReq); err != nil {
		log.Printf("Failed to send email: %v", err)
		sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to send email: %v", err))
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(EmailResponse{
		Success: true,
		Message: "Email sent successfully",
	})

	log.Printf("Email sent successfully to: %s", emailReq.To)
}

func validateEmailRequest(req *EmailRequest) error {
	if req.To == "" {
		return fmt.Errorf("'to' field is required")
	}
	if req.Subject == "" {
		return fmt.Errorf("'subject' field is required")
	}
	if req.TextContent == "" && req.HTMLContent == "" {
		return fmt.Errorf("either 'text_content' or 'html_content' is required")
	}
	return nil
}

func sendEmailViaGmailAPI(req *EmailRequest) error {
	ctx := context.Background()

	// The DWD service account email set via Cloud Run env var
	dwdServiceAccount := os.Getenv("GMAIL_DWD_SERVICE_ACCOUNT")
	if dwdServiceAccount == "" {
		return fmt.Errorf("GMAIL_DWD_SERVICE_ACCOUNT env var not set")
	}

	impersonateUser := os.Getenv("GMAIL_IMPERSONATE_USER")
	if impersonateUser == "" {
		return fmt.Errorf("GMAIL_IMPERSONATE_USER env var not set")
	}

	// Always send from the alias — ignore whatever From is passed in the request
	fromAlias := "hi@dualcore-dev.com"

	// Keyless auth chain:
	// Cloud Run SA (ADC) → Token Creator → DWD SA → Workspace user
	// No JSON key file anywhere in this chain.
	ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
		TargetPrincipal: dwdServiceAccount,
		Scopes:          []string{gmail.GmailSendScope},
		Subject:         impersonateUser,
	})
	if err != nil {
		return fmt.Errorf("failed to create impersonated token source: %w", err)
	}

	srv, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("failed to create gmail service: %w", err)
	}

	// Build the RFC 2822 raw message
	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("From: %s\r\n", fromAlias))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", req.To))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", req.Subject))
	msg.WriteString("MIME-Version: 1.0\r\n")

	if req.HTMLContent != "" {
		msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n")
		msg.WriteString(req.HTMLContent)
	} else {
		msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
		msg.WriteString(req.TextContent)
	}

	raw := base64.URLEncoding.EncodeToString(msg.Bytes())

	_, err = srv.Users.Messages.Send("me", &gmail.Message{Raw: raw}).Do()
	if err != nil {
		return fmt.Errorf("gmail API send error: %w", err)
	}

	return nil
}

func sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(EmailResponse{
		Success: false,
		Message: "Failed to send email",
		Error:   message,
	})
}
