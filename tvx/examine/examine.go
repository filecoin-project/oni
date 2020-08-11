package examine

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"regexp"
	"reflect"
	"strings"

	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
	bitfield "github.com/filecoin-project/go-bitfield"
	chainState "github.com/filecoin-project/lotus/chain/state"
	"github.com/google/go-cmp/cmp"
	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	blocks "github.com/ipfs/go-block-format"
	addr "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	accountActor "github.com/filecoin-project/specs-actors/actors/builtin/account"
	initActor "github.com/filecoin-project/specs-actors/actors/builtin/init"
	rewardActor "github.com/filecoin-project/specs-actors/actors/builtin/reward"
	cronActor "github.com/filecoin-project/specs-actors/actors/builtin/cron"
	storagePowerActor "github.com/filecoin-project/specs-actors/actors/builtin/power"
	marketActor "github.com/filecoin-project/specs-actors/actors/builtin/market"
	storageMinerActor "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	verifiedRegistryActor "github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"
	runtime "github.com/filecoin-project/specs-actors/actors/runtime"
)

func Diff(ctx context.Context, store blockstore.Blockstore, root, a, b cid.Cid) {
	cborStore := cbor.NewCborStore(store)
	adtStore := adt.WrapStore(ctx, cborStore)

	initActorTransformer := func(act initActor.State) *initActorState {
		am, _ := adt.AsMap(adtStore, act.AddressMap)
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

	stateTreeNamer := getInitFor(ctx, cborStore, root, initActorTransformer)

	hamtActorExpander := func (n *hamtNode) map[string]*types.Actor {
		m := make(map[string]*types.Actor)
		kv := hamt.KV{}
		n.Node.ForEach(ctx, func(k string, val interface{}) error {
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
		n.Node.ForEach(ctx, func(k string, val interface{}) error {
			m[k] = fmt.Sprintf("%v", val)
			return nil
		})
		return m
	}

	actorTransformer := func (act *types.Actor) *statefulActor {
		var state interface{}
		block, _ := store.Get(act.Head)

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
		case builtin.CronActorCodeID:
			var cronState cronActor.State
			cbor.DecodeInto(block.RawData(), &cronState)
			state = cronState
		case builtin.StorageMarketActorCodeID:
			var marketState marketActor.State
			cbor.DecodeInto(block.RawData(), &marketState)
			state = marketState
		case builtin.VerifiedRegistryActorCodeID:
			var registryState verifiedRegistryActor.State
			cbor.DecodeInto(block.RawData(), &registryState)
			state = registryState
		case builtin.AccountActorCodeID:
			var accountState accountActor.State
			cbor.DecodeInto(block.RawData(), &accountState)
			state = accountState
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

	cidMap := make(map[string]reflect.Type)
	cidMap[".*BasicBlock\\)\\.cid"] = reflect.TypeOf("") // don't recurse on uninterpreted data.
	cidMap["^{cid.Cid}$"] = reflect.TypeOf((*hamt.Node)(nil)) // handle state root expansion as a top-level hamt

	// storageMinerActor mappings
	cidMap[".*\\(miner\\.State\\)\\.Info$"] = reflect.TypeOf((*storageMinerActor.MinerInfo)(nil))
	cidMap[".*\\(miner\\.State\\)\\.VestingFunds$"] = reflect.TypeOf("") // TODO: use VestingFund after next spec-actors release.
	cidMap[".*\\(miner\\.State\\)\\.PreCommittedSectors$"] = reflect.TypeOf(make(map[string]*storageMinerActor.SectorPreCommitOnChainInfo))
	cidMap[".*\\(miner\\.State\\)\\.PreCommittedSectors.*SealedCID$"] = reflect.TypeOf("")
	cidMap[".*\\(miner\\.State\\)\\.AllocatedSectors$"] = reflect.TypeOf((*bitfield.BitField)(nil))
	cidMap[".*\\(miner\\.State\\)\\.Sectors$"] = reflect.TypeOf((*adt.Array)(nil))
	cidMap[".*\\(miner\\.State\\)\\.Sectors.*SealedCID$"] = reflect.TypeOf("")
	cidMap[".*\\(miner\\.State\\)\\.Deadlines$"] = reflect.TypeOf((*storageMinerActor.Deadlines)(nil))
	cidMap[".*\\(miner\\.State\\)\\.Deadlines.*Due([^\\.]*)$"] = reflect.TypeOf((*storageMinerActor.Deadline)(nil))
	cidMap[".*\\(miner\\.State\\).*Partitions$"] = reflect.TypeOf(make([]*storageMinerActor.Partition, 0))
	cidMap["miner\\.Deadline\\)\\.ExpirationsEpochs$"] = reflect.TypeOf(make([]*bitfield.BitField, 0))
	cidMap[".*\\(miner\\.State\\).*Partitions.*ExpirationsEpochs$"] = reflect.TypeOf(make([]*storageMinerActor.ExpirationSet, 0))
	cidMap[".*\\(miner\\.State\\).*EarlyTerminated$"] = reflect.TypeOf(make([]*bitfield.BitField, 0))

	// storagePowerActor mappings
	cidMap[".*\\(power\\.State\\)\\.CronEventQueue$"] = reflect.TypeOf("") // TODO: support for 'multimap'
	cidMap[".*\\(power\\.State\\)\\.Claims$"] = reflect.TypeOf(make(map[string]*storagePowerActor.Claim))
	cidMap[".*\\(power\\.State\\)\\.ProofValidationBatch$"] = reflect.TypeOf("") // TODO: used?

	// marketActor mappings
	cidMap[".*\\(market\\.State\\)\\.Proposals$"] = reflect.TypeOf(make([]*marketActor.DealProposal, 0))
	cidMap[".*\\(market\\.State\\).*PieceCID$"] = reflect.TypeOf("")
	cidMap[".*\\(market\\.State\\)\\.States$"] = reflect.TypeOf(make([]*marketActor.DealState, 0))
	cidMap[".*\\(market\\.State\\)\\.PendingProposals$"] = reflect.TypeOf(make(map[string]*marketActor.DealProposal))
	cidMap[".*\\(market\\.State\\)\\.EscrowTable$"] = reflect.TypeOf(make(map[string]*big.Int)) // adt.BalanceTable
	cidMap[".*\\(market\\.State\\)\\.LockedTable$"] = reflect.TypeOf(make(map[string]*big.Int)) // adt.BalanceTable
	cidMap[".*\\(market\\.State\\)\\.DealOpsByEpoch$"] = reflect.TypeOf("") // TODO: support for 'multimap'
	
	// verifiedRegistryActor mappings
	cidMap[".*\\(verifreg\\.State\\)\\.Verifiers$"] = reflect.TypeOf(make(map[string]*verifiedRegistryActor.DataCap))
	cidMap[".*\\(verifreg\\.State\\)\\.VerifiedClients$"] = reflect.TypeOf(make(map[string]*verifiedRegistryActor.DataCap))

	opts := []cmp.Option{
		cmp.Comparer(bigIntComparer),
		cmp.AllowUnexported(blocks.BasicBlock{}),
		cmp.Transformer("types.Actor", actorTransformer),
		cmp.Transformer("bitfield.Bitfield", bitfieldTransformer),
		cmp.Transformer("address.Address", addressTransformer),
		cmp.Transformer("initActor.State", initActorTransformer),
		cmp.FilterPath(filterIn("initActor"), cmp.Transformer("init.State", initHampTransformer)),
		cmp.FilterPath(topFilter, cmp.Transformer("state.StateTree", hamtActorExpander)),
	}
	opts = append(opts, cidTransformer(ctx, store, cborStore, cidMap)...)
	if d := cmp.Diff(a, b, opts...);
		d != "" {
		fmt.Printf("Diff: %v\n", d)
	}
}

func cidTransformer(ctx context.Context, store blockstore.Blockstore, cborStore cbor.IpldStore, atlas map[string]reflect.Type) []cmp.Option {
	options := make([]cmp.Option, 0)
	pathFilter := func(matcher string) func(p cmp.Path) bool {
		return func(p cmp.Path) bool {
			ok, _ := regexp.MatchString(matcher, p.GoString())
			return ok
		}
	}

	for r, t := range atlas {
		name := strings.Trim(t.String(), "*")
		if t.Kind() == reflect.Map {
			name = "amtmap." + strings.Trim(t.Elem().String(), "*")
		} else if t.Kind() == reflect.Slice {
			name = "amtarray." +strings.Trim(t.Elem().String(), "*")
		}
		boundFunc := (func(name string, t reflect.Type) func(c cid.Cid) interface{} {
			return func(c cid.Cid) interface{} {
				if name == "hamt.Node" {
					// special case for maps / other complex data types that require multiple cid walks.
					node, _ := hamt.LoadNode(ctx, cborStore, c, hamt.UseTreeBitWidth(5))
					return &hamtNode{Cid: c, Node: node}
				} else if name == "string" {
					// special case for not expanding.
					return c.String()
				} else if strings.HasPrefix(name, "amtmap.") {
					adtStore := adt.WrapStore(ctx, cborStore)
					am, err := adt.AsMap(adtStore, c)
					if err != nil {
						panic(fmt.Sprintf("loading %s failed: %v",name, err))
					}
					val := reflect.New(t.Elem().Elem())
					asUnmarshaller, ok := val.Interface().(runtime.CBORUnmarshaler)
					if !ok {
						panic(fmt.Sprintf("%s must implement CBORUnmarshaler", t.Elem().String()))
					}
					m := reflect.MakeMap(t)
					am.ForEach(asUnmarshaller, func(k string) error {
						m.SetMapIndex(reflect.ValueOf(k), val)
						return nil
					})
					return m.Interface()
				} else if strings.HasPrefix(name, "amtarray.") {
					// Note: this implementation throws away the idx's of the array.
					// it lets you see the more compact array represenation, but may lead to
					// incorrect ordering (and makes it hard to trace the real index)
					adtStore := adt.WrapStore(ctx, cborStore)
					arr, err := adt.AsArray(adtStore, c)
					if err != nil {
						panic(fmt.Sprintf("loading %s failed: %v",name, err))
					}
					val := reflect.New(t.Elem().Elem())
					asUnmarshaller, ok := val.Interface().(runtime.CBORUnmarshaler)
					if !ok {
						panic(fmt.Sprintf("%s must implement CBORUnmarshaler", t.Elem().String()))
					}
					arrLen := int(arr.Length())
					m := reflect.MakeSlice(t, arrLen, arrLen)
					i := 0
					arr.ForEach(asUnmarshaller, func(idx int64) error {
						m.Index(i).Set(val)
						i+=1
						return nil
					})
					return m.Interface()
				}

				val := reflect.New(t.Elem())
				asUnmarshaller, ok := val.Interface().(runtime.CBORUnmarshaler)
				block, _ := store.Get(c)
				if !ok {
					return block.RawData()
				}
				if err := asUnmarshaller.UnmarshalCBOR(bytes.NewBuffer(block.RawData())); err != nil {
					panic(fmt.Sprintf("Unable to interpret %s as a %v", c, t.Elem().String()))
				}
				return val.Interface()
			}
		})(name, t)
		options = append(options, cmp.FilterPath(pathFilter(r), cmp.Transformer(name, boundFunc)))
	}
	return options
}

func getInitFor(ctx context.Context, store cbor.IpldStore, root cid.Cid, helper func(act initActor.State) *initActorState) map[string]string {
	inverseMap := make(map[string]string)
	inverseMap[string(builtin.InitActorAddr.Bytes())] = "<InitActor>"
	inverseMap[string(builtin.RewardActorAddr.Bytes())] = "<RewardActor>"
	inverseMap[string(builtin.CronActorAddr.Bytes())] = "<CronActor>"
	inverseMap[string(builtin.StoragePowerActorAddr.Bytes())] = "<StoragePowerActor>"
	inverseMap[string(builtin.StorageMarketActorAddr.Bytes())] = "<StorageMarketActor>"
	inverseMap[string(builtin.VerifiedRegistryActorAddr.Bytes())] = "<VerifiedRegistryActor>"
	inverseMap[string(builtin.BurntFundsActorAddr.Bytes())] = "<BurntFundsActor>"
	tree, err := chainState.LoadStateTree(store, root)
	if err != nil {
		fmt.Printf("failed to load root State Tree for account mapping\n")
		return inverseMap
	}
	initAct, err := tree.GetActor(builtin.InitActorAddr)

	var initState initActor.State
	if err := store.Get(ctx, initAct.Head, &initState); err != nil {
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

type hamtNode struct {
	Cid  cid.Cid
	Node *hamt.Node
}

func bigIntComparer(a, b *big.Int) bool {
	return a.Cmp(b) == 0
}

func addressTransformer(a addr.Address) string {
	return a.String()
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
