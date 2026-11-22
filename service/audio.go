package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"

	"github.com/braheezy/shine-mp3/pkg/mp3"
)

// Kokoro voices come out at different levels (about -19 to -25 dBFS for the same text) and quiet
// overall, so the speech is brought to one level; a soft limiter keeps the few raised peaks from clipping.
const (
	targetLevel = 0.1    // -20 dBFS RMS over the frames that carry speech
	speechGate  = 0.0032 // -50 dBFS: frames below this are pauses and do not count
	maxGain     = 4.0    // +12 dB at most, so near-silence is never blown up
	limitKnee   = 0.8    // peaks above this are bent smoothly toward 1.0 instead of clipping
)

func normalize(s []float32, rate int) []float32 {
	frame := max(1, rate/50) // 20 ms
	var sum float64
	n := 0
	for i := 0; i < len(s); i += frame {
		var fs float64
		end := min(len(s), i+frame)
		for _, x := range s[i:end] {
			fs += float64(x) * float64(x)
		}
		if math.Sqrt(fs/float64(end-i)) > speechGate {
			sum, n = sum+fs, n+end-i
		}
	}
	if n == 0 {
		return s // nothing but silence
	}
	gain := float32(min(maxGain, targetLevel/math.Sqrt(sum/float64(n))))
	out := make([]float32, len(s))
	for i, x := range s {
		out[i] = softLimit(x * gain)
	}
	return out
}

func softLimit(x float32) float32 {
	a := math.Abs(float64(x))
	if a <= limitKnee {
		return x
	}
	y := limitKnee + (1-limitKnee)*math.Tanh((a-limitKnee)/(1-limitKnee))
	return float32(math.Copysign(y, float64(x)))
}

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
