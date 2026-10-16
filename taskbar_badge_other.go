//go:build !windows

package main

func setTaskbarBadge(_ int) error { return nil }
