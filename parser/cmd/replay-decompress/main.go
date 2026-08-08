// Decompress a Valve replay download (bz2 and/or zstd) to raw PBDEMS2 on stdout.
package main

import (
	"bytes"
	"compress/bzip2"
	"fmt"
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
)

var (
	magicPBDEMS2 = []byte("PBDEMS2\x00")
	magicBZ2     = []byte("BZh")
	magicZstd    = []byte{0x28, 0xb5, 0x2f, 0xfd}
)

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatal(err)
	}
	out, err := decompressReplay(data)
	if err != nil {
		fatal(err)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fatal(err)
	}
}

func decompressReplay(data []byte) ([]byte, error) {
	for step := 0; step < 3; step++ {
		switch {
		case bytes.HasPrefix(data, magicPBDEMS2):
			return data, nil
		case bytes.HasPrefix(data, magicBZ2):
			r, err := decompressBZ2(data)
			if err != nil {
				return nil, err
			}
			data = r
		case bytes.HasPrefix(data, magicZstd):
			r, err := decompressZstd(data)
			if err != nil {
				return nil, err
			}
			data = r
		default:
			return nil, fmt.Errorf("unknown replay compression (magic %q)", data[:min(8, len(data))])
		}
	}
	return nil, fmt.Errorf("replay still compressed after decompression steps")
}

func decompressBZ2(data []byte) ([]byte, error) {
	out, err := io.ReadAll(bzip2.NewReader(bytes.NewReader(data)))
	if err != nil {
		return nil, fmt.Errorf("bz2: %w", err)
	}
	return out, nil
}

func decompressZstd(data []byte) ([]byte, error) {
	dec, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("zstd: %w", err)
	}
	defer dec.Close()
	out, err := io.ReadAll(dec)
	if err != nil {
		return nil, fmt.Errorf("zstd: %w", err)
	}
	return out, nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "replay-decompress: %v\n", err)
	os.Exit(1)
}
