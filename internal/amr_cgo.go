//go:build cgo && amr

package internal

/*
#cgo pkg-config: opencore-amrnb
#cgo LDFLAGS: -lopencore-amrnb
#include <opencore-amrnb/interf_enc.h>
#include <opencore-amrnb/interf_dec.h>
*/
import "C"

import "unsafe"

// AMRAvailable indicates whether real AMR codec support is compiled in.
const AMRAvailable = true

// AMR mode constants matching opencore-amr enum Mode.
const (
	AMRMR475 = 0
	AMRMR515 = 1
	AMRMR59  = 2
	AMRMR67  = 3
	AMRMR74  = 4
	AMRMR795 = 5
	AMRMR102 = 6
	AMRMR122 = 7
)

// CGOAMREncoder wraps the real opencore-amr encoder.
type CGOAMREncoder struct {
	state unsafe.Pointer
	mode  int
}

// NewCGOAMREncoder creates a real AMR-NB encoder. Mode is 0-7 (MR475..MR122).
func NewCGOAMREncoder(mode int) *CGOAMREncoder {
	return &CGOAMREncoder{
		state: unsafe.Pointer(C.Encoder_Interface_new(0)),
		mode:  mode,
	}
}

// Encode encodes 160 int16 PCM samples (20ms @ 8kHz) to an AMR frame.
func (e *CGOAMREncoder) Encode(pcm []int16) []byte {
	if len(pcm) < AMRNBFrameSamples {
		return nil
	}
	out := make([]byte, 64) // max AMR frame is ~32 bytes, 64 is safe
	n := C.Encoder_Interface_Encode(
		e.state,
		C.enum_Mode(e.mode),
		(*C.short)(unsafe.Pointer(&pcm[0])),
		(*C.uchar)(unsafe.Pointer(&out[0])),
		0,
	)
	return out[:int(n)]
}

// Close releases the encoder resources.
func (e *CGOAMREncoder) Close() {
	if e.state != nil {
		C.Encoder_Interface_exit(e.state)
		e.state = nil
	}
}

// CGOAMRDecoder wraps the real opencore-amr decoder.
type CGOAMRDecoder struct {
	state unsafe.Pointer
}

// NewCGOAMRDecoder creates a real AMR-NB decoder.
func NewCGOAMRDecoder() *CGOAMRDecoder {
	return &CGOAMRDecoder{
		state: unsafe.Pointer(C.Decoder_Interface_new()),
	}
}

// Decode decodes an AMR frame to 160 int16 PCM samples.
func (d *CGOAMRDecoder) Decode(data []byte) []int16 {
	if len(data) == 0 {
		return nil
	}
	pcm := make([]int16, AMRNBFrameSamples)
	C.Decoder_Interface_Decode(
		d.state,
		(*C.uchar)(unsafe.Pointer(&data[0])),
		(*C.short)(unsafe.Pointer(&pcm[0])),
		0,
	)
	return pcm
}

// Close releases the decoder resources.
func (d *CGOAMRDecoder) Close() {
	if d.state != nil {
		C.Decoder_Interface_exit(d.state)
		d.state = nil
	}
}
