package auth

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"html/template"
)

//go:embed callback.html
var callbackHTMLTemplate string

//go:embed favicon.svg
var faviconSVG string

var callbackTemplate = template.Must(template.New("callback").Parse(callbackHTMLTemplate))

var successHTML = renderCallbackPage("authenticated", "authentication successful", "you can close this window and return to your terminal.", true)
var deniedHTML = renderCallbackPage("authorization denied", "authorization denied", "no new access was granted. you can close this window and return to your terminal.", false)

func renderCallbackPage(title, heading, message string, success bool) string {
	var page bytes.Buffer
	err := callbackTemplate.Execute(&page, struct {
		Title, Heading, Message string
		Success                 bool
		Favicon                 template.URL
	}{
		Title:   title,
		Heading: heading,
		Message: message,
		Success: success,
		// The embedded icon must survive shutdown of the callback server.
		Favicon: template.URL("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(faviconSVG))),
	})
	if err != nil {
		panic(err)
	}
	return page.String()
}
