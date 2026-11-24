package ump_parser

import (
	"fmt"
	"io"
)

// UMP part type for media payload (per protos: UMPPartId.MEDIA = 21)
const umpPartTypeMedia = 21

// UMPReader parses UMP-encoded data
type UMPPart struct {
	Type int
	Size int
	Data []byte
}

type UMPReader struct {
	Buf []byte
	Pos int
}

func (r *UMPReader) readVarInt() (int, error) {
	if r.Pos >= len(r.Buf) {
		return -1, io.EOF
	}
	first := r.Buf[r.Pos]
	var value int
	var length int
	if first < 128 {
		value = int(first)
		length = 1
	} else if first < 192 {
		if r.Pos+1 >= len(r.Buf) {
			return -1, io.EOF
		}
		value = int(first&0x3f) + 64*int(r.Buf[r.Pos+1])
		length = 2
	} else if first < 224 {
		if r.Pos+2 >= len(r.Buf) {
			return -1, io.EOF
		}
		value = int(first&0x1f) + 32*(int(r.Buf[r.Pos+1])+256*int(r.Buf[r.Pos+2]))
		length = 3
	} else if first < 240 {
		if r.Pos+3 >= len(r.Buf) {
			return -1, io.EOF
		}
		value = int(first&0x0f) + 16*(int(r.Buf[r.Pos+1])+256*(int(r.Buf[r.Pos+2])+256*int(r.Buf[r.Pos+3])))
		length = 4
	} else {
		if r.Pos+4 >= len(r.Buf) {
			return -1, io.EOF
		}
		value = int(first&0x07) + 8*(int(r.Buf[r.Pos+1])+256*(int(r.Buf[r.Pos+2])+256*(int(r.Buf[r.Pos+3])+256*int(r.Buf[r.Pos+4]))))
		length = 5
	}
	r.Pos += length
	return value, nil
}

func (r *UMPReader) NextPart() (*UMPPart, error) {
	if r.Pos >= len(r.Buf) {
		return nil, io.EOF
	}
	start := r.Pos
	typeVal, err := r.readVarInt()
	if err != nil {
		return nil, fmt.Errorf("UMP part type at byte %d: %w", start, io.ErrUnexpectedEOF)
	}
	sizeVal, err := r.readVarInt()
	if err != nil || sizeVal < 0 || sizeVal > len(r.Buf)-r.Pos {
		return nil, fmt.Errorf("UMP part length or payload at byte %d: %w", start, io.ErrUnexpectedEOF)
	}
	data := r.Buf[r.Pos : r.Pos+sizeVal]
	r.Pos += sizeVal
	return &UMPPart{Type: typeVal, Size: sizeVal, Data: data}, nil
}

func DecodeUMPFile(input []byte) ([]byte, error) {
	reader := &UMPReader{Buf: input}
	var mediaData []byte
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if part.Type == umpPartTypeMedia {
			payload := part.Data
			if len(payload) == 0 {
				return nil, fmt.Errorf("empty UMP media part at byte %d", reader.Pos)
			}
			if payload[0] == 0x00 {
				payload = payload[1:]
			}
			mediaData = append(mediaData, payload...)
		}
	}
	return mediaData, nil
}
