package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"

	"github.com/google/go-cmp/cmp"
	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/chain/types"

	// "github.com/filecoin-project/specs-actors/actors/builtin"
	// init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	// "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/oni/tvx/state"
)

var examineFlags struct {
	file string
}

var examineCmd = &cli.Command{
	Name:        "examine",
	Description: "examine an exported state root as represented in a test vector",
	Action:      runExamineCmd,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "file",
			Usage:       "test vector file",
			Required:    true,
			Destination: &examineFlags.file,
		},
	},
}

func runExamineCmd(_ *cli.Context) error {
	file, err := os.Open(examineFlags.file)
	if err != nil {
		return err
	}

	var tv TestVector
	if err := json.NewDecoder(file).Decode(&tv); err != nil {
		return err
	}

	/*
		examine := func(root cid.Cid) error {
			encoded := tv.CAR
			tree, err := state.RecoverStateTree(context.TODO(), encoded, root)
			if err != nil {
				return err
			}

			initActor, err := tree.GetActor(builtin.InitActorAddr)
			if err != nil {
				return fmt.Errorf("cannot recover init actor: %w", err)
			}

			var ias init_.State
			if err := tree.Store.Get(context.TODO(), initActor.Head, &ias); err != nil {
				return err
			}

			adtStore := adt.WrapStore(context.TODO(), tree.Store)
			m, err := adt.AsMap(adtStore, ias.AddressMap)
			if err != nil {
				return err
			}
			actors, err := m.CollectKeys()
			for _, actor := range actors {
				fmt.Printf("%s\n", actor)
			}
			return nil
		}
	*/

	encoded, err := state.RecoverStore(context.TODO(), tv.CAR)
	if err != nil {
		return err
	}
	preTree, err := hamt.LoadNode(context.TODO(), encoded.CBORStore, tv.Pre.StateTree.RootCID, hamt.UseTreeBitWidth(5))
	if err != nil {
		return err
	}
	postTree, err := hamt.LoadNode(context.TODO(), encoded.CBORStore, tv.Post.StateTree.RootCID, hamt.UseTreeBitWidth(5))
	if err != nil {
		return err
	}

	diff(encoded,
		&hamtNode{tv.Pre.StateTree.RootCID, preTree},
		&hamtNode{tv.Pre.StateTree.RootCID, preTree},
		&hamtNode{tv.Post.StateTree.RootCID, postTree})

	return nil
}

func diff(store *state.ProxyingStores, root, a, b *hamtNode) {
	hamtExpander := func(n *hamtNode) *dag {
		d := dag{
			ID:       n.Cid,
			Children: make(map[cid.Cid]interface{}),
			KVs:      make([]kv, 0),
		}
		for _, p := range n.Node.Pointers {
			if p.Link.Defined() {
				child, _ := hamt.LoadNode(context.TODO(), store.CBORStore, p.Link, hamt.UseTreeBitWidth(5))
				d.Children[p.Link] = hamtNode{p.Link, child}
			} else {
				for _, pkv := range p.KVs {
					var actor types.Actor
					cbor.DecodeInto(pkv.Value.Raw, &actor)
					d.KVs = append(d.KVs, kv{pkv.Key, &actor})
				}
			}
		}
		return &d
	}

	if d := cmp.Diff(a, b,
		cmp.Comparer(cidComparer),
		cmp.Comparer(bigIntComparer),
		cmp.Transformer("hamt.Node", hamtExpander)); d != "" {
		fmt.Printf("Diff: %v\n", d)
	}
}

type dag struct {
	ID       cid.Cid
	Children map[cid.Cid]interface{}
	KVs      []kv
}

type kv struct {
	Key   []byte
	Value *types.Actor
}

type hamtNode struct {
	Cid  cid.Cid
	Node *hamt.Node
}

func cidComparer(a, b cid.Cid) bool {
	return a.Equals(b)
}

func bigIntComparer(a, b *big.Int) bool {
	return a.Cmp(b) == 0
}
