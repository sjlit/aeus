package bytepool

import (
	"bytes"
	"sync"
)

var (
	bufferPool sync.Pool
)

func GetBuffer() *bytes.Buffer {
	if v := bufferPool.Get(); v != nil {
		return v.(*bytes.Buffer)
	}
	return bytes.NewBuffer([]byte{})
}

func PutBuffer(b *bytes.Buffer) {
	b.Reset()
	bufferPool.Put(b)
}
