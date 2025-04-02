#!/usr/bin/env python3
import socket
import time
import sys

def send_rtp_packet(host, port):
    # Create a basic RTP packet
    packet = bytearray([
        0x80, 0x00, 0x00, 0x01,  # RTP header (version, padding, extension, CSRC count, marker, payload type)
        0x00, 0x00, 0x00, 0x01,  # Timestamp
        0x00, 0x00, 0x00, 0x01,  # SSRC
        # Payload (16 bytes)
        0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
        0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10
    ])
    
    # Create UDP socket
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    
    # Send 5 packets
    for i in range(5):
        print(f"Sending packet {i+1}...")
        sock.sendto(packet, (host, port))
        
        # Increment sequence number
        packet[2] += 1
        # Increment timestamp
        packet[7] += 1
        
        time.sleep(0.1)
    
    print("Done sending packets")
    sock.close()

if __name__ == "__main__":
    send_rtp_packet("127.0.0.1", 12000)