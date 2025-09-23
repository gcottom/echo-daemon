package ump_parser

import "io"

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
	typeVal, err := r.readVarInt()
	if err != nil {
		return nil, err
	}
	sizeVal, err := r.readVarInt()
	if err != nil {
		return nil, err
	}
	if r.Pos+sizeVal > len(r.Buf) {
		return nil, io.EOF
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
			if part.Data[0] == 0x00 {
				part.Data = part.Data[1:]
			}
			mediaData = append(mediaData, part.Data...)
		}
	}
	return mediaData, nil
}
