package ump_parser

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestDecodeKeepsTwoCompleteMediaParts(t *testing.T) {
	got, err := DecodeUMPFile([]byte{21, 3, 0, 'a', 'b', 21, 3, 0, 'c', 'd'})
	if err != nil || !bytes.Equal(got, []byte("abcd")) {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestDecodeRejectsCutoffAfterValidMedia(t *testing.T) {
	for name, tail := range map[string][]byte{
		"split type":     {0x80},
		"missing length": {21},
		"split length":   {21, 0x80},
		"split payload":  {21, 5, 0, 'c'},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := DecodeUMPFile(append([]byte{21, 3, 0, 'a', 'b'}, tail...))
			if !errors.Is(err, io.ErrUnexpectedEOF) || got != nil {
				t.Fatalf("expected truncation error and no partial output; got %q, %v", got, err)
			}
		})
	}
}

func TestDecodeRejectsEmptyMediaWithoutPanic(t *testing.T) {
	if got, err := DecodeUMPFile([]byte{21, 0}); err == nil || got != nil {
		t.Fatalf("expected invalid part error and no output; got %q, %v", got, err)
	}
}
