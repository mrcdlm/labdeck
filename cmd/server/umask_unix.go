//go:build unix

package main

import "syscall"

// restrictUmask sorgt dafür, dass neu angelegte Dateien nur für den
// Container-User lesbar sind (0600 bzw. 0700).
func restrictUmask() { syscall.Umask(0o077) }
