package main

import (
	"github.com/testground/sdk-go/runtime"
)

var testcases = map[string]runtime.TestCaseFn{
	"lotus-baseline": runBaseline,
}

func main() {
	runtime.InvokeMap(testcases)
}
