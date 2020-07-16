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
	valueTypes = []string{"MinerPower", "CommittedBytes", "ProvingBytes", "FaultyBytes", "Balance", "PreCommitDeposits", "LockedFunds", "AvailableFunds", "WorkerBalance", "MarketEscrow", "MarketLocked"}
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
		for difference, heights := range diffCmp[maddr][valueName] {
			fmt.Fprintf(w, "diff of %v at heights %v\n", difference, heights)
		}
	}
}

func recordDiff(mi *MinerInfo, height abi.ChainEpoch) {
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
				diffCmp[maddr]["MinerPower"][cmp.String()] = append(diffValue[maddr]["MinerPower"][cmp.String()], height)
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
				diffCmp[maddr]["CommittedBytes"][cmp.String()] = append(diffValue[maddr]["CommittedBytes"][cmp.String()], height)
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
				diffCmp[maddr]["ProvingBytes"][cmp.String()] = append(diffValue[maddr]["ProvingBytes"][cmp.String()], height)
			}
		}
	}

	{
		if mi.FaultyBytes.Int != nil {
			value := big.Int(mi.FaultyBytes)
			diffHeight[maddr]["FaultyBytes"][height] = value
			diffValue[maddr]["FaultyBytes"][value.String()] = append(diffValue[maddr]["FaultyBytes"][value.String()], height)

			if prevHeight != -1 {
				prevValue, ok := diffHeight[maddr]["FaultyBytes"][prevHeight]
				if ok {
					cmp := big.Zero()
					cmp.Sub(value.Int, prevValue.Int) // value - prevValue
					if big.Cmp(cmp, big.Zero()) != 0 {
						diffCmp[maddr]["FaultyBytes"][cmp.String()] = append(diffValue[maddr]["FaultyBytes"][cmp.String()], height)
					}
				}
			}
		}
	}

	{
		value := big.Int(mi.Balance)
		diffHeight[maddr]["Balance"][height] = value
		diffValue[maddr]["Balance"][value.String()] = append(diffValue[maddr]["Balance"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["Balance"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["Balance"][cmp.String()] = append(diffValue[maddr]["Balance"][cmp.String()], height)
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
				diffCmp[maddr]["PreCommitDeposits"][cmp.String()] = append(diffValue[maddr]["PreCommitDeposits"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.LockedFunds)
		diffHeight[maddr]["LockedFunds"][height] = value
		diffValue[maddr]["LockedFunds"][value.String()] = append(diffValue[maddr]["LockedFunds"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["LockedFunds"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["LockedFunds"][cmp.String()] = append(diffValue[maddr]["LockedFunds"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.AvailableFunds)
		diffHeight[maddr]["AvailableFunds"][height] = value
		diffValue[maddr]["AvailableFunds"][value.String()] = append(diffValue[maddr]["AvailableFunds"][value.String()], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["AvailableFunds"][prevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				diffCmp[maddr]["AvailableFunds"][cmp.String()] = append(diffValue[maddr]["AvailableFunds"][cmp.String()], height)
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
				diffCmp[maddr]["WorkerBalance"][cmp.String()] = append(diffValue[maddr]["WorkerBalance"][cmp.String()], height)
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
				diffCmp[maddr]["MarketEscrow"][cmp.String()] = append(diffValue[maddr]["MarketEscrow"][cmp.String()], height)
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
				diffCmp[maddr]["MarketLocked"][cmp.String()] = append(diffValue[maddr]["MarketLocked"][cmp.String()], height)
			}
		}
	}

}
