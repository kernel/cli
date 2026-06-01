package cmd

// BoolFlag captures whether a boolean flag was set explicitly and its value.
type BoolFlag struct {
	Set   bool
	Value bool
}

// Int64Flag captures whether an int64 flag was set explicitly and its value.
type Int64Flag struct {
	Set   bool
	Value int64
}
