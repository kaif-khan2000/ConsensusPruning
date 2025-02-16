package SC_Handler

import (
	ckm "StorageNode/Crypt_key_Manager"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"time"
)

var (
	prevHash   [32]byte
	difficulty = 4
)

// sidechain
type SideChainBlock struct {
	Hash       [32]byte
	PrevHash   [32]byte
	From       [65]byte
	Timestamp  int64
	Nonce      int
	Difficulty int
	Data       string
	Signature  []byte
}

type SideChain struct {
	Mux             sync.Mutex
	Chain           map[string]SideChainBlock
	LatestBlockHash string
}

// serializes the object to a byte array
func Serialize(p interface{}) ([]byte, error) {
	return json.Marshal(p)
}

func SerializeForPow(block *SideChainBlock) ([]byte, error) {
	// only include prevHash, timestamp, nonce, data, from
	return json.Marshal(struct {
		PrevHash   [32]byte
		From       [65]byte
		Timestamp  int64
		Difficulty int
		Nonce      int
		Data       string
	}{
		PrevHash:   block.PrevHash,
		From:       block.From,
		Timestamp:  block.Timestamp,
		Difficulty: block.Difficulty,
		Nonce:      block.Nonce,
		Data:       block.Data,
	})
}

func CreateSideChainBlock(data string, timestamp int64, privateKey *ecdsa.PrivateKey) *SideChainBlock {

	block := &SideChainBlock{PrevHash: prevHash, Timestamp: timestamp, Data: data}
	copy(block.From[:], ckm.SerializePublicKey(privateKey.PublicKey))
	block.Difficulty = difficulty
	return block

}

func (block *SideChainBlock) Pow() {
	var hash [32]byte
	nonce := 0
	flag := true
	for flag {
		nonce = 0
		block.Timestamp = time.Now().Unix()
		for nonce < math.MaxUint32 {
			block.Nonce = nonce
			serializedData, _ := SerializeForPow(block)
			hash = sha256.Sum256(serializedData)
			hexHash := hex.EncodeToString(hash[:])
			if hexHash[:difficulty] == strings.Repeat("0", difficulty) {
				flag = false
				break
			} else {
				nonce++
			}
		}
	}

	block.Hash = hash

}

func (block *SideChainBlock) SignBlock(privateKey *ecdsa.PrivateKey) {
	data := block.Hash[:]
	block.Signature = ckm.Sign(privateKey, data)
}

func (block *SideChainBlock) VerifyBlock() bool {

	// verify the signature
	data := block.Hash[:]
	if !ckm.Verify(block.From, data, block.Signature) {
		return false
	}

	// pow check
	serializedData, _ := SerializeForPow(block)
	hash := sha256.Sum256(serializedData)
	hexHash := hex.EncodeToString(hash[:])
	if hexHash[:difficulty] != strings.Repeat("0", difficulty) {
		return false
	}

	return true

}

func AddToSideChain(sc *SideChain, block *SideChainBlock) {
	sc.Mux.Lock()
	defer sc.Mux.Unlock()
	sc.Chain[hex.EncodeToString(block.Hash[:])] = *block
	sc.LatestBlockHash = hex.EncodeToString(block.Hash[:])
}

func CreateSideChainGenesisBlock() *SideChainBlock {
	genesisPrevHash := [32]byte{0}
	block := &SideChainBlock{PrevHash: genesisPrevHash, Timestamp: 0, Data: "Genesis Block"}
	block.Difficulty = difficulty
	block.Pow()
	return block
}

func InitSideChain() *SideChain {
	sc := &SideChain{Chain: make(map[string]SideChainBlock)}
	genesisBlock := CreateSideChainGenesisBlock()
	sc.Chain[hex.EncodeToString(genesisBlock.Hash[:])] = *genesisBlock
	sc.LatestBlockHash = hex.EncodeToString(genesisBlock.Hash[:])
	return sc
}