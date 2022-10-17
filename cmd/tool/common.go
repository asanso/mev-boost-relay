// Package tool exports tool subcommands
package tool

import "github.com/flashbots/mev-boost-relay/common"

var (
	log      = common.LogSetup(false, "info")
	outFiles []string
)
