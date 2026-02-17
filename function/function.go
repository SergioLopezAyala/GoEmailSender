package function

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

func init() {
	// Register the HTTP function with the Functions Framework
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
	// Set CORS headers for browser requests
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle preflight OPTIONS request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Only accept POST requests
	if r.Method != http.MethodPost {
		sendErrorResponse(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	// Parse the request body
	var emailReq EmailRequest
	if err := json.NewDecoder(r.Body).Decode(&emailReq); err != nil {
		log.Printf("Failed to decode request body: %v", err)
		sendErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate the request
	if err := validateEmailRequest(&emailReq); err != nil {
		log.Printf("Validation failed: %v", err)
		sendErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	// Get SendGrid API key from environment variable
	sendGridAPIKey := os.Getenv("SENDGRID_API_KEY")
	if sendGridAPIKey == "" {
		log.Println("SENDGRID_API_KEY environment variable is not set")
		sendErrorResponse(w, http.StatusInternalServerError, "Email service not configured")
		return
	}

	// Send the email
	if err := sendEmailViaSendGrid(sendGridAPIKey, &emailReq); err != nil {
		log.Printf("Failed to send email: %v", err)
		sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to send email: %v", err))
		return
	}

	// Send success response
	response := EmailResponse{
		Success: true,
		Message: "Email sent successfully",
	}

	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		return
	}
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

// sendEmailViaSendGrid sends an email using the SendGrid API
func sendEmailViaSendGrid(apiKey string, req *EmailRequest) error {
	// Create SendGrid mail objects
	from := mail.NewEmail("", req.From)
	to := mail.NewEmail("", req.To)

	// Create the message
	var message *mail.SGMailV3
	if req.HTMLContent != "" {
		// If HTML content is provided, use it (with optional plain text fallback)
		message = mail.NewSingleEmail(from, req.Subject, to, req.TextContent, req.HTMLContent)
	} else {
		// Plain text only
		message = mail.NewSingleEmail(from, req.Subject, to, req.TextContent, "")
	}

	// Create SendGrid client
	client := sendgrid.NewSendClient(apiKey)

	// Send the email
	response, err := client.Send(message)
	if err != nil {
		return fmt.Errorf("sendgrid client error: %w", err)
	}

	// Check response status
	if response.StatusCode >= 400 {
		return fmt.Errorf("sendgrid API error: status %d, body: %s", response.StatusCode, response.Body)
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
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		return
	}
}
