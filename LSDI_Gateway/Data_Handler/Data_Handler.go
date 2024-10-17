package Data_Handler

import (
	crypt "LSDI_Gateway/Crypt_key_Manager"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
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
	IhSequenceStart    int
	IhSequenceEnd      int
}

type Vertex struct {
	Gateway_no int
	Vertex_no  int
	Tx         Transaction
	Signature  []byte
	Neighbours []string
	Weight     int
	Confirmed  bool
}

// DAG defines the data structure to store the blockchain
type DAG struct {
	// Mutex is used to lock the DAG when it is being updated
	Mux sync.Mutex
	//Graph is a map of the vertices in the DAG
	Graph map[string]Vertex
	//length of the longest chain
	Length int
	//??
	RecentTXs []int64
}

type Neighbours_GW struct {
	Gw           []string          // neighbouring gateways
	Sn           []string          // neighbouring storage nodes
	Token_gw     map[string]string // token of neighbouring gateways
	Token_sn     string            // token of storage nodes
	Own_token_gw string            // token of this gateway
	Own_nodeno   int               // Number of node before it
}

var Neighbours_gw Neighbours_GW
var total_vertex int = 0
var Orphan_tx map[string]Vertex
var orphan_count int = 0
var time1 time.Time
var pruneThreshold int
var startThreshold int
var powDifficulty int
var tipThreshold int
var totalTx int64

func init() {
	time1 = time.Now()
	pruneThreshold = 10
	startThreshold = 5
	powDifficulty = 5
	tipThreshold = 1
	totalTx = 0
	fmt.Println("start of dh.")
}

func TotalTx() int64 {
	return totalTx
}

// serializes the object to a byte array
func Serialize(p interface{}) ([]byte, error) {
	return json.Marshal(p)
}

// deserializes the byte array to an object
func Deserialize(data string, p any) error {
	return json.Unmarshal(json.RawMessage(data), p)
}

// get all the possible tips in the graph that have a threshold of neighbours
func getPossibleTips(dag *DAG, threshold int) []string {
	tips := make([]string, 0)
	dag.Mux.Lock()
	graph := dag.Graph

	for key, value := range graph {
		if len(key) == 0 {
			continue
		}
		if value.Weight <= threshold {
			tips = append(tips, key)
		}
	}
	if len(tips) < 1 && len(dag.Graph) != 1 {
		// get 2 tips with least weight
		min1 := math.MaxInt32
		min2 := math.MaxInt32
		var min1Hash string
		var min2Hash string
		for key, value := range graph {
			if value.Confirmed && value.Weight < min1 {
				min1 = value.Weight
				min1Hash = key
			}
		}
		for key, value := range graph {
			if value.Confirmed && value.Weight < min2 && key != min1Hash {
				min2 = value.Weight
				min2Hash = key
			}
		}
		tips = append(tips, min1Hash)
		tips = append(tips, min2Hash)
	}
	dag.Mux.Unlock()

	return tips
}

// get the start of the tip
func GetStart(dag *DAG, tip string) []string {

	// fmt.Println("in get start")
	startArray := make([]string, 0)
	// get the start of the tip
	// start is the vertex with no parents existing in the graph
	// parents are left and right tips of the vertex
	dag.Mux.Lock()
	// fmt.Println("dag acquired by getstart")
	leftTipHash := dag.Graph[tip].Tx.LeftTip
	rightTipHash := dag.Graph[tip].Tx.RightTip
	leftTip := hex.EncodeToString(leftTipHash[:])
	rightTip := hex.EncodeToString(rightTipHash[:])
	_, leftTipExists := dag.Graph[leftTip]
	_, rightTipExists := dag.Graph[rightTip]
	dag.Mux.Unlock()
	// fmt.Println("dag released - 2 by getstart")

	if leftTipExists {
		newArray := GetStart(dag, leftTip)
		startArray = append(startArray, newArray...)
	}
	if rightTipExists {
		newArray := GetStart(dag, rightTip)
		startArray = append(startArray, newArray...)
	}
	if !leftTipExists && !rightTipExists {
		startArray = append(startArray, tip)
	}
	return startArray
}

// random walk to get a random tip with weighted probability
func RandomWalk(dag *DAG, start string, alpha int, threshold int) [32]byte {
	// if the start is a potential tip, return it
	dag.Mux.Lock()
	if dag.Graph[start].Weight <= threshold {
		decodedTip, _ := hex.DecodeString(start)
		// make [32]byte from decodedTip
		var tip [32]byte
		copy(tip[:], decodedTip)
		dag.Mux.Unlock()
		return tip
	}

	neighbours := dag.Graph[start].Neighbours
	var temp []string
	for i, neighbour := range neighbours {
		if dag.Graph[neighbour].Confirmed {
			temp = append(temp, neighbours[i])
		}
	}
	neighbours = temp
	dag.Mux.Unlock()

	if len(neighbours) == 0 {
		decodedTip, _ := hex.DecodeString(start)
		// make [32]byte from decodedTip
		var tip [32]byte
		copy(tip[:], decodedTip)
		return tip
	}
	// calculate weights of all neighbours
	ratings := make([]float64, 0)
	total := 0.0
	for _, neighbour := range neighbours {
		// rating is an exp function of the weight of the neighbour
		// rating = e^(-alpha*(start.weight-neighbour.weight))
		rating := math.Exp(-1 * float64(alpha) * float64(dag.Graph[start].Weight-dag.Graph[neighbour].Weight))
		ratings = append(ratings, rating)

		// update total
		total += rating
	}

	min := 0.0
	max := total
	randNumber := rand.Float64()*(max-min) + min
	var i int
	var rating float64
	for i, rating = range ratings {
		if randNumber <= rating {
			break
		}
		randNumber -= rating
	}
	return RandomWalk(dag, neighbours[i], alpha, threshold)
}

// MCMCTipSelect selects a tip using the MCMC algorithm
func McmcTipSelect(dag *DAG, alpha int, threshold int) ([32]byte, [32]byte) {
	// tipType = 0 -> random tip
	// tipType = 1 -> timebased tip

	// get all possible tips
	tips := getPossibleTips(dag, threshold)
	// fmt.Println("Possible tips: ", tips)
	// find the start of each possible tip
	starts := make([]string, 0)

	for _, tip := range tips {
		fmt.Println("getstart for :", tip)
		newStarts := GetStart(dag, tip)
		dupMap := make(map[string]bool)
		// no duplicates
		for _, newStart := range newStarts {
			if _, ok := dupMap[newStart]; !ok {
				starts = append(starts, newStart)
				dupMap[newStart] = true
			}
		}
	}

	// fmt.Println("Starts: ", starts)

	// randomly select one start and keep on moving towards the tip using mcmc algorithm
	// randomly select a start
	min := 0
	max := len(starts)
	rand.Seed(time.Now().UnixNano())
	index := rand.Intn(max-min) + min // range is min to max

	index2 := rand.Intn(max-min) + min
	// chose the start with the least weight and the timestamp
	for i, start := range starts {
		if dag.Graph[start].Weight < dag.Graph[starts[index]].Weight && time.Now().UnixNano()-dag.Graph[start].Tx.Timestamp < int64(startThreshold)*60*1000000000 {
			index2 = i
		}
	}

	start := starts[index]
	start2 := starts[index2]

	// now do the random walk
	actualTip := RandomWalk(dag, start, alpha, threshold)
	actualTip2 := RandomWalk(dag, start2, alpha, threshold)

	return actualTip, actualTip2
}

// proof of work
func Pow(transaction *Transaction, difficulty int) {
	var hash [32]byte
	var nonce uint32
	var flag bool = true
	for flag {
		nonce = 0
		transaction.Timestamp = time.Now().UnixNano()
		for nonce < math.MaxUint32 {
			transaction.Nonce = nonce
			SerializedData, _ := Serialize(transaction)
			hash = sha256.Sum256(SerializedData)
			hexHash := hex.EncodeToString(hash[:])
			if hexHash[:difficulty] == strings.Repeat("0", difficulty) {
				flag = false
				break
			} else {
				nonce++
			}
		}
	}
	transaction.Hash = hash

	hexHash := hex.EncodeToString(transaction.Hash[:])
	if hexHash[:difficulty] != strings.Repeat("0", difficulty) {
		fmt.Println("Orphan Count: POW verification failed: ", hexHash)
	}

}

// create genesis transaction
func CreateGenesisTx() Transaction {
	var tx Transaction
	tx.Type = 0
	tx.LeftTip = [32]byte{}
	tx.RightTip = [32]byte{}
	tx.Nonce = 0
	tx.Timestamp = 0

	// proof of work is done seperately for this transaction as it is the genesis transaction
	// and it shoud be same for all nodes
	difficulty := 3
	var nonce uint32 = 0
	var hash [32]byte
	for nonce < math.MaxUint32 {
		tx.Nonce = nonce
		SerializedData, _ := Serialize(tx)
		hash = sha256.Sum256(SerializedData)
		hexHash := hex.EncodeToString(hash[:])
		if hexHash[:difficulty] == strings.Repeat("0", difficulty) {
			break
		} else {
			nonce++
		}
	}
	tx.Hash = hash
	return tx
}

// // this function is my implementation update the weight of the vertices in the graph
// func UpdateWeight(dag *DAG, tip string) {
// 	// get the left and right tips of the current tip
// 	dag.Mux.Lock()
// 	leftTipHash := dag.Graph[tip].Tx.LeftTip
// 	rightTipHash := dag.Graph[tip].Tx.RightTip
// 	leftTip := hex.EncodeToString(leftTipHash[:])
// 	rightTip := hex.EncodeToString(rightTipHash[:])
// 	dag.Mux.Unlock()

// 	// increment the weight of left and right tips

// 	// if left tip exists
// 	dag.Mux.Lock()
// 	if vertex, ok := dag.Graph[leftTip]; ok {
// 		vertex.Weight++
// 		dag.Graph[leftTip] = vertex
// 		dag.Mux.Unlock()
// UpdateWeight(dag, leftTip)
// 	} else {
// 		dag.Mux.Unlock()
// 	}

// 	// if right tip exists
// 	dag.Mux.Lock()
// 	if vertex, ok := dag.Graph[rightTip]; ok {
// 		vertex.Weight++
// 		dag.Graph[rightTip] = vertex
// 		dag.Mux.Unlock()
// 		UpdateWeight(dag, rightTip)
// 	} else {
// 		dag.Mux.Unlock()
// 	}
// }

// these 2 functions provided by aravind
func UpdateWeightsHandler(dag *DAG, h string) map[string]Vertex {
	// Update the weights of the transactions in the DAG
	var vertices = make(map[string]Vertex)

	currvert, ok := dag.Graph[h]
	if !ok {
		return vertices
	}

	vertices[h] = currvert
	// currvert.Weight = 1
	leftTip := hex.EncodeToString(currvert.Tx.LeftTip[:])
	rightTip := hex.EncodeToString(currvert.Tx.RightTip[:])
	verticesleft := UpdateWeightsHandler(dag, leftTip)
	verticesright := UpdateWeightsHandler(dag, rightTip)
	for k, v := range verticesleft {
		vertices[k] = v
	}
	for k, v := range verticesright {
		vertices[k] = v
	}
	return vertices
	// dag.Mux.Unlock()
}

func UpdateWeightsHandlerIterative(dag *DAG, h string) map[string]Vertex {
	// Update the weights of the transactions in the DAG
	var vertices = make(map[string]Vertex)
	currVert, ok := dag.Graph[h]
	if !ok {
		return vertices
	}
	vertices[h] = currVert
	stack := make([]string, 0)
	stack = append(stack, h)
	for len(stack) != 0 {
		k := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		currVert, ok := dag.Graph[k]
		if !ok {
			continue
		}
		leftTip := hex.EncodeToString(currVert.Tx.LeftTip[:])
		rightTip := hex.EncodeToString(currVert.Tx.RightTip[:])
		if _, ok := vertices[leftTip]; !ok {
			vert, ok2 := dag.Graph[leftTip]
			if ok2 {
				vertices[leftTip] = vert
				stack = append(stack, leftTip)
			}
		}
		if _, ok := vertices[rightTip]; !ok {
			vert, ok2 := dag.Graph[rightTip]
			if ok2 {
				stack = append(stack, rightTip)
				vertices[rightTip] = vert
			}
		}
	}

	return vertices
}

func UpdateWeights(dag *DAG, h string) {
	// Update the weights of the transactions in the DAG
	fmt.Println("Updating weights for: ", h)
	dag.Mux.Lock()
	// vertices := UpdateWeightsHandler(dag, h)
	vertices := UpdateWeightsHandlerIterative(dag, h)
	// fmt.Println("update:", vertices)
	fmt.Println("weights updated")
	for a, v := range vertices {
		v.Weight++
		_, ok := dag.Graph[a]
		if !ok {
			continue
		}
		dag.Graph[a] = v
	}

	dag.Mux.Unlock()
}

func UpdateWeightOfChildren(dag *DAG, tip1 string, tip2 string) {
	zeroString := "0000000000000000000000000000000000000000000000000000000000000000"
	if tip1 != zeroString {
		tip1_vertex := dag.Graph[tip1]
		tip1_vertex.Weight++
		dag.Graph[tip1] = tip1_vertex
		fmt.Println("UpdateWeightOfChildren: ", tip1, " weight: ", tip1_vertex.Weight)
	}

	if tip2 != zeroString {
		tip2_vertex := dag.Graph[tip2]
		tip2_vertex.Weight++
		dag.Graph[tip2] = tip2_vertex
		fmt.Println("UpdateWeightOfChildren: ", tip2, " weight: ", tip2_vertex.Weight)
	}
}

// PruningHandler prunes the Dag when required
func Prune(dag *DAG, threshold int) {
	fmt.Println("Pruning the DAG")
	count := 0
	hashArray := make([]string, 0)
	dag.Mux.Lock()

	// collect all the transactions that are confirmed and have weight greater than threshold and exceed the threshold time
	thresholdTime := (10 * time.Minute).Nanoseconds()
	for hash, vertex := range dag.Graph {
		if (vertex.Confirmed) && (vertex.Weight >= threshold) && (time.Now().UnixNano()-vertex.Tx.Timestamp > int64(thresholdTime)) {
			// delete(dag.Graph, hash)
			count++
			hashArray = append(hashArray, hash)
		}
	}
	dag.Mux.Unlock()
	fmt.Println("prune: total ", count, " transactions")
	if count <= 1 {
		fmt.Println("Pruned ", 0, " transactions")
		return
	} else {
		// delete except one with lowest weight
		dag.Mux.Lock()
		min := dag.Graph[hashArray[0]].Weight
		minHash := hashArray[0]
		for _, hash := range hashArray {
			if dag.Graph[hash].Weight < min {
				min = dag.Graph[hash].Weight
				minHash = hash
			}
		}
		count = 0
		for _, hash := range hashArray {
			if hash != minHash {
				delete(dag.Graph, hash)
				// fmt.Println("Orphan Count: pruned: ", hash)
				count++
			}
		}
		dag.Mux.Unlock()
	}
	fmt.Println("Pruned ", count, " transactions")

	// delete the vertex from the graph if it exceeds 30 mins and are unconfirmed.

	dag.Mux.Lock()
	count = 0
	for hash, vertex := range dag.Graph {
		if !vertex.Confirmed && time.Now().UnixNano()-vertex.Tx.Timestamp > (10*time.Minute).Nanoseconds() {
			delete(dag.Graph, hash)
			// fmt.Println("Orphan Count: pruned(confirmation timeout): ", hash)
			count++
		}
	}

	dag.Mux.Unlock()
	fmt.Println("Pruned ", count, " old transactions")
}

func Ack_orphan_checker(txHash string, dag *DAG) (bool, Vertex) {
	flag := false
	fmt.Println("dag acquiring by ack_orphan_checker")
	// dag.Mux.Lock()
	fmt.Println("dag acquired by ack_orphan_checker")
	vtx, ok := Orphan_tx[txHash]
	if ok {
		delete(Orphan_tx, txHash)
		//adding tx to dag
		flag = true
	}
	// dag.Mux.Unlock()
	fmt.Println("dag released by ack_orphan_checker")
	return flag, vtx
}

// create a new transaction
func CreateMerkleTx(privateKey *ecdsa.PrivateKey, dag *DAG, merkleHash [32]byte, sessionId [32]byte) Transaction {
	var tx Transaction
	copy(tx.From[:], crypt.SerializePublicKey(privateKey.PublicKey))
	tx.Type = 1
	fmt.Println("Tip Selection Merkle")
	tx.LeftTip, tx.RightTip = McmcTipSelect(dag, 1, tipThreshold)
	for tx.LeftTip == [32]byte{0} || tx.RightTip == [32]byte{0} {
		tx.LeftTip, tx.RightTip = McmcTipSelect(dag, 1, tipThreshold)
	}
	fmt.Println("Tip Selection Merkle END")
	tx.Nonce = 0
	tx.Timestamp = 0
	tx.SessionId = sessionId
	tx.MerkleRoot = merkleHash
	fmt.Println("POW merkle Start")
	Pow(&tx, powDifficulty)
	fmt.Println("POW merkle End")
	return tx
}

func PrioritySelection(dag *DAG, threshold int) ([32]byte, [32]byte) {
	tips := getPossibleTips(dag, threshold)
	fmt.Println("Possible tips Len: ", len(tips))
	// fmt.Println("Possible tips: ", tips)
	//select 2 tips probabilistically (equal probability)
	min := 0
	max := len(tips)
	rand.Seed(time.Now().UnixNano())
	index := rand.Intn(max-min) + min // range is min to max
	// rand.Seed(time.Now().UnixNano())
	index2 := rand.Intn(max-min) + min
	for index == index2 && len(tips) > 1 {
		index2 = rand.Intn(max-min) + min
	}
	decoded_tip1, _ := hex.DecodeString(tips[index])
	decoded_tip2, _ := hex.DecodeString(tips[index2])
	var tip1, tip2 [32]byte
	copy(tip1[:], decoded_tip1)
	copy(tip2[:], decoded_tip2)
	return tip1, tip2
}

func CreateTx(privateKey *ecdsa.PrivateKey, dag *DAG, merkleHash [32]byte) Transaction {
	var tx Transaction
	copy(tx.From[:], crypt.SerializePublicKey(privateKey.PublicKey))

	tx.Type = 1
	fmt.Println("for Tip Selection")
	tx.LeftTip, tx.RightTip = PrioritySelection(dag, tipThreshold)
	fmt.Println("out Tip Selection")

	fmt.Println("Tip Selection Merkle END")
	tx.Nonce = 0
	tx.Timestamp = 0
	tx.MerkleRoot = merkleHash
	fmt.Println("POW merkle Start")
	Pow(&tx, powDifficulty)
	fmt.Println("POW merkle End")

	return tx
}

// create a Intermediate hash Transaction (IHTx) type - 2 transaction
func CreateIHTx(privateKey *ecdsa.PrivateKey, dag *DAG, ih [][32]byte, sessionId [32]byte, ihStart int, ihEnd int) Transaction {
	var tx Transaction
	copy(tx.From[:], crypt.SerializePublicKey(privateKey.PublicKey))
	tx.Type = 2
	fmt.Println(":Tip Selection IH")
	tx.LeftTip, tx.RightTip = McmcTipSelect(dag, 1, tipThreshold)
	for tx.LeftTip == [32]byte{0} || tx.RightTip == [32]byte{0} {
		tx.LeftTip, tx.RightTip = McmcTipSelect(dag, 1, tipThreshold)
	}
	fmt.Println(":Tip Selection IH END")
	tx.SessionId = sessionId
	tx.Nonce = 0
	tx.Timestamp = 0
	tx.IntermediateHashes = ih
	tx.IhSequenceStart = ihStart
	tx.IhSequenceEnd = ihEnd
	fmt.Println("POW IH Start")
	Pow(&tx, powDifficulty)
	fmt.Println("POW IH End")
	return tx
}

func CreateVertex(transaction Transaction, privateKey *ecdsa.PrivateKey) Vertex {
	var vertex Vertex
	vertex.Vertex_no = total_vertex
	vertex.Gateway_no = Neighbours_gw.Own_nodeno
	total_vertex += 1
	vertex.Tx = transaction
	vertex.Weight = 0
	vertex.Neighbours = make([]string, 0)
	// data, _ := Serialize(transaction)
	data := transaction.Hash[:]
	vertex.Confirmed = false
	vertex.Signature = crypt.Sign(privateKey, data)
	return vertex
}

func AddToDAG(transaction Transaction, dag *DAG, privateKey *ecdsa.PrivateKey) Vertex {
	// create a vertex
	vertex := CreateVertex(transaction, privateKey)
	if transaction.Type == 0 {
		vertex.Confirmed = true
	} else {
		if !VerifyVertex(vertex, dag) {
			fmt.Println("Orphan Count: signature failed")
		}
	}
	// add the vertex to the graph
	// hash of the transaction is the key
	hash := hex.EncodeToString(vertex.Tx.Hash[:])

	//lock the dag
	fmt.Println("dag acquiring by addtodag")
	dag.Mux.Lock()
	fmt.Println("dag acquired by addtodag")

	dag.Graph[hash] = vertex

	dag.Mux.Unlock()
	fmt.Println("dag released by addtodag")

	// update the neighbours of the lefttip and righttip
	leftTip := hex.EncodeToString(vertex.Tx.LeftTip[:])
	rightTip := hex.EncodeToString(vertex.Tx.RightTip[:])

	// if leftTip exist in dag
	fmt.Println("dag acquiring by addtodag")
	dag.Mux.Lock()
	fmt.Println("dag acquired by addtodag")
	if tip, ok := dag.Graph[leftTip]; ok {
		tip.Neighbours = append(tip.Neighbours, hash)

		dag.Graph[leftTip] = tip
	}

	// if rightTip exist in dag

	if tip, ok := dag.Graph[rightTip]; ok && strings.Compare(rightTip, leftTip) != 0 {
		tip.Neighbours = append(tip.Neighbours, hash)

		dag.Graph[rightTip] = tip
	}
	UpdateWeightOfChildren(dag, leftTip, rightTip)

	dag.Mux.Unlock()
	fmt.Println("dag released by addtodag")
	totalTx += 1
	return vertex
}

func VerifyTips(vertex Vertex, dag *DAG) bool {
	leftTip := hex.EncodeToString(vertex.Tx.LeftTip[:])
	rightTip := hex.EncodeToString(vertex.Tx.RightTip[:])
	// if leftTip exist in dag
	fmt.Println("dag acquiring by verifytips")
	dag.Mux.Lock()
	fmt.Println("dag acquired by verifytips")
	if _, ok := dag.Graph[leftTip]; !ok {
		fmt.Println("No left Tip vertex no is ", vertex.Vertex_no)
		if _, ok := Orphan_tx[leftTip]; !ok {
			fmt.Println("No left Tip vertex no in orphan ", vertex.Vertex_no)
			dag.Mux.Unlock()
			fmt.Println("dag released by verifytips")
			return false
		} else {
			fmt.Println("add left Tip in dag ", vertex.Vertex_no)
			otx := Orphan_tx[leftTip]
			dag.Mux.Unlock()
			UploadToDAG(otx, dag)
			delete(Orphan_tx, leftTip)
		}
	} else {
		dag.Mux.Unlock()
		fmt.Println("dag released by verifytips")
	}
	// if rightTip exist in dag
	fmt.Println("dag acquiring by verifytips")
	dag.Mux.Lock()
	fmt.Println("dag acquired by verifytips")
	if _, ok := dag.Graph[rightTip]; !ok {
		fmt.Println("No right Tip vertex no is ", vertex.Vertex_no)
		if _, ok := Orphan_tx[rightTip]; !ok {
			fmt.Println("No right Tip vertex no in orphan ", vertex.Vertex_no)
			dag.Mux.Unlock()
			fmt.Println("dag released by verifytips")
			return false
		} else {
			fmt.Println("add right Tip in dag ", vertex.Vertex_no)
			otx := Orphan_tx[rightTip]
			dag.Mux.Unlock()
			fmt.Println("dag released by verifytips")
			UploadToDAG(otx, dag)
			delete(Orphan_tx, rightTip)
		}
	} else {
		dag.Mux.Unlock()
		fmt.Println("dag released by verifytips")
	}
	return true
}

// verify vertex (Signature, POW)
func VerifyVertex(vertex Vertex, dag *DAG) bool {

	// verify the signature
	// data, _ := Serialize(vertex.Tx)
	data := vertex.Tx.Hash[:]
	if !crypt.Verify(vertex.Tx.From, data, vertex.Signature) {
		fmt.Println("Signature verification failed")
		return false
	}

	// verify the POW
	if vertex.Tx.Type == 0 {
		difficulty := 3
		hexHash := hex.EncodeToString(vertex.Tx.Hash[:])
		if hexHash[:difficulty] != strings.Repeat("0", difficulty) {
			fmt.Println("POW verification failed")
			return false
		}
	} else if vertex.Tx.Type == 1 {
		difficulty := powDifficulty
		hexHash := hex.EncodeToString(vertex.Tx.Hash[:])
		if hexHash[:difficulty] != strings.Repeat("0", difficulty) {
			fmt.Println("POW verification failed")
			return false
		}
	} else {
		difficulty := powDifficulty
		hexHash := hex.EncodeToString(vertex.Tx.Hash[:])
		if hexHash[:difficulty] != strings.Repeat("0", difficulty) {
			fmt.Println("POW verification failed")
			return false
		}
	}
	return true
}

func AddVtxToDAG(vertex Vertex, dag *DAG, privateKey *ecdsa.PrivateKey) {
	// add the vertex to the graph
	fmt.Println("verifying vtx: ", vertex.Vertex_no)
	if !VerifyVertex(vertex, dag) {
		orphan_count += 1
		time2 := time.Now()
		// sig := hex.EncodeToString(vertex.Signature[:])
		// hash := hex.EncodeToString(vertex.Tx.Hash[:])
		fmt.Println("Orphan Count: ", time2.Sub(time1), orphan_count, len(Orphan_tx), totalTx)
		fmt.Println("Vertex Verification Failed")
		return
	}
	//lock the dag
	fmt.Println("verifying vtx tips: ", vertex.Vertex_no)
	if !VerifyTips(vertex, dag) {
		fmt.Println("Vertex Verification Tips Failed")
		// handle orphan nodes
		Orphan_tx[hex.EncodeToString(vertex.Tx.Hash[:])] = vertex
		orphan_count += 1
		time2 := time.Now()
		dag.Mux.Lock()
		// leftTip := hex.EncodeToString(vertex.Tx.LeftTip[:])
		// rightTip := hex.EncodeToString(vertex.Tx.RightTip[:])
		fmt.Println("Orphan Count: ", time2.Sub(time1), orphan_count, len(Orphan_tx), totalTx)
		dag.Mux.Unlock()
		return
	}
	// hash of the transaction is the key
	hash := hex.EncodeToString(vertex.Tx.Hash[:])
	fmt.Println("locking dag: ", vertex.Vertex_no)
	dag.Mux.Lock()
	fmt.Println("locked dag: ", vertex.Vertex_no)
	dag.Graph[hash] = vertex

	// update the neighbours of the lefttip and righttip
	leftTip := hex.EncodeToString(vertex.Tx.LeftTip[:])
	rightTip := hex.EncodeToString(vertex.Tx.RightTip[:])

	// var tip Vertex
	// tip = vertex
	// if leftTip exist in dag
	if tip, ok := dag.Graph[leftTip]; ok {
		tip.Neighbours = append(tip.Neighbours, hash)

		dag.Graph[leftTip] = tip
	}

	// if rightTip exist in dag

	if tip, ok := dag.Graph[rightTip]; ok && strings.Compare(rightTip, leftTip) != 0 {
		tip.Neighbours = append(tip.Neighbours, hash)

		dag.Graph[rightTip] = tip
	}

	dag.Mux.Unlock()
	fmt.Println("unlocking dag: ", vertex.Vertex_no)
	totalTx += 1
	// update weight of the vertices

	UpdateWeights(dag, hash)
	// prune the dag
	Prune(dag, pruneThreshold)
}

// Add the vertex to the dag without verifying the vertex
func UploadToDAG(vtx Vertex, dag *DAG) {
	// hash of the transaction is the key
	hash := hex.EncodeToString(vtx.Tx.Hash[:])

	fmt.Println("dag acquiring by upload")
	dag.Mux.Lock()
	dag.Graph[hash] = vtx

	// update the neighbours of the lefttip and righttip
	leftTip := hex.EncodeToString(vtx.Tx.LeftTip[:])
	rightTip := hex.EncodeToString(vtx.Tx.RightTip[:])

	// if leftTip exist in dag
	if tip, ok := dag.Graph[leftTip]; ok {
		tip.Neighbours = append(tip.Neighbours, hash)

		dag.Graph[leftTip] = tip
	}

	// if rightTip exist in dag

	if tip, ok := dag.Graph[rightTip]; ok && strings.Compare(rightTip, leftTip) != 0 {
		tip.Neighbours = append(tip.Neighbours, hash)

		dag.Graph[rightTip] = tip
	}

	dag.Mux.Unlock()
	fmt.Println("dag released by upload")
	totalTx += 1
	// update weight of the vertices
	UpdateWeights(dag, hash)
	// prune the dag
	Prune(dag, pruneThreshold)
}

func InitDag(dag *DAG) {

	dag.Graph = make(map[string]Vertex)
	Orphan_tx = make(map[string]Vertex)
}
