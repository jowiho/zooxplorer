package snapshot

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

type decoder struct {
	r   *bufio.Reader
	off int64
}

func newDecoder(r io.Reader) *decoder {
	return &decoder{r: bufio.NewReader(r)}
}

func (d *decoder) Offset() int64 {
	return d.off
}

func (d *decoder) readN(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(d.r, buf); err != nil {
		return nil, d.wrapErr(err)
	}
	d.off += int64(n)
	return buf, nil
}

func (d *decoder) ReadInt32() (int32, error) {
	b, err := d.readN(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(b)), nil
}

func (d *decoder) ReadInt64() (int64, error) {
	b, err := d.readN(8)
	if err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(b)), nil
}

func (d *decoder) ReadString(maxLen int32) (string, error) {
	l, err := d.ReadInt32()
	if err != nil {
		return "", err
	}
	if l < 0 {
		return "", fmt.Errorf("invalid string length %d at offset %d", l, d.off-4)
	}
	if l > maxLen {
		return "", fmt.Errorf("string length %d exceeds limit %d at offset %d", l, maxLen, d.off-4)
	}
	b, err := d.readN(int(l))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (d *decoder) ReadBuffer(maxLen int32) ([]byte, error) {
	l, err := d.ReadInt32()
	if err != nil {
		return nil, err
	}
	if l == -1 {
		return nil, nil
	}
	if l < -1 {
		return nil, fmt.Errorf("invalid buffer length %d at offset %d", l, d.off-4)
	}
	if l > maxLen {
		return nil, fmt.Errorf("buffer length %d exceeds limit %d at offset %d", l, maxLen, d.off-4)
	}
	return d.readN(int(l))
}

func (d *decoder) wrapErr(err error) error {
	return fmt.Errorf("decode failed at offset %d: %w", d.off, err)
}
