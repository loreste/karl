package internal

import (
	"fmt"
	"log"
	"os/exec"
)

// Supported codecs for transcoding
var SupportedCodecs = []string{"opus", "g722", "pcmu", "pcma", "aac", "vp8", "h264"}

// TranscodeRTP converts RTP media from one codec to another using FFmpeg
func TranscodeRTP(inputAddr, outputAddr, inputCodec, outputCodec string) error {
	if !isCodecSupported(inputCodec) || !isCodecSupported(outputCodec) {
		return fmt.Errorf("unsupported codec: %s -> %s", inputCodec, outputCodec)
	}

	cmd := exec.Command("ffmpeg",
		"-i", fmt.Sprintf("rtp://%s", inputAddr),
		"-acodec", outputCodec,
		"-f", "rtp",
		fmt.Sprintf("rtp://%s", outputAddr),
	)

	log.Printf("Starting RTP transcoding: %s -> %s", inputCodec, outputCodec)
	return cmd.Run()
}

// isCodecSupported checks if a given codec is supported
func isCodecSupported(codec string) bool {
	for _, c := range SupportedCodecs {
		if c == codec {
			return true
		}
	}
	return false
}
