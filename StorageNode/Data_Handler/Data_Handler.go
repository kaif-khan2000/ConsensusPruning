package Data_Handler

import (
	crypt "StorageNode/Crypt_key_Manager"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"math/rand"
)

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

var (
	bitLen         = 16
	thresholdTime  = (5 * time.Minute).Nanoseconds()
	adamIterations = 5
)

// serializes the object to a byte array
func Serialize(p interface{}) ([]byte, error) {
	return json.Marshal(p)
}

// deserializes the byte array to an object
func Deserialize(data string, p any) error {
	return json.Unmarshal(json.RawMessage(data), p)
}

func getNeighbourWithProbability(dag *DAG, neighbours []string) string {
	// get the weight of the neighbours
	weights := make([]int, len(neighbours))
	totalWeight := 0
	for i, neighbour := range neighbours {
		dag.Mux.Lock()
		if neighbourVertex, ok := dag.Graph[neighbour]; ok {
			dag.Mux.Unlock()
			weights[i] = neighbourVertex.Weight
			totalWeight += neighbourVertex.Weight
		} else {
			dag.Mux.Unlock()
		}
	}

	// select a random neighbour
	// fmt.Println("PruneListCompression: Total Weight: ", totalWeight)
	random := rand.Intn(totalWeight+1) - 1
	// select the neighbour
	selectedNeighbour := ""
	currWeight := 0
	for i, weight := range weights {
		currWeight += weight
		if random < currWeight {
			selectedNeighbour = neighbours[i]
			break
		}
	}

	return selectedNeighbour
}

// Random walk of bitLen size path from a node backwards
func RandomWalk(dag *DAG, hash string, traverseLen int, visited map[string]int) {
	if hash == "" || traverseLen == 0 {
		return
	}

	visited[hash] += 1
	// get the vertex
	dag.Mux.Lock()
	if vertex, ok := dag.Graph[hash]; ok {
		dag.Mux.Unlock()

		neighbours := vertex.Neighbours
		if len(neighbours) == 0 {
			return
		}
		selectedNeighbour := getNeighbourWithProbability(dag, neighbours)

		RandomWalk(dag, selectedNeighbour, traverseLen-1, visited)
	} else {
		dag.Mux.Unlock()
	}

}

// find the adamPoint
func findAdamPoint(dag *DAG, pruneList []string, iterations int) string {
	// find the adam point
	visited := make(map[string]int)
	for _, hash := range pruneList {
		for i := 0; i < iterations; i++ {
			RandomWalk(dag, hash, bitLen, visited)
		}
	}

	// find the node with max visits
	maxVisits := 0
	adamPoint := ""
	for hash, visits := range visited {
		if visits > maxVisits {
			maxVisits = visits
			adamPoint = hash
		}
	}

	return adamPoint
}

// prune the DAG (from GW's data_handler)
func getTxToBePruned(dag *DAG) []string {
	fmt.Println("Pruning the DAG")
	count := 0
	hashArray := make([]string, 0)
	dag.Mux.Lock()

	// collect all the transactions that are confirmed and have weight greater than threshold and exceed the threshold time
	// thresholdTime := (10 * time.Minute).Nanoseconds()
	for hash, vertex := range dag.Graph {
		if time.Now().UnixNano()-vertex.Tx.Timestamp > int64(thresholdTime) {
			// delete(dag.Graph, hash)
			count++
			hashArray = append(hashArray, hash)
		}
	}
	dag.Mux.Unlock()

	return hashArray
}

func removeChildrenRecusrive(dag *DAG, pruneMap map[string]bool, parent string) {
	// remove the children of the parent from the prune list
	// get the vertex
	dag.Mux.Lock()
	if vertex, ok := dag.Graph[parent]; ok {
		dag.Mux.Unlock()
		// get the children of the vertex
		leftTip := hex.EncodeToString(vertex.Tx.LeftTip[:])
		rightTip := hex.EncodeToString(vertex.Tx.RightTip[:])

		if _, ok := pruneMap[leftTip]; ok {
			delete(pruneMap, leftTip)
			removeChildrenRecusrive(dag, pruneMap, leftTip)
		}
		if _, ok := pruneMap[rightTip]; ok {
			delete(pruneMap, rightTip)
			removeChildrenRecusrive(dag, pruneMap, rightTip)
		}
	} else {
		dag.Mux.Unlock()
	}
}

func removeChildrenFromPruneList(dag *DAG, pruneList []string) []string {
	// if the parent is getting pruned then the children should also be pruned
	// therefore remove the children from the prune list

	// store the string array in a map
	pruneMap := make(map[string]bool)
	for _, hash := range pruneList {
		pruneMap[hash] = true
	}

	pruneListLen := len(pruneList)

	for hash := range pruneMap {
		removeChildrenRecusrive(dag, pruneMap, hash)
	}

	//clear the prune list
	// pruneList = pruneList[:0]
	compressedPruneList := make([]string, 0)

	// add the keys back to the prune list
	for hash := range pruneMap {
		compressedPruneList = append(compressedPruneList, hash)
	}

	newPruneListLen := len(compressedPruneList)
	// calculate the compression ratio
	compressionRatio := float64(newPruneListLen) / float64(pruneListLen)
	fmt.Println("PruneListCompression: (", pruneListLen, newPruneListLen, compressionRatio*100, "%)")

	return compressedPruneList

}

func GeneratePruneList(dag *DAG) []string {
	// get the list of transactions to be pruned
	pruneList := getTxToBePruned(dag)

	compressedPruneList := removeChildrenFromPruneList(dag, pruneList)

	if verifyCompression(dag, pruneList, compressedPruneList) {
		fmt.Println("PruneListCompression: Compression successful")
	} else {
		fmt.Println("PruneListCompression: Compression failed")
	}

	adamPoint := findAdamPoint(dag, compressedPruneList, adamIterations)
	fmt.Println("Adam Point: ", adamPoint)

	if verifyCompression(dag, compressedPruneList, []string{adamPoint}) {
		fmt.Println("PruneListCompression: Adampoint Compression successful")
	} else {
		fmt.Println("PruneListCompression: Adampoint Compression failed")
	}

	return compressedPruneList
}

func generateChildren(dag *DAG, pruneList []string, compressedPruneList []string, decompressedPruneMap map[string]bool) {
	pruneListMap := make(map[string]bool)
	for _, hash := range pruneList {
		pruneListMap[hash] = true
	}

	visited := make(map[string]bool)
	// now traverse through dag and get the children of the compressed prune list
	for _, hash := range compressedPruneList {
		addChildren(dag, hash, pruneListMap, decompressedPruneMap, visited)
	}
}

func addChildren(dag *DAG, hash string, pruneListMap map[string]bool, decompressedPruneMap map[string]bool, visited map[string]bool) {
	if _, ok := visited[hash]; ok {
		return
	}

	visited[hash] = true
	if _, ok := pruneListMap[hash]; ok {
		decompressedPruneMap[hash] = true
	}

	// get the vertex
	dag.Mux.Lock()
	if vertex, ok := dag.Graph[hash]; ok {
		dag.Mux.Unlock()
		// get the children of the vertex
		leftTip := hex.EncodeToString(vertex.Tx.LeftTip[:])
		rightTip := hex.EncodeToString(vertex.Tx.RightTip[:])

		addChildren(dag, leftTip, pruneListMap, decompressedPruneMap, visited)
		addChildren(dag, rightTip, pruneListMap, decompressedPruneMap, visited)
	} else {
		dag.Mux.Unlock()
	}
}

func verifyCompression(dag *DAG, pruneList []string, compressedPruneList []string) bool {
	// check if all the elements from the prune list can be reached from the compressed prune list
	// if not then the compression failed

	// store the compressed prune list in a map
	decompressedPruneMap := make(map[string]bool)

	// check if all the elements from the prune list can be reached from the compressed prune list
	generateChildren(dag, pruneList, compressedPruneList, decompressedPruneMap)

	// check if all the elements from the prune list can be reached from the compressed prune list
	missed := 0
	for _, hash := range pruneList {
		if _, ok := decompressedPruneMap[hash]; !ok {
			missed++
		}
	}
	fmt.Println("PruneListCompression: Missed: ", missed, " Total: ", len(pruneList))
	return missed == 0
}

func updateWeightOfChildren(dag *DAG, children map[string]bool) {
	// update the weight of the children
	for child := range children {
		dag.Mux.Lock()
		if vertex, ok := dag.Graph[child]; ok {
			vertex.Weight++
			dag.Graph[child] = vertex
		}
		dag.Mux.Unlock()
	}
}

func getChildren(dag *DAG, tip [32]byte, children map[string]bool, iteration int) {
	// get the child of child iteratiely until the length of the path is 16
	if iteration == 0 {
		return
	}

	// get the vertex
	hash := hex.EncodeToString(tip[:])
	dag.Mux.Lock()
	if vertex, ok := dag.Graph[hash]; ok {
		dag.Mux.Unlock()
		// get the children of the vertex
		leftTip := hex.EncodeToString(vertex.Tx.LeftTip[:])
		rightTip := hex.EncodeToString(vertex.Tx.RightTip[:])

		children[leftTip] = true
		children[rightTip] = true

		// get the children of the left tip
		getChildren(dag, vertex.Tx.LeftTip, children, iteration-1)
		// get the children of the right tip
		getChildren(dag, vertex.Tx.RightTip, children, iteration-1)
	} else {
		dag.Mux.Unlock()
	}

}

func UpdateWeight(dag *DAG, hash string) {
	// get all the unique children of the vertex with max 16 len path

	// get the vertex
	currVertex := dag.Graph[hash]
	// get the children of the vertex from the tips
	children := make(map[string]bool)
	getChildren(dag, currVertex.Tx.Hash, children, bitLen)

	updateWeightOfChildren(dag, children)

}

// verify vertex (Signature, POW, lefttip, righttip)
func VerifyVertex(vertex Vertex, dag *DAG) (bool, string) {

	// verify the signature

	// data, err := Serialize(vertex.Tx)
	// if err != nil {
	// 	fmt.Println("Serialization failed: ", err)
	// 	return false, "serialization"
	// }
	data := vertex.Tx.Hash[:]
	if !crypt.Verify(vertex.Tx.From, data, vertex.Signature) {
		fmt.Println("Signature verification failed: ", vertex.Tx.Type, vertex.Tx.From, data, vertex.Signature)
		fmt.Println("sig fail packet: ", vertex.Tx)
		sig := hex.EncodeToString(vertex.Signature[:])
		return false, "signature " + sig
	}

	// verify the POW
	if vertex.Tx.Type == 0 {
		difficulty := 3
		hexHash := hex.EncodeToString(vertex.Tx.Hash[:])
		if hexHash[:difficulty] != strings.Repeat("0", difficulty) {
			return false, "POW"
		}
	} else if vertex.Tx.Type == 1 {
		difficulty := 4
		hexHash := hex.EncodeToString(vertex.Tx.Hash[:])
		if hexHash[:difficulty] != strings.Repeat("0", difficulty) {
			fmt.Println("POW verification failed type - 1")
			return false, "POW"
		}
	} else {
		difficulty := 4
		hexHash := hex.EncodeToString(vertex.Tx.Hash[:])
		if hexHash[:difficulty] != strings.Repeat("0", difficulty) {
			fmt.Println("POW verification failed type - 2")
			return false, "POW"
		}
	}
	leftTip := hex.EncodeToString(vertex.Tx.LeftTip[:])
	rightTip := hex.EncodeToString(vertex.Tx.RightTip[:])
	// if leftTip exist in dag
	dag.Mux.Lock()
	if _, ok := dag.Graph[leftTip]; !ok {
		fmt.Println("Left tip not found")
		dag.Mux.Unlock()
		return false, "left tip " + leftTip
	}
	// if rightTip exist in dag
	if _, ok := dag.Graph[rightTip]; !ok {
		fmt.Println("Right tip not found")
		dag.Mux.Unlock()
		return false, "right tip " + rightTip
	}
	dag.Mux.Unlock()
	return true, ""
}

// adds the vertex to the DAG
func AddToDAG(vertex Vertex, dag *DAG) {

	// add the vertex to the graph
	// hash of the transaction is the key
	hash := hex.EncodeToString(vertex.Tx.Hash[:])
	dag.Mux.Lock()
	dag.Graph[hash] = vertex

	// update the neighbours of the lefttip and righttip
	leftTip := hex.EncodeToString(vertex.Tx.LeftTip[:])
	rightTip := hex.EncodeToString(vertex.Tx.RightTip[:])

	//add the vertex as neighbour of its left tip and right tip
	// this is done for the purpose of backtracking etc.

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

	// update weight of the vertices
	dag.Mux.Unlock()

	// update the weight of the children
	UpdateWeight(dag, hash)
}

func CreateVertex(transaction Transaction, privateKey *ecdsa.PrivateKey) Vertex {
	var vertex Vertex
	vertex.Tx = transaction
	vertex.Weight = 1
	vertex.Neighbours = make([]string, 0)
	data, _ := Serialize(transaction)
	vertex.Signature = crypt.Sign(privateKey, data)
	return vertex
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

func InitDag(dag *DAG, genesis Transaction, privateKey *ecdsa.PrivateKey) {
	dag.Graph = make(map[string]Vertex)
	// inserting data
	// deserializing genesis
	// creating genesis vertex
	genesisVertex := CreateVertex(genesis, privateKey)
	// add genesis to dag
	AddToDAG(genesisVertex, dag)

}
