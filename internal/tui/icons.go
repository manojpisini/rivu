package tui

// Nerd Font v3 glyphs (P10.04). Private-use codepoints that every
// popular Nerd font ships; RIVU_ASCII swaps them for width-1 letters
// at the single Root.View exit point (asciiSwap in tui.go), so layout
// maths never sees a double-width rune.
const (
	iconGit      = "󰊠" // nf-dev-git
	iconBank     = "󰟀" // nf-md-bank
	iconMap      = "󰉺" // nf-md-map
	iconSource   = "󰐿" // nf-md-sprout
	iconActive   = "󰒃" // nf-md-play
	iconMaint    = "󰒾" // nf-md-wrench
	iconResearch = "󰋽" // nf-md-flask
	iconDelta    = "󰆊" // nf-md-archive
	iconHealth   = "󰋜" // nf-md-heart
	iconPresent  = "󰄳" // nf-md-check_circle
	iconMissing  = "󰚌" // nf-md-alert_circle
)
