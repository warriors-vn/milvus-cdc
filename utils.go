package milvus_cdc

import "unsafe"

func DecodeUnsafeF32(bs []byte) []float32 {
	if len(bs) == 0 {
		return nil
	}

	return unsafe.Slice((*float32)(unsafe.Pointer(&bs[0])), len(bs)/4)
}
