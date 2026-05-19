package usage

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/klauspost/compress/zstd"
)

const requestLogContentCompression = "zstd"

var (
	requestLogContentBytes atomic.Int64 // total compressed bytes; -1 means unknown

	zstdEncoderPool = sync.Pool{
		New: func() any {
			encoder, err := zstd.NewWriter(nil)
			if err != nil {
				panic(err)
			}
			return encoder
		},
	}
	zstdDecoderPool = sync.Pool{
		New: func() any {
			decoder, err := zstd.NewReader(nil)
			if err != nil {
				panic(err)
			}
			return decoder
		},
	}
)

func init() {
	requestLogContentBytes.Store(-1)
}

func stopRequestLogMaintenance() {
}

func compressLogContent(content string) ([]byte, error) {
	if content == "" {
		return []byte{}, nil
	}
	encoder := zstdEncoderPool.Get().(*zstd.Encoder)
	defer zstdEncoderPool.Put(encoder)
	return encoder.EncodeAll([]byte(content), make([]byte, 0, len(content)/2)), nil
}

func decompressLogContent(compression string, content []byte) (string, error) {
	if len(content) == 0 {
		return "", nil
	}
	switch compression {
	case "", requestLogContentCompression:
		decoder := zstdDecoderPool.Get().(*zstd.Decoder)
		defer zstdDecoderPool.Put(decoder)
		decoded, err := decoder.DecodeAll(content, nil)
		if err != nil {
			return "", fmt.Errorf("usage: decompress content: %w", err)
		}
		return string(decoded), nil
	default:
		return "", fmt.Errorf("usage: unsupported content compression %q", compression)
	}
}

// RequestLogContentBytes returns the cached total compressed content bytes.
// Returns -1 if unknown.
func RequestLogContentBytes() int64 {
	return requestLogContentBytes.Load()
}

// UpdateRequestLogContentBytesDelta adjusts the cached total by delta (positive or negative).
func UpdateRequestLogContentBytesDelta(delta int64) {
	requestLogContentBytes.Add(delta)
}

// InvalidateRequestLogContentBytes forces a refresh on next read.
func InvalidateRequestLogContentBytes() {
	requestLogContentBytes.Store(-1)
}
