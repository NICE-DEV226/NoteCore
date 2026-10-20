package bindings

import "testing"

func TestFileURL(t *testing.T) {
	for in, want := range map[string]string{
		"/tmp/notecore-open-1/cours.pdf":            "file:///tmp/notecore-open-1/cours.pdf",
		"/tmp/x/mon fichier #1.pdf":                 "file:///tmp/x/mon%20fichier%20%231.pdf",
		"C:/Users/moi/AppData/Local/Temp/a/doc.pdf": "file:///C:/Users/moi/AppData/Local/Temp/a/doc.pdf",
	} {
		if got := fileURL(in); got != want {
			t.Errorf("fileURL(%q) = %q, attendu %q", in, got, want)
		}
	}
}
