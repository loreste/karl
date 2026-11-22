//go:build cgo && g729

package internal

/*
#cgo pkg-config: bcg729
#cgo LDFLAGS: -lbcg729
#include <bcg729/encoder.h>
#include <bcg729/decoder.h>
*/
import "C"

import "unsafe"

// G729Available indicates whether real G.729 codec support is compiled in.
const G729Available = true

// CGOg729Encoder wraps the real bcg729 encoder.
type CGOg729Encoder struct {
	ctx *C.bcg729EncoderChannelContextStruct
}

// NewCGOg729Encoder creates a real G.729 encoder via bcg729.
func NewCGOg729Encoder() *CGOg729Encoder {
	return &CGOg729Encoder{ctx: C.initBcg729EncoderChannel(0)}
}

// Encode encodes 80 int16 PCM samples (10ms @ 8kHz) to 10 bytes of G.729.
func (e *CGOg729Encoder) Encode(pcm []int16) []byte {
	if len(pcm) < G729FrameSamples {
		return nil
	}
	out := make([]byte, G729FrameSize)
	var outLen C.uint8_t
	C.bcg729Encoder(
		e.ctx,
		(*C.int16_t)(unsafe.Pointer(&pcm[0])),
		(*C.uint8_t)(unsafe.Pointer(&out[0])),
		&outLen,
	)
	return out[:int(outLen)]
}

// Close releases the encoder resources.
func (e *CGOg729Encoder) Close() {
	if e.ctx != nil {
		C.closeBcg729EncoderChannel(e.ctx)
		e.ctx = nil
	}
}

// CGOg729Decoder wraps the real bcg729 decoder.
type CGOg729Decoder struct {
	ctx *C.bcg729DecoderChannelContextStruct
}

// NewCGOg729Decoder creates a real G.729 decoder via bcg729.
func NewCGOg729Decoder() *CGOg729Decoder {
	return &CGOg729Decoder{ctx: C.initBcg729DecoderChannel()}
}

// Decode decodes 10 bytes of G.729 to 80 int16 PCM samples.
func (d *CGOg729Decoder) Decode(data []byte) []int16 {
	if len(data) < G729FrameSize {
		return nil
	}
	pcm := make([]int16, G729FrameSamples)
	C.bcg729Decoder(
		d.ctx,
		(*C.uint8_t)(unsafe.Pointer(&data[0])),
		C.uint8_t(len(data)),
		0,
		0,
		0,
		(*C.int16_t)(unsafe.Pointer(&pcm[0])),
	)
	return pcm
}

// Close releases the decoder resources.
func (d *CGOg729Decoder) Close() {
	if d.ctx != nil {
		C.closeBcg729DecoderChannel(d.ctx)
		d.ctx = nil
	}
}
