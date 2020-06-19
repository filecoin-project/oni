package main

import (
	"fmt"

	"github.com/testground/sdk-go/runtime"
)

var testplans = map[string]runtime.TestCaseFn{
	"lotus-baseline": testRunner(baselineRoles),
}

func main() {
	runtime.InvokeMap(testplans)
}

func testRunner(roles map[string]func(*Environment) error) func(runenv *runtime.RunEnv) error {
	func(runenv *runtime.RunEnv) error {
		return runTestPlan(runenv, roles)
	}
}

func runTestPlan(runenv *runtime.RunEnv, roles map[string]func(*Environment) error) {
	env := prepareEnv(runenv)
	role := runenv.StringParam("role")
	proc, ok := roles[role]
	if ok {
		return proc(env)
	}

	return fmt.Errorf("Unknown role: %s", role)
}
