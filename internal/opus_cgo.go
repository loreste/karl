//go:build cgo && opus

package internal

/*
#cgo pkg-config: opus
#include <opus/opus.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// OpusAvailable indicates whether real Opus codec support is compiled in.
const OpusAvailable = true

// CGOOpusEncoder wraps the real libopus encoder.
type CGOOpusEncoder struct {
	encoder    *C.OpusEncoder
	sampleRate int
	channels   int
}

// NewCGOOpusEncoder creates a real Opus encoder via libopus.
func NewCGOOpusEncoder(sampleRate, channels int) (*CGOOpusEncoder, error) {
	var err C.int
	encoder := C.opus_encoder_create(C.opus_int32(sampleRate), C.int(channels), C.OPUS_APPLICATION_VOIP, &err)
	if err != C.OPUS_OK {
		return nil, fmt.Errorf("opus_encoder_create failed: %d", int(err))
	}
	return &CGOOpusEncoder{encoder: encoder, sampleRate: sampleRate, channels: channels}, nil
}

// Encode encodes PCM samples to Opus.
func (e *CGOOpusEncoder) Encode(pcm []int16, frameSize int) ([]byte, error) {
	if len(pcm) == 0 {
		return nil, fmt.Errorf("empty PCM input")
	}
	maxOut := 4000
	out := make([]byte, maxOut)
	n := C.opus_encode(e.encoder,
		(*C.opus_int16)(unsafe.Pointer(&pcm[0])),
		C.int(frameSize),
		(*C.uchar)(unsafe.Pointer(&out[0])),
		C.opus_int32(maxOut))
	if n < 0 {
		return nil, fmt.Errorf("opus_encode failed: %d", int(n))
	}
	return out[:int(n)], nil
}

// Close releases the encoder.
func (e *CGOOpusEncoder) Close() {
	if e.encoder != nil {
		C.opus_encoder_destroy(e.encoder)
		e.encoder = nil
	}
}

// CGOOpusDecoder wraps the real libopus decoder.
type CGOOpusDecoder struct {
	decoder    *C.OpusDecoder
	sampleRate int
	channels   int
}

// NewCGOOpusDecoder creates a real Opus decoder via libopus.
func NewCGOOpusDecoder(sampleRate, channels int) (*CGOOpusDecoder, error) {
	var err C.int
	decoder := C.opus_decoder_create(C.opus_int32(sampleRate), C.int(channels), &err)
	if err != C.OPUS_OK {
		return nil, fmt.Errorf("opus_decoder_create failed: %d", int(err))
	}
	return &CGOOpusDecoder{decoder: decoder, sampleRate: sampleRate, channels: channels}, nil
}

// Decode decodes Opus to PCM samples.
func (d *CGOOpusDecoder) Decode(data []byte, frameSize int) ([]int16, error) {
	pcm := make([]int16, frameSize*d.channels)
	var dataPtr *C.uchar
	dataLen := C.opus_int32(0)
	if len(data) > 0 {
		dataPtr = (*C.uchar)(unsafe.Pointer(&data[0]))
		dataLen = C.opus_int32(len(data))
	}
	n := C.opus_decode(d.decoder, dataPtr, dataLen,
		(*C.opus_int16)(unsafe.Pointer(&pcm[0])),
		C.int(frameSize), 0)
	if n < 0 {
		return nil, fmt.Errorf("opus_decode failed: %d", int(n))
	}
	return pcm[:int(n)*d.channels], nil
}

// Close releases the decoder.
func (d *CGOOpusDecoder) Close() {
	if d.decoder != nil {
		C.opus_decoder_destroy(d.decoder)
		d.decoder = nil
	}
}
