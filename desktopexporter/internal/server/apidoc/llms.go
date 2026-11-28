package apidoc

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/skills"
)

const revalidateCacheControl = "no-cache"

// LLMsTextHandler serves the agent inspection guide at /llms.txt with an ETag.
func LLMsTextHandler() http.Handler {
	guide := skills.Guide()
	sum := sha256.Sum256([]byte(guide))
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.Header().Set("Cache-Control", revalidateCacheControl)
		writer.Header().Set("ETag", etag)
		http.ServeContent(writer, request, "llms.txt", time.Time{}, strings.NewReader(guide))
	})
}
