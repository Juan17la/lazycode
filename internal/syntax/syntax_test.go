package syntax

import "testing"

func TestDefaultLanguagesParse(t *testing.T) {
	for _, tc := range []struct{ language, good, bad string }{
		{"go", "package a\nfunc b() {}\n", "package a\nfunc {{{\n"},
		{"javascript", "function a() { return 1; }\n", "function ((( {\n"},
		{"python", "def a():\n    return 1\n", "def (((:\n"},
		{"typescript", "function a(b: number): number { return b; }\n", "function a(b: : { \n"},
	} {
		if hasErrors, err := Check(tc.language, []byte(tc.good)); err != nil || hasErrors {
			t.Errorf("%s valid code: hasErrors=%v err=%v", tc.language, hasErrors, err)
		}
		if hasErrors, err := Check(tc.language, []byte(tc.bad)); err != nil || !hasErrors {
			t.Errorf("%s broken code: hasErrors=%v err=%v", tc.language, hasErrors, err)
		}
	}
}

func TestUnknownLanguageIsAnError(t *testing.T) {
	if _, err := Check("cobol", nil); err == nil {
		t.Error("want an error")
	}
}
