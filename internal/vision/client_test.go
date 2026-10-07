package vision

import "testing"

func TestLooksLikeCaptchaQuestion(t *testing.T) {
	if !looksLikeCaptchaQuestion("识别validate码，只输出字符") {
		t.Fatal("expected captcha hint")
	}
	if looksLikeCaptchaQuestion("description登录页布局") {
		t.Fatal("expected non-captcha")
	}
}
