package main

import "runtime"

// runtimeVersion mengembalikan versi Go yang dipakai membangun biner ini.
func runtimeVersion() string { return runtime.Version() }
