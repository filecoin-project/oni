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
	
)

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
		Flags: []cli.Flag{&fromFlag, &toFlag, &apiFlag},
		Action: extract,
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
