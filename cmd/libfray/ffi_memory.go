package main

/*
#include <stdlib.h>
*/
import "C"

import "unsafe"

func FrayFreeString(ptr *C.char) {
	if ptr != nil {
		C.free(unsafe.Pointer(ptr))
	}
}
