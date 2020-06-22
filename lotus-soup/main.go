package main

import (
	"fmt"

	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
)

var testplans = map[string]interface{}{
	"lotus-baseline": testRunner(baselineRoles),
}

func main() {
	run.InvokeMap(testplans)
}

func testRunner(roles map[string]func(*Environment) error) run.TestCaseFn {
	return func(runenv *runtime.RunEnv) error {
		return runTestPlan(runenv, roles)
	}
}

func runTestPlan(runenv *runtime.RunEnv, roles map[string]func(*Environment) error) error {
	env := prepareEnv(runenv)
	role := runenv.StringParam("role")
	proc, ok := roles[role]
	if ok {
		return proc(env)
	}

	return fmt.Errorf("Unknown role: %s", role)
}
