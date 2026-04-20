package provider

import (
	"bytes"
	"io"
	"os/exec"
)

// bytesReader wraps a byte slice as an io.Reader without pulling in
// bytes.NewReader everywhere. Exists so callers don't import bytes for
// a trivial conversion.
func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// lookPathImpl is the real implementation; kept in its own file so
// unit tests can stub the exported alias in binary_release.go.
func lookPathImpl(name string) (string, error) {
	return exec.LookPath(name)
}
