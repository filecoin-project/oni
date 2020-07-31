# What & Why

This is essentially a shell script written in go that invokes a testground composition multiple times and collects
the output of each run into a directory.

It's designed to be used with tests that select parameters randomly from a range, so you can run monte-carlo style
simulations with lots of randomly chosen values.

# Usage

`go run . -runs 10 -output ~/test-outputs-will-be-saved-here my-composition.toml`
