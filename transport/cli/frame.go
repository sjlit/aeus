package cli

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"time"
)

const (
	PacketTypeCompleter byte = 0x01
	PacketTypeCommand   byte = 0x02
	PacketTypeHandshake byte = 0x03
)

const (
	FlagPortion  = 0x00
	FlagComplete = 0x01
)

type (
	Frame struct {
		Feature   []byte
		Type      byte   `json:"type"`
		Flag      byte   `json:"flag"`
		Seq       uint16 `json:"seq"`
		Data      []byte `json:"data"`
		Error     string `json:"error"`
		Timeout   int64  `json:"timeout"`
		Timestamp int64  `json:"timestamp"`
	}
)

func readFrame(r io.Reader) (frame *Frame, err error) {
	var (
		n           int
		dataLength  uint16
		errorLength uint16
		errBuf      []byte
	)
	frame = &Frame{Feature: make([]byte, 3)}
	if _, err = io.ReadFull(r, frame.Feature); err != nil {
		return
	}
	if !bytes.Equal(frame.Feature, Feature) {
		err = io.ErrUnexpectedEOF
		return
	}
	if err = binary.Read(r, binary.LittleEndian, &frame.Type); err != nil {
		return
	}
	if err = binary.Read(r, binary.LittleEndian, &frame.Flag); err != nil {
		return
	}
	if err = binary.Read(r, binary.LittleEndian, &frame.Seq); err != nil {
		return
	}
	if err = binary.Read(r, binary.LittleEndian, &frame.Timeout); err != nil {
		return
	}
	if err = binary.Read(r, binary.LittleEndian, &frame.Timestamp); err != nil {
		return
	}
	if err = binary.Read(r, binary.LittleEndian, &dataLength); err != nil {
		return
	}
	if err = binary.Read(r, binary.LittleEndian, &errorLength); err != nil {
		return
	}
	if dataLength > 0 {
		frame.Data = make([]byte, dataLength)
		if n, err = io.ReadFull(r, frame.Data); err == nil {
			if n < int(dataLength) {
				err = io.ErrShortBuffer
			}
		}
	}
	if errorLength > 0 {
		errBuf = make([]byte, errorLength)
		if n, err = io.ReadFull(r, errBuf); err == nil {
			if n < int(dataLength) {
				err = io.ErrShortBuffer
			} else {
				frame.Error = string(errBuf)
			}
		}
	}
	return
}

func writeFrame(w io.Writer, frame *Frame) (err error) {
	var (
		n           int
		dl          int
		dataLength  uint16
		errorLength uint16
		errBuf      []byte
	)
	if _, err = w.Write(Feature); err != nil {
		return
	}
	if frame.Data != nil {
		dl = len(frame.Data)
		if dl > math.MaxUint16 {
			return io.ErrNoProgress
		}
		dataLength = uint16(dl)
	}
	if frame.Error != "" {
		errBuf = []byte(frame.Error)
		errorLength = uint16(len(errBuf))
	}
	if err = binary.Write(w, binary.LittleEndian, frame.Type); err != nil {
		return
	}
	if err = binary.Write(w, binary.LittleEndian, frame.Flag); err != nil {
		return
	}
	if err = binary.Write(w, binary.LittleEndian, frame.Seq); err != nil {
		return
	}
	if err = binary.Write(w, binary.LittleEndian, frame.Timeout); err != nil {
		return
	}
	if err = binary.Write(w, binary.LittleEndian, frame.Timestamp); err != nil {
		return
	}
	if err = binary.Write(w, binary.LittleEndian, dataLength); err != nil {
		return
	}
	if err = binary.Write(w, binary.LittleEndian, errorLength); err != nil {
		return
	}
	if dataLength > 0 {
		if n, err = w.Write(frame.Data); err == nil {
			if n < int(dataLength) {
				err = io.ErrShortWrite
			}
		}
	}
	if errorLength > 0 {
		if n, err = w.Write(errBuf); err == nil {
			if n < int(errorLength) {
				err = io.ErrShortWrite
			}
		}
	}
	return
}

func newFrame(t, f byte, seq uint16, timeout time.Duration, data []byte) *Frame {
	return &Frame{
		Feature:   Feature,
		Type:      t,
		Flag:      f,
		Seq:       seq,
		Data:      data,
		Timeout:   int64(timeout),
		Timestamp: time.Now().Unix(),
	}
}
