package notify

import "testing"

func TestEscapeQuotesAndBackslashes(t *testing.T) {
	got := escape(`say "hi" \ bye`)
	want := `say \"hi\" \\ bye`
	if got != want {
		t.Fatalf("escape() = %q, want %q", got, want)
	}
}

func TestEscapeControlCharacters(t *testing.T) {
	got := escape("line1\nline2\rline3\tline4")
	want := `line1\nline2\rline3\tline4`
	if got != want {
		t.Fatalf("escape() = %q, want %q", got, want)
	}
}

func TestEscapeStripsOtherControlCharacters(t *testing.T) {
	got := escape("bad\x00path\x07here")
	want := "badpathhere"
	if got != want {
		t.Fatalf("escape() = %q, want %q", got, want)
	}
}
