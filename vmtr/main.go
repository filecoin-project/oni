package main

import (
	"flag"
	"os"
	"sort"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
)

var chainfile = flag.String("chainfile", "chain.car", "location of chain file")

var log = logging.Logger("main")

func init() {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")

	_ = logging.SetLogLevel("*", "INFO")

	build.InsecurePoStValidation = true
	build.DisableBuiltinAssets = true

	power.ConsensusMinerMinPower = big.NewInt(2048)
	miner.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg2KiBV1: {},
	}
	verifreg.MinVerifiedDealSize = big.NewInt(256)

	// MessageConfidence is the amount of tipsets we wait after a message is
	// mined, e.g. payment channel creation, to be considered committed.
	//build.MessageConfidence = 1

	// The period over which all a miner's active sectors will be challenged.
	miner.WPoStProvingPeriod = abi.ChainEpoch(240) // instead of 24 hours

	// The duration of a deadline's challenge window, the period before a deadline when the challenge is available.
	miner.WPoStChallengeWindow = abi.ChainEpoch(5) // instead of 30 minutes (still 48 per day)

	// Number of epochs between publishing the precommit and when the challenge for interactive PoRep is drawn
	// used to ensure it is not predictable by miner.
	miner.PreCommitChallengeDelay = abi.ChainEpoch(10)
}

func main() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "chainfile",
				Value: "",
				Usage: "",
			},
			&cli.StringFlag{
				Name:  "repodir",
				Value: "",
				Usage: "",
			},
			&cli.StringFlag{
				Name:  "output",
				Value: "",
				Usage: "",
			},
		},
		Commands: []*cli.Command{
			{
				Name:    "import",
				Aliases: []string{"i"},
				Usage:   "import a chainfile",
				Action:  cmdImport,
			},
			{
				Name:    "export",
				Aliases: []string{"e"},
				Usage:   "export from repo to chainfile",
				Action:  cmdExport,
			},
			{
				Name:    "generate_genesis",
				Aliases: []string{"gg"},
				Usage:   "generate genesis block based on state from heaviest tipset",
				Action:  cmdGenerate,
			},
			{
				Name:    "load_statetree",
				Aliases: []string{"ls"},
				Usage:   "load statetree",
				Action:  cmdLoadStateTree,
			},
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
