package rfwp

import (
	"bufio"
	"fmt"
	"os"
	"sort"

	"github.com/filecoin-project/oni/lotus-soup/testkit"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
)

var (
	prevHeight = abi.ChainEpoch(-1)
	diffHeight = make(map[string]map[string]map[abi.ChainEpoch]big.Int)  // height -> value
	diffValue  = make(map[string]map[string]map[string][]abi.ChainEpoch) // value -> []height
	diffCmp    = make(map[string]map[string]map[string][]abi.ChainEpoch) // difference (height, height-1) -> []height
	valueTypes = []string{"MinerPower", "CommittedBytes", "ProvingBytes", "Balance", "PreCommitDeposits", "LockedFunds", "AvailableFunds", "WorkerBalance", "MarketEscrow", "MarketLocked", "Faults", "ProvenSectors", "Recoveries", "NewSectors"}
)

func printDiff(t *testkit.TestEnvironment, mi *MinerInfo, height abi.ChainEpoch) {
	maddr := mi.MinerAddr.String()
	filename := fmt.Sprintf("%s%cdiff-%s-%d", t.TestOutputsPath, os.PathSeparator, maddr, height)

	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	keys := make([]string, 0, len(diffCmp[maddr]))
	for k := range diffCmp[maddr] {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Fprintln(w, "=====", maddr, "=====")
	for _, valueName := range keys {
		fmt.Fprintln(w, "=====", valueName, "=====")
		if len(diffCmp[maddr][valueName]) > 0 {
			fmt.Fprint(w, "diff of             |\n")
		}

		for difference, heights := range diffCmp[maddr][valueName] {
			fmt.Fprintf(w, "diff of %30v at heights %v\n", difference, heights)
		}
	}
}

func recordDiff(mi *MinerInfo, ps *ProvingInfoState, height abi.ChainEpoch) {
	maddr := mi.MinerAddr.String()
	if _, ok := diffHeight[maddr]; !ok {
		diffHeight[maddr] = make(map[string]map[abi.ChainEpoch]big.Int)
		diffValue[maddr] = make(map[string]map[string][]abi.ChainEpoch)
		diffCmp[maddr] = make(map[string]map[string][]abi.ChainEpoch)

		for _, v := range valueTypes {
			diffHeight[maddr][v] = make(map[abi.ChainEpoch]big.Int)
			diffValue[maddr][v] = make(map[string][]abi.ChainEpoch)
			diffCmp[maddr][v] = make(map[string][]abi.ChainEpoch)
		}
	}

	{
		value := big.Int(mi.MinerPower.MinerPower.RawBytePower)
		diffHeight[maddr]["MinerPower"][height] = value
		diffValue[maddr]["MinerPower"][value.String()] = append(diffValue[maddr]["MinerPower"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["MinerPower"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["MinerPower"][cmp.String()] = append(diffCmp[maddr]["MinerPower"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.CommittedBytes)
		diffHeight[maddr]["CommittedBytes"][height] = value
		diffValue[maddr]["CommittedBytes"][value.String()] = append(diffValue[maddr]["CommittedBytes"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["CommittedBytes"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["CommittedBytes"][cmp.String()] = append(diffCmp[maddr]["CommittedBytes"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.ProvingBytes)
		diffHeight[maddr]["ProvingBytes"][height] = value
		diffValue[maddr]["ProvingBytes"][value.String()] = append(diffValue[maddr]["ProvingBytes"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["ProvingBytes"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["ProvingBytes"][cmp.String()] = append(diffCmp[maddr]["ProvingBytes"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.Balance)
		roundBalance(&value)
		diffHeight[maddr]["Balance"][height] = value
		diffValue[maddr]["Balance"][value.String()] = append(diffValue[maddr]["Balance"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["Balance"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["Balance"][cmp.String()] = append(diffCmp[maddr]["Balance"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.PreCommitDeposits)
		diffHeight[maddr]["PreCommitDeposits"][height] = value
		diffValue[maddr]["PreCommitDeposits"][value.String()] = append(diffValue[maddr]["PreCommitDeposits"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["PreCommitDeposits"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["PreCommitDeposits"][cmp.String()] = append(diffCmp[maddr]["PreCommitDeposits"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.LockedFunds)
		roundBalance(&value)
		diffHeight[maddr]["LockedFunds"][height] = value
		diffValue[maddr]["LockedFunds"][value.String()] = append(diffValue[maddr]["LockedFunds"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["LockedFunds"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["LockedFunds"][cmp.String()] = append(diffCmp[maddr]["LockedFunds"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.AvailableFunds)
		roundBalance(&value)
		diffHeight[maddr]["AvailableFunds"][height] = value
		diffValue[maddr]["AvailableFunds"][value.String()] = append(diffValue[maddr]["AvailableFunds"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["AvailableFunds"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["AvailableFunds"][cmp.String()] = append(diffCmp[maddr]["AvailableFunds"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.WorkerBalance)
		diffHeight[maddr]["WorkerBalance"][height] = value
		diffValue[maddr]["WorkerBalance"][value.String()] = append(diffValue[maddr]["WorkerBalance"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["WorkerBalance"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["WorkerBalance"][cmp.String()] = append(diffCmp[maddr]["WorkerBalance"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.MarketEscrow)
		diffHeight[maddr]["MarketEscrow"][height] = value
		diffValue[maddr]["MarketEscrow"][value.String()] = append(diffValue[maddr]["MarketEscrow"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["MarketEscrow"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["MarketEscrow"][cmp.String()] = append(diffCmp[maddr]["MarketEscrow"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.MarketLocked)
		diffHeight[maddr]["MarketLocked"][height] = value
		diffValue[maddr]["MarketLocked"][value.String()] = append(diffValue[maddr]["MarketLocked"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["MarketLocked"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["MarketLocked"][cmp.String()] = append(diffCmp[maddr]["MarketLocked"][cmp.String()], height)
			}
		}
	}

	{
		value := big.NewInt(int64(ps.Faults))
		diffHeight[maddr]["Faults"][height] = value
		diffValue[maddr]["Faults"][value.String()] = append(diffValue[maddr]["Faults"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["Faults"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["Faults"][cmp.String()] = append(diffCmp[maddr]["Faults"][cmp.String()], height)
			}
		}
	}

	{
		value := big.NewInt(int64(ps.ProvenSectors))
		diffHeight[maddr]["ProvenSectors"][height] = value
		diffValue[maddr]["ProvenSectors"][value.String()] = append(diffValue[maddr]["ProvenSectors"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["ProvenSectors"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["ProvenSectors"][cmp.String()] = append(diffCmp[maddr]["ProvenSectors"][cmp.String()], height)
			}
		}
	}

	{
		value := big.NewInt(int64(ps.Recoveries))
		diffHeight[maddr]["Recoveries"][height] = value
		diffValue[maddr]["Recoveries"][value.String()] = append(diffValue[maddr]["Recoveries"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["Recoveries"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["Recoveries"][cmp.String()] = append(diffCmp[maddr]["Recoveries"][cmp.String()], height)
			}
		}
	}

	{
		value := big.NewInt(int64(ps.NewSectors))
		diffHeight[maddr]["NewSectors"][height] = value
		diffValue[maddr]["NewSectors"][value.String()] = append(diffValue[maddr]["NewSectors"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["NewSectors"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["NewSectors"][cmp.String()] = append(diffCmp[maddr]["NewSectors"][cmp.String()], height)
			}
		}
	}
}

func roundBalance(i *big.Int) {
	*i = big.Div(*i, big.NewInt(1000000000000000))
	*i = big.Mul(*i, big.NewInt(1000000000000000))
}
