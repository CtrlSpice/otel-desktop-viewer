package evaldecode

import "fmt"

// Detect selects a decoder by envelope structure and producer version. Input is
// bounded, validated context, with the streamed results array replaced by [].
func Detect(context []byte) (string, error) {
	// Keep the registry small: add a recognizer only alongside its implemented
	// producer reader. Filenames never participate in selection.
	decoders := []func([]byte) (string, error){promptfooFormat}
	var reason error
	for _, detect := range decoders {
		format, err := detect(context)
		if err == nil {
			return format, nil
		}
		reason = err
	}
	return "", fmt.Errorf("unsupported evaluation envelope: %w", reason)
}
