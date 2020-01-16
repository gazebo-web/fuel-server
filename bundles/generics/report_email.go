package generics

import (
	"fmt"
	"net/http"
	"time"

	"bitbucket.org/ignitionrobotics/ign-fuelserver/globals"
	"bitbucket.org/ignitionrobotics/ign-go"
)

// SendReportEmail sends an alert to admins about a resource report
func SendReportEmail(name, owner, category, reason string, r *http.Request) (interface{}, *ign.ErrMsg) {
	sender := globals.FlagsEmailSender
	recipient := globals.FlagsEmailRecipient

	subject := fmt.Sprintf("Resource %s (%s) created by %s was reported", name, category, owner)

	s := globals.Server
	var scheme = "http"
	if s.IsUsingSSL() {
		scheme = "https"
	}

	link := fmt.Sprintf("%s://%s/%s/%s/%s/%s", scheme, r.Host, globals.APIVersion, owner, category, name)

	templateFilename := "templates/email/report_email.html"

	templateData := struct {
		Name     string
		Category string
		Owner    string
		Reason   string
		Link     string
		Time     string
	}{
		Name:     name,
		Category: category,
		Owner:    owner,
		Reason:   reason,
		Link:     link,
		Time:     time.Now().String(),
	}

	logLine := fmt.Sprintf("[REPORT] Resource: %s. Reason: %s. Time: %s.", link, reason, time.Now())

	ign.LoggerFromRequest(r).Info(logLine)

	err := SendEmail(&recipient, &sender, subject, templateFilename, templateData)

	if err != nil {
		return nil, err
	}

	return nil, nil
}

// SendEmail sends a generic email from HTML template
func SendEmail(recipient *string, sender *string, subject string, templateFilename string,
	templateData interface{}) *ign.ErrMsg {

	if recipient == nil {
		recipient = &globals.FlagsEmailRecipient
	}

	if sender == nil {
		sender = &globals.FlagsEmailSender
	}

	// If the sender or recipient are not defined, then don't send the email
	if (recipient != nil && len(*recipient) == 0) || (sender != nil && *sender == "") {
		return nil
	}

	// Prepare the template
	content, err := ign.ParseHTMLTemplate(templateFilename, templateData)
	if err != nil {
		return ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	// Send the email
	err = ign.SendEmail(*sender, *recipient, subject, content)
	if err != nil {
		return ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	return nil
}
