package evals

import "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/evaldecode"

// Cache records share the decoder's bounded part representation. Producer
// parsing belongs to evaldecode.
type IngestPart = evaldecode.IngestPart

const MaxRecordBytes = evaldecode.MaxRecordBytes
