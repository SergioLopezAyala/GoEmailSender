package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

func init() {
	functions.HTTP("SendEmail", SendEmail)
}

// EmailRequest represents the incoming email request structure
type EmailRequest struct {
	To          string `json:"to"`
	From        string `json:"from"`
	Subject     string `json:"subject"`
	TextContent string `json:"text_content,omitempty"`
	HTMLContent string `json:"html_content,omitempty"`
}

// EmailResponse represents the response structure
type EmailResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// SendEmail is the Cloud Function entry point
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

	// 🔐 CALL SMTP FUNCTION
	if err := sendEmailViaSMTP(&emailReq); err != nil {
		log.Printf("Failed to send email: %v", err)
		sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to send email: %v", err))
		return
	}

	response := EmailResponse{
		Success: true,
		Message: "Email sent successfully",
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)

	log.Printf("Email sent successfully to: %s", emailReq.To)
}

// validateEmailRequest validates the email request fields
func validateEmailRequest(req *EmailRequest) error {
	if req.To == "" {
		return fmt.Errorf("'to' field is required")
	}
	if req.From == "" {
		return fmt.Errorf("'from' field is required")
	}
	if req.Subject == "" {
		return fmt.Errorf("'subject' field is required")
	}
	if req.TextContent == "" && req.HTMLContent == "" {
		return fmt.Errorf("either 'text_content' or 'html_content' is required")
	}
	return nil
}

func sendEmailViaSMTP(req *EmailRequest) error {

	from := os.Getenv("GMAIL_ADDRESS")
	if from == "" {
		return fmt.Errorf("GMAIL_ADDRESS environment variable not set")
	}

	password := os.Getenv("GMAIL_APP_PASSWORD")
	if password == "" {
		return fmt.Errorf("GMAIL_APP_PASSWORD environment variable not set")
	}

	smtpHost := "smtp.gmail.com"
	smtpPort := "587"

	auth := smtp.PlainAuth("", from, password, smtpHost)

	// ================================
	// BUILD EMAIL MESSAGE
	// ================================
	var msg string

	if req.HTMLContent != "" {
		msg = "MIME-version: 1.0;\r\n" +
			"Content-Type: text/html; charset=\"UTF-8\";\r\n" +
			fmt.Sprintf("Subject: %s\r\n", req.Subject) +
			fmt.Sprintf("To: %s\r\n", req.To) +
			"\r\n" + req.HTMLContent
	} else {
		msg = fmt.Sprintf(
			"Subject: %s\r\nTo: %s\r\n\r\n%s",
			req.Subject,
			req.To,
			req.TextContent,
		)
	}

	addr := smtpHost + ":" + smtpPort

	// ================================
	// SEND EMAIL
	// ================================
	err := smtp.SendMail(
		addr,
		auth,
		from,
		[]string{req.To},
		[]byte(msg),
	)

	if err != nil {
		return fmt.Errorf("smtp error: %w", err)
	}

	return nil
}

// sendErrorResponse sends a JSON error response
func sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := EmailResponse{
		Success: false,
		Message: "Failed to send email",
		Error:   message,
	}

	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(response)
}
