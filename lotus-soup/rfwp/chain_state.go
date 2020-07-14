package rfwp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/lib/adtutil"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/oni/lotus-soup/testkit"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	sealing "github.com/filecoin-project/storage-fsm"

	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
	cbor "github.com/ipfs/go-ipld-cbor"

	tstats "github.com/filecoin-project/lotus/tools/stats"
)

func ChainState(t *testkit.TestEnvironment, m *testkit.LotusMiner) error {
	height := 0
	headlag := 3

	ctx := context.Background()

	tipsetsCh, err := tstats.GetTips(ctx, m.FullApi, abi.ChainEpoch(height), headlag)
	if err != nil {
		return err
	}

	for tipset := range tipsetsCh {
		maddrs, err := m.FullApi.StateListMiners(ctx, tipset.Key())
		if err != nil {
			return err
		}

		for _, maddr := range maddrs {
			err := info(t, m, maddr, tipset.Height())
			if err != nil {
				return err
			}

			err = provingFaults(t, m, maddr, tipset.Height())
			if err != nil {
				return err
			}

			err = provingInfo(t, m, maddr, tipset.Height())
			if err != nil {
				return err
			}

			err = provingDeadlines(t, m, maddr, tipset.Height())
			if err != nil {
				return err
			}

			err = sectorsList(t, m, maddr, tipset.Height())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func provingFaults(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, height abi.ChainEpoch) error {
	api := m.FullApi
	ctx := context.Background()

	filename := fmt.Sprintf("%s%cchain-state-%s-%d-faults", t.TestOutputsPath, os.PathSeparator, maddr, height)

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	var mas miner.State
	{
		mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		rmas, err := api.ChainReadObj(ctx, mact.Head)
		if err != nil {
			return err
		}
		if err := mas.UnmarshalCBOR(bytes.NewReader(rmas)); err != nil {
			return err
		}
	}
	faults, err := mas.Faults.All(100000000000)
	if err != nil {
		return err
	}
	if len(faults) == 0 {
		fmt.Fprintf(w, "no faulty sectors\n")
		return nil
	}
	head, err := api.ChainHead(ctx)
	if err != nil {
		return err
	}
	deadlines, err := api.StateMinerDeadlines(ctx, maddr, head.Key())
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "deadline\tsectors")
	for deadline, sectors := range deadlines.Due {
		intersectSectors, _ := bitfield.IntersectBitField(sectors, mas.Faults)
		if intersectSectors != nil {
			allSectors, _ := intersectSectors.All(100000000000)
			for _, num := range allSectors {
				_, _ = fmt.Fprintf(tw, "%d\t%d\n", deadline, num)
			}
		}

	}
	tw.Flush()
	return nil
}

func provingInfo(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, height abi.ChainEpoch) error {
	api := m.FullApi
	ctx := context.Background()

	filename := fmt.Sprintf("%s%cchain-state-%s-%d-proving-info", t.TestOutputsPath, os.PathSeparator, maddr, height)

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	head, err := api.ChainHead(ctx)
	if err != nil {
		return err
	}

	cd, err := api.StateMinerProvingDeadline(ctx, maddr, head.Key())
	if err != nil {
		return err
	}

	deadlines, err := api.StateMinerDeadlines(ctx, maddr, head.Key())
	if err != nil {
		return err
	}

	var mas miner.State
	{
		mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		rmas, err := api.ChainReadObj(ctx, mact.Head)
		if err != nil {
			return err
		}
		if err := mas.UnmarshalCBOR(bytes.NewReader(rmas)); err != nil {
			return err
		}
	}

	newSectors, err := mas.NewSectors.Count()
	if err != nil {
		return err
	}

	faults, err := mas.Faults.Count()
	if err != nil {
		return err
	}

	recoveries, err := mas.Recoveries.Count()
	if err != nil {
		return err
	}

	var provenSectors uint64
	for _, d := range deadlines.Due {
		c, err := d.Count()
		if err != nil {
			return err
		}
		provenSectors += c
	}

	var faultPerc float64
	if provenSectors > 0 {
		faultPerc = float64(faults*10000/provenSectors) / 100
	}

	fmt.Fprintf(w, "Current Epoch:           %d\n", cd.CurrentEpoch)
	fmt.Fprintf(w, "Chain Period:            %d\n", cd.CurrentEpoch/miner.WPoStProvingPeriod)
	fmt.Fprintf(w, "Chain Period Start:      %s\n", epochTime(cd.CurrentEpoch, (cd.CurrentEpoch/miner.WPoStProvingPeriod)*miner.WPoStProvingPeriod))
	fmt.Fprintf(w, "Chain Period End:        %s\n\n", epochTime(cd.CurrentEpoch, (cd.CurrentEpoch/miner.WPoStProvingPeriod+1)*miner.WPoStProvingPeriod))

	fmt.Fprintf(w, "Proving Period Boundary: %d\n", cd.PeriodStart%miner.WPoStProvingPeriod)
	fmt.Fprintf(w, "Proving Period Start:    %s\n", epochTime(cd.CurrentEpoch, cd.PeriodStart))
	fmt.Fprintf(w, "Next Period Start:       %s\n\n", epochTime(cd.CurrentEpoch, cd.PeriodStart+miner.WPoStProvingPeriod))

	fmt.Fprintf(w, "Faults:      %d (%.2f%%)\n", faults, faultPerc)
	fmt.Fprintf(w, "Recovering:  %d\n", recoveries)
	fmt.Fprintf(w, "New Sectors: %d\n\n", newSectors)

	fmt.Fprintf(w, "Deadline Index:       %d\n", cd.Index)

	if cd.Index < uint64(len(deadlines.Due)) {
		curDeadlineSectors, err := deadlines.Due[cd.Index].Count()
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "Deadline Sectors:     %d\n", curDeadlineSectors)
	}

	fmt.Fprintf(w, "Deadline Open:        %s\n", epochTime(cd.CurrentEpoch, cd.Open))
	fmt.Fprintf(w, "Deadline Close:       %s\n", epochTime(cd.CurrentEpoch, cd.Close))
	fmt.Fprintf(w, "Deadline Challenge:   %s\n", epochTime(cd.CurrentEpoch, cd.Challenge))
	fmt.Fprintf(w, "Deadline FaultCutoff: %s\n", epochTime(cd.CurrentEpoch, cd.FaultCutoff))
	return nil
}

func epochTime(curr, e abi.ChainEpoch) string {
	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s ago)", e, time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(curr-e)))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return fmt.Sprintf("%d (in %s)", e, time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(e-curr)))
	}

	panic("math broke")
}

func provingDeadlines(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, height abi.ChainEpoch) error {
	api := m.FullApi
	ctx := context.Background()

	filename := fmt.Sprintf("%s%cchain-state-%s-%d-deadlines", t.TestOutputsPath, os.PathSeparator, maddr, height)

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	deadlines, err := api.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}

	di, err := api.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}

	var mas miner.State
	var info *miner.MinerInfo
	{
		mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		rmas, err := api.ChainReadObj(ctx, mact.Head)
		if err != nil {
			return err
		}
		if err := mas.UnmarshalCBOR(bytes.NewReader(rmas)); err != nil {
			return err
		}

		info, err = mas.GetInfo(adtutil.NewStore(ctx, cbor.NewCborStore(apibstore.NewAPIBlockstore(api))))
		if err != nil {
			return err
		}
	}

	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "deadline\tsectors\tpartitions\tproven")

	for i, field := range deadlines.Due {
		c, err := field.Count()
		if err != nil {
			return err
		}

		firstPartition, sectorCount, err := miner.PartitionsForDeadline(deadlines, info.WindowPoStPartitionSectors, uint64(i))
		if err != nil {
			return err
		}

		partitionCount := (sectorCount + info.WindowPoStPartitionSectors - 1) / info.WindowPoStPartitionSectors

		var provenPartitions uint64
		{
			var maskRuns []rlepluslazy.Run
			if firstPartition > 0 {
				maskRuns = append(maskRuns, rlepluslazy.Run{
					Val: false,
					Len: firstPartition,
				})
			}
			maskRuns = append(maskRuns, rlepluslazy.Run{
				Val: true,
				Len: partitionCount,
			})

			ppbm, err := bitfield.NewFromIter(&rlepluslazy.RunSliceIterator{Runs: maskRuns})
			if err != nil {
				return err
			}

			pp, err := bitfield.IntersectBitField(ppbm, mas.PostSubmissions)
			if err != nil {
				return err
			}

			provenPartitions, err = pp.Count()
			if err != nil {
				return err
			}
		}

		var cur string
		if di.Index == uint64(i) {
			cur += "\t(current)"
		}
		_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%d%s\n", i, c, partitionCount, provenPartitions, cur)
	}

	return tw.Flush()
}

func sectorsList(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, height abi.ChainEpoch) error {
	api := m.FullApi
	ctx := context.Background()

	filename := fmt.Sprintf("%s%cchain-state-%s-%d-sectors", t.TestOutputsPath, os.PathSeparator, maddr, height)

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	list, err := m.MinerApi.SectorsList(ctx)
	if err != nil {
		return err
	}

	pset, err := api.StateMinerProvingSet(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}
	provingIDs := make(map[abi.SectorNumber]struct{}, len(pset))
	for _, info := range pset {
		provingIDs[info.ID] = struct{}{}
	}

	sset, err := api.StateMinerSectors(ctx, maddr, nil, true, types.EmptyTSK)
	if err != nil {
		return err
	}
	commitedIDs := make(map[abi.SectorNumber]struct{}, len(pset))
	for _, info := range sset {
		commitedIDs[info.ID] = struct{}{}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i] < list[j]
	})

	tw := tabwriter.NewWriter(w, 8, 4, 1, ' ', 0)

	for _, s := range list {
		st, err := m.MinerApi.SectorsStatus(ctx, s)
		if err != nil {
			fmt.Fprintf(w, "%d:\tError: %s\n", s, err)
			continue
		}

		_, inSSet := commitedIDs[s]
		_, inPSet := provingIDs[s]

		fmt.Fprintf(tw, "%d: %s\tsSet: %s\tpSet: %s\ttktH: %d\tseedH: %d\tdeals: %v\n",
			s,
			st.State,
			yesno(inSSet),
			yesno(inPSet),
			st.Ticket.Epoch,
			st.Seed.Epoch,
			st.Deals,
		)
	}

	return tw.Flush()
}

func yesno(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

func info(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, height abi.ChainEpoch) error {
	api := m.FullApi
	ctx := context.Background()

	filename := fmt.Sprintf("%s%cchain-state-%s-%d-info", t.TestOutputsPath, os.PathSeparator, maddr, height)

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}
	var mas miner.State
	{
		rmas, err := api.ChainReadObj(ctx, mact.Head)
		if err != nil {
			return err
		}
		if err := mas.UnmarshalCBOR(bytes.NewReader(rmas)); err != nil {
			return err
		}
	}

	fmt.Fprintf(w, "Miner: %s\n", maddr)

	// Sector size
	mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Sector Size: %s\n", types.SizeStr(types.NewInt(uint64(mi.SectorSize))))

	pow, err := api.StateMinerPower(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}

	rpercI := types.BigDiv(types.BigMul(pow.MinerPower.RawBytePower, types.NewInt(1000000)), pow.TotalPower.RawBytePower)
	qpercI := types.BigDiv(types.BigMul(pow.MinerPower.QualityAdjPower, types.NewInt(1000000)), pow.TotalPower.QualityAdjPower)

	fmt.Fprintf(w, "Byte Power:   %s / %s (%0.4f%%)\n",
		types.SizeStr(pow.MinerPower.RawBytePower),
		types.SizeStr(pow.TotalPower.RawBytePower),
		float64(rpercI.Int64())/10000)

	fmt.Fprintf(w, "Actual Power: %s / %s (%0.4f%%)\n",
		types.DeciStr(pow.MinerPower.QualityAdjPower),
		types.DeciStr(pow.TotalPower.QualityAdjPower),
		float64(qpercI.Int64())/10000)

	secCounts, err := api.StateMinerSectorCount(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}
	faults, err := api.StateMinerFaults(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}

	nfaults, err := faults.Count()
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "\tCommitted: %s\n", types.SizeStr(types.BigMul(types.NewInt(secCounts.Sset), types.NewInt(uint64(mi.SectorSize)))))
	if nfaults == 0 {
		fmt.Fprintf(w, "\tProving: %s\n", types.SizeStr(types.BigMul(types.NewInt(secCounts.Pset), types.NewInt(uint64(mi.SectorSize)))))
	} else {
		var faultyPercentage float64
		if secCounts.Sset != 0 {
			faultyPercentage = float64(10000*nfaults/secCounts.Sset) / 100.
		}
		fmt.Fprintf(w, "\tProving: %s (%s Faulty, %.2f%%)\n",
			types.SizeStr(types.BigMul(types.NewInt(secCounts.Pset), types.NewInt(uint64(mi.SectorSize)))),
			types.SizeStr(types.BigMul(types.NewInt(nfaults), types.NewInt(uint64(mi.SectorSize)))),
			faultyPercentage)
	}

	if pow.MinerPower.RawBytePower.LessThan(power.ConsensusMinerMinPower) {
		fmt.Fprintf(w, "Below minimum power threshold, no blocks will be won")
	} else {
		expWinChance := float64(types.BigMul(qpercI, types.NewInt(build.BlocksPerEpoch)).Int64()) / 1000000
		if expWinChance > 0 {
			if expWinChance > 1 {
				expWinChance = 1
			}
			winRate := time.Duration(float64(time.Second*time.Duration(build.BlockDelaySecs)) / expWinChance)
			winPerDay := float64(time.Hour*24) / float64(winRate)

			fmt.Print("Expected block win rate: ")
			fmt.Fprintf(w, "%.4f/day (every %s)", winPerDay, winRate.Truncate(time.Second))
		}
	}

	fmt.Println()

	fmt.Fprintf(w, "Miner Balance: %s\n", types.FIL(mact.Balance))
	fmt.Fprintf(w, "\tPreCommit:   %s\n", types.FIL(mas.PreCommitDeposits))
	fmt.Fprintf(w, "\tLocked:      %s\n", types.FIL(mas.LockedFunds))
	fmt.Fprintf(w, "\tAvailable:   %s", types.FIL(types.BigSub(mact.Balance, types.BigAdd(mas.LockedFunds, mas.PreCommitDeposits))))
	wb, err := api.WalletBalance(ctx, mi.Worker)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Worker Balance: %s", types.FIL(wb))

	mb, err := api.StateMarketBalance(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Market (Escrow):  %s\n", types.FIL(mb.Escrow))
	fmt.Fprintf(w, "Market (Locked):  %s\n\n", types.FIL(mb.Locked))

	fmt.Println("Sectors:")
	err = sectorsInfo(ctx, w, m.MinerApi)
	if err != nil {
		return err
	}
	return nil
}

func sectorsInfo(ctx context.Context, w io.Writer, napi api.StorageMiner) error {
	sectors, err := napi.SectorsList(ctx)
	if err != nil {
		return err
	}

	buckets := map[sealing.SectorState]int{
		"Total": len(sectors),
	}
	for _, s := range sectors {
		st, err := napi.SectorsStatus(ctx, s)
		if err != nil {
			return err
		}

		buckets[sealing.SectorState(st.State)]++
	}

	var sorted []stateMeta
	for state, i := range buckets {
		sorted = append(sorted, stateMeta{i: i, state: state})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return stateOrder[sorted[i].state].i < stateOrder[sorted[j].state].i
	})

	for _, s := range sorted {
		_, _ = fmt.Fprintf(w, "\t%s: %d\n", s.state, s.i)
	}

	return nil
}

type stateMeta struct {
	i     int
	state sealing.SectorState
}

var stateOrder = map[sealing.SectorState]stateMeta{}
var stateList = []stateMeta{
	{state: "Total"},
	{state: sealing.Proving},

	{state: sealing.UndefinedSectorState},
	{state: sealing.Empty},
	{state: sealing.Packing},
	{state: sealing.PreCommit1},
	{state: sealing.PreCommit2},
	{state: sealing.PreCommitting},
	{state: sealing.PreCommitWait},
	{state: sealing.WaitSeed},
	{state: sealing.Committing},
	{state: sealing.CommitWait},
	{state: sealing.FinalizeSector},

	{state: sealing.FailedUnrecoverable},
	{state: sealing.SealPreCommit1Failed},
	{state: sealing.SealPreCommit2Failed},
	{state: sealing.PreCommitFailed},
	{state: sealing.ComputeProofFailed},
	{state: sealing.CommitFailed},
	{state: sealing.PackingFailed},
	{state: sealing.FinalizeFailed},
	{state: sealing.Faulty},
	{state: sealing.FaultReported},
	{state: sealing.FaultedFinal},
}

func init() {
	for i, state := range stateList {
		stateOrder[state.state] = stateMeta{
			i: i,
		}
	}
}
