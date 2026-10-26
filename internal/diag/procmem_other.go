//go:build !windows

package diag

func processMemory() (privateWS, commit uint64) { return 0, 0 }
