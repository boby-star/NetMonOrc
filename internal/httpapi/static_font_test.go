package httpapi

import (
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestLocalFontAssetsDoNotExposeOtherFiles(t *testing.T) {
	h := &Handler{ui: fstest.MapFS{
		"assets/manrope.ttf": {Data: []byte("test-font")},
		"assets/private.key": {Data: []byte("private")},
		"private.ttf":        {Data: []byte("private")},
	}}
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/assets/manrope.ttf", 200},
		{"GET", "/assets/private.key", 404},
		{"GET", "/private.ttf", 404},
		{"GET", "/assets/../private.ttf", 404},
		{"POST", "/assets/manrope.ttf", 404},
	} {
		w := httptest.NewRecorder()
		h.static(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
		if w.Code == 200 && w.Header().Get("Content-Type") != "font/ttf" {
			t.Fatal("font MIME missing")
		}
	}
}
