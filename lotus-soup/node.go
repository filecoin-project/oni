package main

import (
	"context"
	"errors"

	"github.com/testground/sdk-go/sync"

	"github.com/filecoin-project/lotus/api"
)

var PrepareNodeTimeout = time.Minute

type Node struct {
	api api.FullNode
}

type IntialBalanceMsg struct {
}

type PresealMsg struct {
}

type GenesisMsg struct {
}

func prepareBootstrapper(env *Environment) (*Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

	clients := env.runenv.IntParam("clients")
	miners := env.runenv.IntParam("miners")
	nodes := clients + miners

	// the first duty of the boostrapper is to construct the genesis block
	// first collect all client and miner balances to assign initial funds
	balanceMsgs := make([]*InitialBalanceMsg, 0, nodes)
	balanceTopic := sync.NewTopic("balance", &InitialBalanceMsg{})
	balanceCh := make(chan *InitialBalanceMsg)

	env.syncC.MustSubscribe(ctx, balanceTopic, balanceCh)
	for i := 0; i < nodes; i++ {
		m := <-balanceCh
		balanceMsgs = append(balanceMsgs, m)
	}

	// then collect all preseals from miners
	presealMsgs := make([]*PresealMsg, 0, miners)
	presealTopic := sync.NewTopic("preseal", &PresealMsg{})
	presealCh := make(chan *PresealMsg)

	env.syncC.MustSubscribe(ctx, presealTopic, presealCh)
	for i := 0; i < miners; i++ {
		m := <-presealCh
		presealMsgs = append(presealMsgs, m)
	}

	// now construct the genesis block and broadcast it
	// TODO make genesis block

	// TODO braodcast genesis block
	genesisMsg := &GenesisMsg{TODO}
	genesisTopic := sync.NewTopic("genesis", &GenesisMsg{})
	env.syncC.MustPublish(ctx, genesisTopic, genesisMsg)

	// TODO create api.FullNode and Node instance

	// TODO broadcast bootstrapper address

	// TODO Wait for all nodes to connect

	return nil, errors.New("TODO")
}

func prepareMiner(env *Environment) (*Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

	// first publish the account ID/balance and preseal commitment
	balanceTopic := sync.NewTopic("balance", &InitialBalanceMsg{})
	// TODO Create balance ID and amount
	balanceMsg := &IntialBalanceMsg{TODO}
	env.syncC.Publish(ctx, balanceTopic, balanceMsg)

	presealTopic := sync.NewTopic("preseal", &PresealMsg{})
	// TODO create preseal commitment
	presealMsg := &PresealMsg{TODO}
	env.syncC.Publish(ctx, presealTopic, presealMsg)

	// then collect the genesis block
	genesisTopic := sync.NewTopic("genesis", &GenesisMsg{})
	genesisCh := make(chan *GenesisMsg)
	env.syncC.MustSubscribe(ctx, genesisTopic, genesisCh)
	genesisMsg := <-genesisCh

	// TODO create api.FullNode and Node instance

	// TODO receive bootstrapper address

	// TODO connect node to bootstrapper

	return nil, errors.New("TODO")
}

func prepareClient(env *Environment) (*Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

	// first publish the account ID/balance
	balanceTopic := sync.NewTopic("balance", &InitialBalanceMsg{})
	// TODO Create balance ID and amount
	balanceMsg := &IntialBalanceMsg{TODO}
	env.syncC.Publish(ctx, balanceTopic, balanceMsg)

	// then collect the genesis block
	genesisTopic := sync.NewTopic("genesis", &GenesisMsg{})
	genesisCh := make(chan *GenesisMsg)
	env.syncC.MustSubscribe(ctx, genesisTopic, genesisCh)
	genesisMsg := <-genesisCh

	// TODO create api.FullNode and Node instance

	// TODO receive bootstrapper address

	// TODO connect node to bootstrapper

	return nil, errors.New("TODO")
}
