package common

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"sync"

	"github.com/piotrnar/gocoin/lib/btc"
)

type oneMinerId struct {
	Name string
	Tag  []byte
}

var MinerIds []oneMinerId

// return miner ID of the given coinbase transaction
func TxMiner(cbtx *btc.Tx) (string, int) {
	txdat := cbtx.Serialize()
	for i, m := range MinerIds {
		if bytes.Equal(m.Tag, []byte("_p2pool_")) { // P2Pool
			if len(cbtx.TxOut) > 10 &&
				bytes.Equal(cbtx.TxOut[len(cbtx.TxOut)-1].Pk_script[:2], []byte{0x6A, 0x28}) {
				return m.Name, i
			}
		} else if bytes.Equal(m.Tag, []byte("_RSK_")) { // RSK Pool
			if len(cbtx.TxOut) > 1 {
				txo := cbtx.TxOut[len(cbtx.TxOut)-1]
				if len(txo.Pk_script) >= 10 && txo.Pk_script[0] == 0x6A && string(txo.Pk_script[2:10]) == "RSKBLOCK" {
					return m.Name, i
				}
			}
		} else if bytes.Contains(txdat, m.Tag) {
			return m.Name, i
		}
	}

	for _, txo := range cbtx.TxOut {
		adr := btc.NewAddrFromPkScript(txo.Pk_script, Testnet)
		if adr != nil {
			return adr.String(), -1
		}
	}

	return "", -1
}

func ReloadMiners() {
	d, _ := ioutil.ReadFile("miners.json")
	if d != nil {
		var MinerIdFile [][3]string
		e := json.Unmarshal(d, &MinerIdFile)
		if e != nil {
			println("miners.json", e.Error())
			return
		}
		MinerIds = nil
		for _, r := range MinerIdFile {
			var rec oneMinerId
			rec.Name = r[0]
			if r[1] != "" {
				rec.Tag = []byte(r[1])
			} else {
				if a, _ := btc.NewAddrFromString(r[2]); a != nil {
					rec.Tag = a.OutScript()
				} else {
					println("Error in miners.json for", r[0])
					continue
				}
			}
			MinerIds = append(MinerIds, rec)
		}
	}
}

var (
	AverageFeeMutex     sync.Mutex
	AverageFeeBytes     uint64
	AverageFeeTotal     uint64
	AverageFee_SPB      float64
	averageFeeLastBlock uint32 = 0xffffffff
	averageFeeLastCount uint   = 0xffffffff
)

func GetAverageFee() float64 {
	Last.Mutex.Lock()
	end := Last.Block
	Last.Mutex.Unlock()

	LockCfg()
	blocks := CFG.Stat.FeesBlks
	UnlockCfg()
	if blocks <= 0 {
		blocks = 1 // at leats one block
	}

	AverageFeeMutex.Lock()
	defer AverageFeeMutex.Unlock()

	if end.Height == averageFeeLastBlock && averageFeeLastCount == blocks {
		return AverageFee_SPB // we've already calculated for this block
	}

	averageFeeLastBlock = end.Height
	averageFeeLastCount = blocks

	AverageFeeBytes = 0
	AverageFeeTotal = 0

	for blocks > 0 {
		bl, _, e := BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			return 0
		}
		block, e := btc.NewBlock(bl)
		if e != nil {
			return 0
		}

		cbasetx, cbasetxlen := btc.NewTx(bl[block.TxOffset:])
		var fees_from_this_block int64
		for o := range cbasetx.TxOut {
			fees_from_this_block += int64(cbasetx.TxOut[o].Value)
		}
		fees_from_this_block -= int64(btc.GetBlockReward(end.Height))

		if fees_from_this_block > 0 {
			AverageFeeTotal += uint64(fees_from_this_block)
		}

		AverageFeeBytes += uint64(len(bl) - block.TxOffset - cbasetxlen) /*do not count block header and conibase tx */

		blocks--
		end = end.Parent
	}
	if AverageFeeBytes == 0 {
		if AverageFeeTotal != 0 {
			panic("Impossible that miner gest a fee with no transactions in the block")
		}
		AverageFee_SPB = 0
	} else {
		AverageFee_SPB = float64(AverageFeeTotal) / float64(AverageFeeBytes)
	}
	return AverageFee_SPB
}
