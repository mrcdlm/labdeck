//go:build !unix

package main

// restrictUmask: ohne Unix-umask nichts zu tun.
func restrictUmask() {}
