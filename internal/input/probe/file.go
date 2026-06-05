package probe

import (
	"fmt"
	"os"
)

type FileInfo struct {
	Path   string
	Size   int64
	Header []byte
}

func ReadFileInfo(path string, headerBytes int) (*FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file %q: %w", path, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file %q: %w", path, err)
	}

	if headerBytes < 0 {
		headerBytes = 0
	}
	header := make([]byte, headerBytes)
	n, err := file.Read(header)
	if err != nil && n == 0 {
		return nil, fmt.Errorf("read header from %q: %w", path, err)
	}

	return &FileInfo{
		Path:   path,
		Size:   stat.Size(),
		Header: header[:n],
	}, nil
}
