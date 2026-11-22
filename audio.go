package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"

	"github.com/braheezy/shine-mp3/pkg/mp3"
)

func toPCM16(samples []float32) []int16 {
	out := make([]int16, len(samples))
	for i, s := range samples {
		out[i] = int16(math.Round(float64(max(-1, min(1, s))) * math.MaxInt16))
	}
	return out
}

func writeWAV(w io.Writer, samples []float32, rate int) error {
	pcm := toPCM16(samples)
	size := uint32(len(pcm) * 2)
	header := struct {
		Riff     [4]byte
		Size     uint32
		Wave     [4]byte
		Fmt      [4]byte
		FmtSize  uint32
		Format   uint16
		Channels uint16
		Rate     uint32
		ByteRate uint32
		Align    uint16
		Bits     uint16
		Data     [4]byte
		DataSize uint32
	}{[4]byte{'R', 'I', 'F', 'F'}, 36 + size, [4]byte{'W', 'A', 'V', 'E'}, [4]byte{'f', 'm', 't', ' '},
		16, 1, 1, uint32(rate), uint32(rate * 2), 2, 16, [4]byte{'d', 'a', 't', 'a'}, size}
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, header)
	binary.Write(&b, binary.LittleEndian, pcm)
	_, err := w.Write(b.Bytes())
	return err
}

func writeMP3(w io.Writer, samples []float32, rate int) error {
	pcm := toPCM16(samples)
	// The encoder works in 1152-sample frames; a short last frame would encode stale buffer data.
	if r := len(pcm) % 1152; r != 0 {
		pcm = append(pcm, make([]int16, 1152-r)...)
	}
	return mp3.NewEncoder(rate, 1).Write(w, pcm)
}
