package syntax

/*
#include <stddef.h>
// Weak, so that a C library without malloc_trim, such as musl, still links.
extern int malloc_trim(size_t) __attribute__((weak));
static void trim(void) { if (malloc_trim) malloc_trim(0); }
*/
import "C"

// ReleaseMemory gives memory the parser freed back to the system. The C allocator keeps freed memory
// for reuse, so without this a closed tab's megabytes stay in the process's size until the next
// parse needs them. Call it from any goroutine after closing highlighters.
func ReleaseMemory() { C.trim() }
