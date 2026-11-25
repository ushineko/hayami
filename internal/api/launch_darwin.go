package api

import (
	"syscall"
	"unsafe"
)

// launch_activate_socket and free are libSystem's, reached without cgo the way
// golang.org/x/sys/unix reaches it: the dynamic import, a trampoline in
// launch_darwin.s, and the runtime's call into C (spec 113's spike).

//go:cgo_import_dynamic libc_launch_activate_socket launch_activate_socket "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_free free "/usr/lib/libSystem.B.dylib"

var (
	libc_launch_activate_socket_trampoline_addr uintptr //nolint:revive,gochecknoglobals // the assembly's name
	libc_free_trampoline_addr                   uintptr //nolint:revive,gochecknoglobals // the assembly's name
)

//go:linkname syscall_syscall syscall.syscall
func syscall_syscall(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, err syscall.Errno) //nolint:revive // the runtime's name

/*
launchActivateSocket asks launchd for the sockets it made under name in this
job's Sockets dictionary. ESRCH is a process launchd did not start, ENOENT a
job with no such entry.
*/
func launchActivateSocket(name string) ([]int, error) {
	cname, err := syscall.BytePtrFromString(name)
	if err != nil {
		return nil, err //nolint:wrapcheck // a name with a NUL in it
	}
	var fds *int32
	var count uintptr
	r, _, _ := syscall_syscall(libc_launch_activate_socket_trampoline_addr,
		uintptr(unsafe.Pointer(cname)), uintptr(unsafe.Pointer(&fds)), uintptr(unsafe.Pointer(&count))) //nolint:gosec // the pointers C asks for
	if r != 0 {
		return nil, syscall.Errno(r)
	}
	if fds == nil {
		return nil, nil
	}
	out := make([]int, 0, count)
	for _, fd := range unsafe.Slice(fds, count) { //nolint:gosec // the list launchd made, count long
		out = append(out, int(fd))
	}
	_, _, _ = syscall_syscall(libc_free_trampoline_addr, uintptr(unsafe.Pointer(fds)), 0, 0) //nolint:gosec // the list C made
	return out, nil
}
