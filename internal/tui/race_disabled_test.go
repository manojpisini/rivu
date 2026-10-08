//go:build !race

package tui

// raceBuild is false for normal builds; see race_enabled_test.go.
const raceBuild = false
