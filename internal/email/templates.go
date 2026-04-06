// internal/email/templates.go
//
// HTML email templates for Docflow transactional emails.
//
// Design constraints:
// - Inline CSS only — email clients strip <style> blocks and <head> entirely.
// - No external resources — images, fonts, or stylesheets behind URLs can be
//   blocked by security-conscious email clients (Outlook, Apple Mail).
// - Width capped at 600px — the universally safe width for email rendering.
// - Dark-on-light palette — dark mode email support is inconsistent across
//   clients; light background is the safe default.
// - Plain text fallback not included (Resend handles that automatically from HTML).
package email

import "fmt"

// InvitationEmailData contains all values the invitation template needs.
// Keep this flat — no nested structs — for simple template rendering.
type InvitationEmailData struct {
	RecipientEmail   string // address being invited
	InviterName      string // display name of the person who sent the invite
	WorkspaceName    string // workspace the recipient is being invited to
	AcceptURL        string // deep link: frontend_url/invitations/:token
	IsExistingUser   bool   // controls CTA copy (accept vs register)
	ExpiresInDays    int    // e.g. 7, shown in the email footer
}

// renderInvitationEmail returns the full HTML string for a workspace invitation.
// Template uses fmt.Sprintf for simple substitution — no html/template needed
// at this scale, and it avoids the overhead of parsing a template on every call.
func renderInvitationEmail(d InvitationEmailData) string {
	ctaText := "Accept Invitation"
	subheading := fmt.Sprintf(
		"%s has invited you to collaborate on <strong>%s</strong> in Docflow.",
		d.InviterName,
		d.WorkspaceName,
	)

	if !d.IsExistingUser {
		ctaText = "Create Account & Join"
		subheading = fmt.Sprintf(
			"%s has invited you to collaborate on <strong>%s</strong> in Docflow. "+
				"Create a free account to get started.",
			d.InviterName,
			d.WorkspaceName,
		)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>You're invited to %s</title>
</head>
<body style="margin:0;padding:0;background-color:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background-color:#f4f4f5;padding:40px 0;">
    <tr>
      <td align="center">
        <!-- Card -->
        <table role="presentation" width="600" cellpadding="0" cellspacing="0"
               style="max-width:600px;width:100%%;background-color:#ffffff;border-radius:12px;
                      box-shadow:0 1px 3px rgba(0,0,0,0.08);overflow:hidden;">

          <!-- Header bar -->
          <tr>
            <td style="background-color:#09090b;padding:28px 40px;text-align:left;">
              <span style="color:#ffffff;font-size:20px;font-weight:700;letter-spacing:-0.3px;">
                Docflow
              </span>
            </td>
          </tr>

          <!-- Body -->
          <tr>
            <td style="padding:40px 40px 32px;">
              <h1 style="margin:0 0 12px;font-size:24px;font-weight:700;color:#09090b;
                         letter-spacing:-0.5px;line-height:1.3;">
                You've been invited
              </h1>
              <p style="margin:0 0 32px;font-size:16px;color:#52525b;line-height:1.6;">
                %s
              </p>

              <!-- Workspace chip -->
              <table role="presentation" cellpadding="0" cellspacing="0"
                     style="margin-bottom:32px;background-color:#f4f4f5;border-radius:8px;
                            border:1px solid #e4e4e7;">
                <tr>
                  <td style="padding:16px 20px;">
                    <span style="font-size:11px;font-weight:600;color:#a1a1aa;
                                 text-transform:uppercase;letter-spacing:0.8px;">
                      Workspace
                    </span>
                    <br />
                    <span style="font-size:18px;font-weight:600;color:#09090b;">
                      %s
                    </span>
                  </td>
                </tr>
              </table>

              <!-- CTA button -->
              <table role="presentation" cellpadding="0" cellspacing="0">
                <tr>
                  <td style="border-radius:8px;background-color:#09090b;">
                    <a href="%s"
                       style="display:inline-block;padding:14px 28px;font-size:15px;
                              font-weight:600;color:#ffffff;text-decoration:none;
                              letter-spacing:-0.1px;">
                      %s
                    </a>
                  </td>
                </tr>
              </table>

              <p style="margin:24px 0 0;font-size:13px;color:#a1a1aa;line-height:1.6;">
                Or copy and paste this link into your browser:
                <br />
                <a href="%s" style="color:#09090b;word-break:break-all;">%s</a>
              </p>
            </td>
          </tr>

          <!-- Footer -->
          <tr>
            <td style="padding:20px 40px 32px;border-top:1px solid #f4f4f5;">
              <p style="margin:0;font-size:12px;color:#a1a1aa;line-height:1.6;">
                This invitation was sent to <strong>%s</strong> and expires in
                <strong>%d days</strong>. If you weren't expecting this, you can
                safely ignore this email.
              </p>
              <p style="margin:12px 0 0;font-size:12px;color:#a1a1aa;">
                &copy; Docflow &mdash; Real-time collaborative document boards
              </p>
            </td>
          </tr>

        </table>
      </td>
    </tr>
  </table>
</body>
</html>`,
		d.WorkspaceName,  // <title>
		subheading,       // body paragraph
		d.WorkspaceName,  // workspace chip
		d.AcceptURL,      // CTA href
		ctaText,          // CTA text
		d.AcceptURL,      // plain text link href
		d.AcceptURL,      // plain text link display
		d.RecipientEmail, // footer: "sent to"
		d.ExpiresInDays,  // footer: "expires in N days"
	)
}