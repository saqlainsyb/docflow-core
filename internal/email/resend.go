// internal/email/resend.go
//
// Resend API client — zero external dependencies.
//
// We call Resend's REST API directly with net/http rather than pulling in their
// Go SDK. This keeps the dependency graph clean and the implementation fully
// auditable. Resend's free tier gives 3,000 emails/month and 100/day, which is
// more than enough for a workspace collaboration tool at this scale.
//
// Resend API reference: https://resend.com/docs/api-reference/emails/send-email
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const resendAPIURL = "https://api.resend.com/emails"

// Client sends transactional emails via the Resend API.
// It is safe for concurrent use — all state is immutable after construction.
type Client struct {
	apiKey     string
	fromAddr   string
	httpClient *http.Client
}

// NewClient constructs a Resend email client.
//
//   apiKey   — Resend API key (re_xxxx), loaded from RESEND_API_KEY env var
//   fromAddr — verified sender address, e.g. "Docflow <invites@docflow.asia>"
func NewClient(apiKey, fromAddr string) *Client {
	return &Client{
		apiKey:   apiKey,
		fromAddr: fromAddr,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// sendEmailRequest is the JSON body Resend expects.
type sendEmailRequest struct {
	From    string `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

// sendEmailResponse is the successful response from Resend.
type sendEmailResponse struct {
	ID string `json:"id"`
}

// resendErrorResponse is the error body Resend returns on failure.
type resendErrorResponse struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
	Name       string `json:"name"`
}

// SendInvitation sends a workspace invitation email to the given address.
// It is the only outbound email type in V1 — designed to be extended later
// (e.g. due-date reminders, mention notifications) by adding more Send* methods.
//
// Returns a non-nil error if Resend rejects the request or the network fails.
// Callers should treat email send failure as non-fatal: log the error and
// return an appropriate API error to the requester rather than panicking.
func (c *Client) SendInvitation(ctx context.Context, req InvitationEmailData) error {
	body := sendEmailRequest{
		From:    c.fromAddr,
		To:      []string{req.RecipientEmail},
		Subject: fmt.Sprintf("You've been invited to %s on Docflow", req.WorkspaceName),
		HTML:    renderInvitationEmail(req),
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("email: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, resendAPIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("email: build request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("email: http do: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("email: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errResp resendErrorResponse
		_ = json.Unmarshal(respBytes, &errResp)
		return fmt.Errorf("email: resend error %d: %s", resp.StatusCode, errResp.Message)
	}

	return nil
}