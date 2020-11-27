package main

import (
	"github.com/filecoin-project/oni/onions"
	"github.com/filecoin-project/oni/onions/vm"
)

func main() {
	// TODO these should be input parameters
	nMiners := 2
	nClients := 3
	initialBalance := 2000000
	presealSectors := 10

	// create initial actors + genesis block
	// Note: this can go into a utility method in the onions module
	var actors []*onions.Actor
	var miners []*onions.MinerActor
	var preseals []*onions.Preseal
	for i := 0; i < nMiners; i++ {
		miner, err := onions.CreateMinerActor(i, initialBalance)
		if err != nil {
			panic(err)
		}
		miners = append(miners, miner)
		actors = append(actors, miner.Actor)

		preseal, err := onions.Preseal(miner, presealSectors)
		if err != nil {
			panic(err)
		}
		preseals = append(preseals, preseal)
	}

	for i := 0; i < nClients; i++ {
		actor, err := onions.CreateActor(initialBalance)
		if err != nil {
			panic(err)
		}
		actors = append(actors, actor)
	}

	genesis, err := onions.CreateGenesis(actors, preseals)
	if err != nil {
		panic(err)
	}

	// start the vm execution
	vm.Start(genesis)

	// 1st batch of messages: miners must change their peer IDs
	for i := 0; i < nMiners; i++ {
		msg := onions.NewChangeMinerIDMessage(miner)
		vm.Push(msg)
	}

	// commit the first batch of messages
	vm.Commit()

	// now we need to create some deals
	// we need to decompose all the deal consistuents into vm messages, create messages,
	// push them in the vm, and commit them for each block

	// end of the test
	vm.End()
}
