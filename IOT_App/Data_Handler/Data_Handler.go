package Data_Handler

import (
	"encoding/json"
)

type Record struct {
	// the type of the data
	Type      string
	Timestamp int64
	Data      float32
	CrateId   string
	GwID      string
}

type Chunk struct {
	//ChunkID is the hash of the chunk
	ChunkID [32]byte `json:"ChunkID,omitempty"`
	//ChunkData is the data in the chunk
	ChunkData []Record
	//ChunkSize is the size of the chunk
	ChunkSize int
	// IhSequence
	IhSequence int
	//Chunk Position
	ChunkPosition int
}

type Transaction struct {
	//Type 0: MHT Tx
	//Type 1: IH Tx
	Type int8
	// POW Hash
	Hash [32]byte
	// Address
	From      [65]byte
	LeftTip   [32]byte
	RightTip  [32]byte
	Nonce     uint32
	Timestamp int64
	SessionId [32]byte

	//type 1 tx only
	MerkleRoot [32]byte
	//type 2 tx only
	IntermediateHashes [][32]byte
}

type Vertex struct {
	Tx         Transaction
	Signature  []byte
	Neighbours []string
	Weight     int
	Confirmed  bool
}

// serializes the object to a byte array
func Serialize(p interface{}) ([]byte, error) {
	return json.Marshal(p)
}

// deserializes the byte array to an object
func Deserialize(data string, p any) error {
	return json.Unmarshal(json.RawMessage(data), p)
}
