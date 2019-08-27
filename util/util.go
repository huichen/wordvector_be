package util

import (
	"encoding/binary"
	"math"
)


func Float32frombytes(bytes []byte) float32 {
	bits := binary.LittleEndian.Uint32(bytes)
	float := math.Float32frombits(bits)
	return float
}

func Float32bytes(float float32) []byte {
	bits := math.Float32bits(float)
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint32(bytes, bits)
	return bytes
}

func Uint32frombytes(bytes []byte) uint32 {
	return binary.LittleEndian.Uint32(bytes)
}

func Uint32bytes(value uint32) []byte {
    bs := make([]byte, 4)
    binary.LittleEndian.PutUint32(bs, value)
	return bs
}
