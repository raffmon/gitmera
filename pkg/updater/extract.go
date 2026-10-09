package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path/filepath"
)

func ExtractBinary(archive []byte, binaryName string) ([]byte, error) {
	return extractFromTarGz(archive, binaryName)
}

func extractFromTarGz(data []byte, binaryName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to open gzip archive: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar archive: %w", err)
		}
		if filepath.Base(hdr.Name) == binaryName {
			data, err := readLimited(tr, defaultMaxDownloadSize)
			if err != nil {
				return nil, fmt.Errorf("failed to read %q from archive: %w", binaryName, err)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", binaryName)
}
