package cmd

import "github.com/spf13/pflag"

// BoolFlag captures whether a boolean flag was set explicitly and its value.
type BoolFlag struct {
	Set   bool
	Value bool
}

// readBoolFlag preserves the distinction between an omitted flag and an
// explicitly false value.
func readBoolFlag(flags *pflag.FlagSet, name string) BoolFlag {
	value, _ := flags.GetBool(name)
	return BoolFlag{Set: flags.Changed(name), Value: value}
}

// Int64Flag captures whether an int64 flag was set explicitly and its value.
type Int64Flag struct {
	Set   bool
	Value int64
}
