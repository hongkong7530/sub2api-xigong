package logredact

import (
	"strings"
	"testing"
)

func TestDevinCredentialInputsAreRedacted(t *testing.T) {
	body := []byte(`{"credentials":{"session_token":"fixture-session-secret","credentials_content":"windsurf_api_key='fixture-toml-secret'","windsurf_api_key":"fixture-alias-secret"},"name":"safe-account"}`)
	redacted := RedactJSON(body)
	for _, value := range []string{"fixture-session-secret", "fixture-toml-secret", "fixture-alias-secret"} {
		if strings.Contains(redacted, value) {
			t.Fatal("Devin input credential leaked through shared redactor")
		}
	}
	if !strings.Contains(redacted, "safe-account") {
		t.Fatal("non-secret account label was lost")
	}
}
