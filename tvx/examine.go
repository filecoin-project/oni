package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/filecoin-project/lotus/chain/types"
	bitfield "github.com/filecoin-project/go-bitfield"
	chainState "github.com/filecoin-project/lotus/chain/state"
	"github.com/google/go-cmp/cmp"
	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	blocks "github.com/ipfs/go-block-format"
	addr "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	initActor "github.com/filecoin-project/specs-actors/actors/builtin/init"
	rewardActor "github.com/filecoin-project/specs-actors/actors/builtin/reward"
	storagePowerActor "github.com/filecoin-project/specs-actors/actors/builtin/power"
	storageMinerActor "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"

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

func getInitFor(store *state.ProxyingStores, root cid.Cid, helper func(act initActor.State) *initActorState) map[string]string {
	inverseMap := make(map[string]string)
	inverseMap[string(builtin.InitActorAddr.Bytes())] = "<InitActor>"
	inverseMap[string(builtin.RewardActorAddr.Bytes())] = "<RewardActor>"
	inverseMap[string(builtin.CronActorAddr.Bytes())] = "<CronActor>"
	inverseMap[string(builtin.StoragePowerActorAddr.Bytes())] = "<StoragePowerActor>"
	inverseMap[string(builtin.StorageMarketActorAddr.Bytes())] = "<StorageMarketActor>"
	inverseMap[string(builtin.VerifiedRegistryActorAddr.Bytes())] = "<VerifiedRegistryActor>"
	inverseMap[string(builtin.BurntFundsActorAddr.Bytes())] = "<BurntFundsActor>"
	tree, err := chainState.LoadStateTree(store.CBORStore, root)
	if err != nil {
		fmt.Printf("failed to load root State Tree for account mapping\n")
		return inverseMap
	}
	initAct, err := tree.GetActor(builtin.InitActorAddr)

	var initState initActor.State
	if err := store.CBORStore.Get(context.TODO(), initAct.Head, &initState); err != nil {
		fmt.Printf("failed to load Init acct @%v: %v\n", initAct.Head, err)
		return inverseMap
	}
	forward := helper(initState)
	for k, v := range forward.ADT {
		address, _ := addr.NewIDAddress(v)
		inverseMap[string(address.Bytes())] = k
	}
	return inverseMap
}

func diff(store *state.ProxyingStores, root, a, b *hamtNode) {
	initActorTransformer := func(act initActor.State) *initActorState {
		am, _ := adt.AsMap(store.ADTStore, act.AddressMap)
		var val cbg.CborInt
		m := make(map[string]uint64)
		am.ForEach(&val, func(k string) error {
			address, _ := addr.NewFromBytes([]byte(k))
			m[address.String()] = uint64(val)
			return nil
		})
		return &initActorState{
			NextID: act.NextID.String(),
			NetworkName: act.NetworkName,
			ADTRoot: act.AddressMap.String(),
			ADT: m,
		}
	}

	stateTreeNamer := getInitFor(store, root.Cid, initActorTransformer)

	hamtActorExpander := func (n *hamtNode) map[string]*types.Actor {
		m := make(map[string]*types.Actor)
		kv := hamt.KV{}
		n.Node.ForEach(context.TODO(), func(k string, val interface{}) error {
			kv.Key = []byte(k)
			if v, ok := val.(*cbg.Deferred); !ok {
				return fmt.Errorf("unexpected hamt node %v", val)
			} else {
				kv.Value = v
			}
	
			var template types.Actor
			cbor.DecodeInto(kv.Value.Raw, &template)
	
			if nk, ok := stateTreeNamer[k]; ok {
				k = nk
			}
			m[k] = &template
	
			return nil
		})
		return m
	}

	initHampTransformer := func(n *hamtNode) map[string]string {
		m := make(map[string]string)
		n.Node.ForEach(context.TODO(), func(k string, val interface{}) error {
			m[k] = fmt.Sprintf("%v", val)
			return nil
		})
		return m
	}

	actorTransformer := func (act *types.Actor) *statefulActor {
		var state interface{}
		block, _ := store.Blockstore.Get(act.Head)

		switch act.Code {
		case builtin.InitActorCodeID:
			var initState initActor.State
			cbor.DecodeInto(block.RawData(), &initState)
			state = initState
		case builtin.RewardActorCodeID:
			var rewardState rewardActor.State
			cbor.DecodeInto(block.RawData(), &rewardState)
			state = rewardState
		case builtin.StoragePowerActorCodeID:
			var storagePowerState storagePowerActor.State
			cbor.DecodeInto(block.RawData(), &storagePowerState)
			state = storagePowerState
		case builtin.StorageMinerActorCodeID:
			var storageMinerState storageMinerActor.State
			cbor.DecodeInto(block.RawData(), &storageMinerState)
			state = storageMinerState
		default:
			state = block
		}
		return &statefulActor{
			Type: builtin.ActorNameByCode(act.Code),
			State: state,
			Nonce: act.Nonce,
			Balance: act.Balance.String(),
		}
	}

	if d := cmp.Diff(a, b,
		cmp.Comparer(cidComparer),
		cmp.Comparer(bigIntComparer),
		cmp.AllowUnexported(blocks.BasicBlock{}),
		cmp.Transformer("types.Actor", actorTransformer),
		cmp.Transformer("bitfield.Bitfield", bitfieldTransformer),
		cmp.Transformer("initActor.State", initActorTransformer),
		cmp.FilterPath(filterIn("initActor"), cmp.Transformer("init.State", initHampTransformer)),
		cmp.FilterPath(topFilter, cmp.Transformer("state.StateTree", hamtActorExpander)));
		d != "" {
		fmt.Printf("Diff: %v\n", d)
	}
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

func topFilter(p cmp.Path) bool {
	return !strings.Contains(p.GoString(), "Actor")
}

func filterIn(substr string) func(cmp.Path) bool {
	return func(p cmp.Path) bool {
		return strings.Contains(p.GoString(), substr)
	}
}

func bitfieldTransformer(b *bitfield.BitField) string {
	data, _ := b.MarshalJSON()
	return string(data)
}
 
type statefulActor struct {
	Type string
	State interface{}
	Nonce uint64
	Balance string
}

type initActorState struct {
	ADT map[string]uint64
	ADTRoot string
	NextID string
	NetworkName string
}
