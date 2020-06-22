module github.com/filecoin-project/oni/lotus-soup

go 1.14

require (
	github.com/filecoin-project/lotus v0.3.1-0.20200518172415-1ed618334471
	github.com/testground/sdk-go v0.2.3-0.20200617132925-2e4d69f9ba38
)

replace github.com/filecoin-project/filecoin-ffi => ../lotus/extern/filecoin-ffi

replace github.com/filecoin-project/lotus => ../lotus
