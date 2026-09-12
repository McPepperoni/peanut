package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

func WriteWAV(w io.Writer, audio Audio) error {
	if audio.SampleRate != SampleRate || audio.Channels != Channels {
		return errors.New("audio must be 16 kHz mono")
	}
	dataSize := uint32(len(audio.Samples) * 4)
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataSize)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 3)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], SampleRate)
	binary.LittleEndian.PutUint32(header[28:32], SampleRate*4)
	binary.LittleEndian.PutUint16(header[32:34], 4)
	binary.LittleEndian.PutUint16(header[34:36], 32)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataSize)
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write WAV header: %w", err)
	}
	for _, sample := range audio.Samples {
		if err := binary.Write(w, binary.LittleEndian, sample); err != nil {
			return fmt.Errorf("write WAV samples: %w", err)
		}
	}
	return nil
}

func ReadWAV(r io.Reader) (Audio, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return Audio{}, fmt.Errorf("read WAV: %w", err)
	}
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return Audio{}, errors.New("invalid WAV header")
	}
	var format, channels, bits uint16
	var rate uint32
	var data []byte
	for offset := 12; offset+8 <= len(b); {
		size := int(binary.LittleEndian.Uint32(b[offset+4 : offset+8]))
		start, end := offset+8, offset+8+size
		if end > len(b) {
			return Audio{}, errors.New("truncated WAV chunk")
		}
		switch string(b[offset : offset+4]) {
		case "fmt ":
			if size < 16 {
				return Audio{}, errors.New("invalid WAV format chunk")
			}
			format = binary.LittleEndian.Uint16(b[start : start+2])
			channels = binary.LittleEndian.Uint16(b[start+2 : start+4])
			rate = binary.LittleEndian.Uint32(b[start+4 : start+8])
			bits = binary.LittleEndian.Uint16(b[start+14 : start+16])
		case "data":
			data = b[start:end]
		}
		offset = end + size%2
	}
	if rate != SampleRate || channels != Channels || (format != 1 && format != 3) {
		return Audio{}, errors.New("WAV must be 16 kHz mono PCM or float32")
	}
	var samples []float32
	switch {
	case format == 3 && bits == 32:
		if len(data)%4 != 0 {
			return Audio{}, errors.New("invalid float32 WAV data")
		}
		samples = make([]float32, len(data)/4)
		for i := range samples {
			samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
		}
	case format == 1 && bits == 16:
		if len(data)%2 != 0 {
			return Audio{}, errors.New("invalid PCM16 WAV data")
		}
		samples = make([]float32, len(data)/2)
		for i := range samples {
			samples[i] = float32(int16(binary.LittleEndian.Uint16(data[i*2:]))) / 32768
		}
	default:
		return Audio{}, errors.New("WAV must use 16-bit PCM or 32-bit float samples")
	}
	return NewAudio(int(rate), int(channels), samples)
}
