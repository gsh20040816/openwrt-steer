// SPDX-License-Identifier: GPL-3.0-or-later

package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	"github.com/gsh20040816/steer/go/pkg/steermacos"
)

func main() {}

//export SteerABIVersion
func SteerABIVersion() C.int {
	return 1
}

//export SteerValidateJSON
func SteerValidateJSON(input *C.char) *C.char {
	return C.CString(string(steermacos.ValidateJSON(cBytes(input))))
}

//export SteerCompileMacOS
func SteerCompileMacOS(input *C.char, stateDirectory *C.char) *C.char {
	return C.CString(string(steermacos.CompileJSON(cBytes(input), cString(stateDirectory))))
}

//export SteerPrepareMacOS
func SteerPrepareMacOS(input *C.char, appGroupRoot *C.char) *C.char {
	return C.CString(string(steermacos.PrepareJSON(cBytes(input), cString(appGroupRoot))))
}

//export SteerFreeString
func SteerFreeString(value *C.char) {
	C.free(unsafe.Pointer(value))
}

func cBytes(value *C.char) []byte {
	if value == nil {
		return nil
	}
	return []byte(C.GoString(value))
}

func cString(value *C.char) string {
	if value == nil {
		return ""
	}
	return C.GoString(value)
}
