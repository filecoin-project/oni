package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	manet "github.com/multiformats/go-multiaddr-net"
	"github.com/multiformats/go-multiaddr"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/go-address"
	"github.com/urfave/cli/v2"
	"github.com/ipfs/go-cid"
)

var (
	fromFlag = cli.StringFlag{
		Name: "from",
		Usage: "block CID of initial state",
		Required: true,
	}

	toFlag = cli.StringFlag{
		Name: "to",
		Usage: "block CID of ending state",
		Required: true,
	}

	apiFlag = cli.StringFlag{
		Name: "api",
		Usage: "api endpoint, formatted as token:multiaddr",
		Value: "",
		EnvVars: []string{"FULLNODE_API_INFO"},
	}
	
	idFlag = cli.StringFlag{
		Name: "cid",
		Usage: "CID to act upon",
		Required: true,
	}
)

var deltaCmd = &cli.Command{
	Name: "delta",
	Description: "Collect affected state between two tipsets",
	Flags: []cli.Flag{&fromFlag,&toFlag, &apiFlag},
	Action: extract,
}

var messageCmd = &cli.Command{
	Name: "message",
	Description: "Extract affected actors from a single message",
	Flags: []cli.Flag{&idFlag, &apiFlag},
	Action: message,
}

func makeClient(api string) (api.FullNode, error) {
	sp := strings.SplitN(api, ":", 2)
	if len(sp) != 2 {
		return nil, fmt.Errorf("invalid api value, missing token or address: %s", api)
	}
	//TODO: discovery from filesystem
	token := sp[0]
	ma, err := multiaddr.NewMultiaddr(sp[1])
	if err != nil {
		return nil, fmt.Errorf("could not parse provided multiaddr: %w", err)
	}
	_, dialAddr, err := manet.DialArgs(ma)
	if err != nil {
		return nil, fmt.Errorf("invalid api multiAddr: %w", err)
	}
	addr := "ws://" + dialAddr + "/rpc/v0"

	headers := http.Header{}
	if len(token) != 0 {
		headers.Add("Authorization", "Bearer "+string(token))
	}
	node, _, err := client.NewFullNodeRPC(addr, headers)
	if err != nil {
		return nil, fmt.Errorf("could not connect to api: %w", err)
	}
	return node, nil
}

func main() {
	app := &cli.App{
		Name: "fc-extract-delta",
		Usage: "Extract the delta between two filecoin states.",
		Commands: []*cli.Command{deltaCmd,messageCmd},
	}
	
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func hasParent(block *types.BlockHeader, parent cid.Cid) bool {
	for _, p := range block.Parents {
		if p.Equals(parent) {
			return true
		}
	}
	return false
}

func extract(c *cli.Context) error {
	node, err := makeClient(c.String(apiFlag.Name))
	if err != nil {
		return err
	}

	to, err := cid.Decode(c.String(toFlag.Name))
	if err != nil {
		return err
	}
	from, err := cid.Decode(c.String(fromFlag.Name))
	if err != nil {
		return err
	}

	currBlock, err := node.ChainGetBlock(context.TODO(), to)
	srcBlock, err := node.ChainGetBlock(context.TODO(), from)

	allMsgs := make(map[uint64][]*types.Message)

	epochs := currBlock.Height - srcBlock.Height -1
	for epochs >0 {
		msgs, err := node.ChainGetBlockMessages(context.TODO(), to)
		if err != nil {
			return err
		}
		allMsgs[uint64(currBlock.Height)] = msgs.BlsMessages
		currBlock, err = node.ChainGetBlock(context.TODO(), currBlock.Parents[0])
		epochs--
	}
	
	if !hasParent(currBlock, from) {
		return fmt.Errorf("from block was not a parent of `to` as expected")
	}

	m := 0
	for _, msgs := range allMsgs {
		m += len(msgs)
	}
	fmt.Printf("messages: %d\n", m)
	fmt.Printf("initial state root: %v\n", currBlock.ParentStateRoot)
	return nil
}

func message(c *cli.Context) error {
	node, err := makeClient(c.String(apiFlag.Name))
	if err != nil {
		return err
	}

	mid, err := cid.Decode(c.String(idFlag.Name))
	if err != nil {
		return err
	}

	msgInfo, err := node.StateSearchMsg(context.TODO(), mid)
	if err != nil {
		return err
	}

	trace, err := node.StateReplay(context.TODO(), msgInfo.TipSet, mid)
	if err != nil {
		return err
	}

	addresses := make(map[address.Address]struct{})
	populateFromTrace(&addresses, &trace.ExecutionTrace)

	for k, _ := range addresses {
		fmt.Printf("%v\n", k)
	}
	return nil
}

func populateFromTrace(m *map[address.Address]struct{}, trace *types.ExecutionTrace) {
	(*m)[trace.Msg.To] = struct{}{}
	(*m)[trace.Msg.From] = struct{}{}

	for _, s := range trace.Subcalls {
		populateFromTrace(m, &s)
	}
}
