package httpapi

import (
	"github.com/boby-star/NetMonOrc/internal/resources"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResourceHTTPRejectsDuplicateKeysBeforeTypedDecode(t *testing.T) {
	for _, body := range []string{
		`{"apiVersion":"netmonorc.io/v1alpha1","kind":"Target","kind":"Monitor","metadata":{"name":"x"},"spec":{}}`,
		`{"apiVersion":"netmonorc.io/v1alpha1","kind":"Target","metadata":{"name":"x"},"spec":{"address":"192.0.2.1","address":"127.0.0.1"}}`,
	} {
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		var m resources.Manifest
		if resourceJSON(w, req, &m) || w.Code != 400 {
			t.Fatal("ambiguous HTTP resource accepted")
		}
	}
}
