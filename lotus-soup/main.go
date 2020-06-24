package main

import (
	"fmt"

	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
)

var testplans = map[string]interface{}{
	"lotus-baseline": doRun(baselineRoles),
}

func main() {
	run.InvokeMap(testplans)
}

func doRun(roles map[string]run.InitializedTestCaseFn) run.InitializedTestCaseFn {
	return func(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
		role := runenv.StringParam("role")
		proc, ok := baselineRoles[role]
		if ok {
			return proc(runenv, initCtx)
		}
		return fmt.Errorf("Unknown role: %s", role)
	}
}
