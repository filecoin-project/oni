package main

import (
	"github.com/filecoin-project/oni/lotus-soup/testkit"
)

func syncMining(t *testkit.TestEnvironment) error {
	return testkit.HandleDefaultRole(t)
}
