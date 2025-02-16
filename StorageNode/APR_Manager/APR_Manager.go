package APR_Manager

import (
	"encoding/hex"
	"fmt"
	"time"
	dh "StorageNode/Data_Handler"
	"golang.org/x/exp/rand"
)

type AdamPointState struct {
	Timestamp int64
	AdamPoint string
	AdamPaths []string
}

var (
	bitLen         = 16
	thresholdTime  = (5 * time.Minute).Nanoseconds()
	adamIterations = 5
)

func getNeighbourWithProbability(dag *dh.DAG, neighbours []string) string {
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
func RandomWalk(dag *dh.DAG, hash string, traverseLen int, visited map[string]int) {
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
func findAdamPoint(dag *dh.DAG, pruneList []string, iterations int) string {
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

// prune the dh.DAG (from GW's data_handler)
func getTxToBePruned(dag *dh.DAG) []string {
	fmt.Println("Pruning the dh.DAG")
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

func removeChildrenRecusrive(dag *dh.DAG, pruneMap map[string]bool, parent string) {
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

func removeChildrenFromPruneList(dag *dh.DAG, pruneList []string) []string {
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

func adamPathGeneration(dag *dh.DAG, currNode string, path string, pathSet map[string]string, compressedPruneList map[string]bool) {
	if len(path) >= bitLen || currNode == "" {
		return
	}
	// if the currNode in pathSet then return
	if _, ok := pathSet[currNode]; ok {
		return
	}

	// if currNode in compressedPruneList then add to pathSet
	if _, ok := compressedPruneList[currNode]; ok {
		pathSet[currNode] = path
	}

	// get the vertex
	dag.Mux.Lock()
	if vertex, ok := dag.Graph[currNode]; ok {
		dag.Mux.Unlock()
		// get the children of the vertex
		leftTip := hex.EncodeToString(vertex.Tx.LeftTip[:])
		rightTip := hex.EncodeToString(vertex.Tx.RightTip[:])

		adamPathGeneration(dag, leftTip, path+"0", pathSet, compressedPruneList)
		adamPathGeneration(dag, rightTip, path+"1", pathSet, compressedPruneList)
	} else {
		dag.Mux.Unlock()
	}

}

func getTransactionFromAdamPoint(dag *dh.DAG, adamPoint string, path string) string {
	if path == "" {
		return adamPoint
	}

	currNode := adamPoint
	for i := 0; i < len(path); i++ {
		dag.Mux.Lock()
		if vertex, ok := dag.Graph[currNode]; ok {
			dag.Mux.Unlock()
			if path[i] == '0' {
				currNode = hex.EncodeToString(vertex.Tx.LeftTip[:])
			} else {
				currNode = hex.EncodeToString(vertex.Tx.RightTip[:])
			}
		} else {
			dag.Mux.Unlock()
		}
	}

	return currNode
}

func aprCompression(dag *dh.DAG, adamPoint string, compressedPruneList []string) []string {
	pathSet := make(map[string]string)
	compressedPruneListMap := make(map[string]bool)
	for _, hash := range compressedPruneList {
		compressedPruneListMap[hash] = true
	}
	adamPathGeneration(dag, adamPoint, "", pathSet, compressedPruneListMap)

	// verify the compression
	missed := 0
	for _, hash := range compressedPruneList {
		if _, ok := pathSet[hash]; !ok {
			missed++
		}
	}

	// also check if the paths reach the transaction
	notReached := 0
	for hash, path := range pathSet {
		txHash := getTransactionFromAdamPoint(dag, adamPoint, path)
		if txHash != hash {
			notReached++
		}
	}

	fmt.Println("aprCompression: Missed: ", missed, " Total: ", len(compressedPruneList), "Unreachable: ", notReached)

	aprList := make([]string, 0)
	for _, path := range pathSet {
		aprList = append(aprList, path)
	}

	return aprList
}

func GeneratePruneList(dag *dh.DAG) []string {
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

	adamPointCompressedPruneList := aprCompression(dag, adamPoint, compressedPruneList)

	// add adam point to the prune list at the start of the list
	adamPointCompressedPruneList = append([]string{adamPoint}, adamPointCompressedPruneList...)

	return adamPointCompressedPruneList
}

func generateChildren(dag *dh.DAG, pruneList []string, compressedPruneList []string, decompressedPruneMap map[string]bool) {
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

func addChildren(dag *dh.DAG, hash string, pruneListMap map[string]bool, decompressedPruneMap map[string]bool, visited map[string]bool) {
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

func verifyCompression(dag *dh.DAG, pruneList []string, compressedPruneList []string) bool {
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
