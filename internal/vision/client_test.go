package vision

import "testing"

func TestLooksLikeCaptchaQuestion(t *testing.T) {
	if !looksLikeCaptchaQuestion("identify captcha, output only the characters") {
		t.Fatal("expected captcha hint")
	}
	if looksLikeCaptchaQuestion("describe login page layout") {
		t.Fatal("expected non-captcha")
	}
}
