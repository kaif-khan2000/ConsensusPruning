package main

import (
	crypt "StorageNode/Crypt_key_Manager"
	dh "StorageNode/Data_Handler"
	p2p "StorageNode/P2P_Manager"
	sc "StorageNode/SC_Handler"
	"bufio"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"time"

	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
	"github.com/gin-gonic/gin"
)

var (
	dag          dh.DAG
	chain        sc.SideChain
	aprBucket    []dh.AdamPointState
	gwPrivateKey *ecdsa.PrivateKey
	gwPublicKey  ecdsa.PublicKey
)

// get the image of the dag
func getDagImage(c *gin.Context) {
	// sort the dag graph by timestamp
	dag.Mux.Lock()
	dagGraph := dag.Graph
	dag.Mux.Unlock()
	keys := make([]string, 0, len(dagGraph))
	for k := range dagGraph {
		keys = append(keys, k)
	}

	// sort the keys
	sort.SliceStable(keys, func(i, j int) bool {
		return dagGraph[keys[i]].Tx.Timestamp < dagGraph[keys[j]].Tx.Timestamp
	})

	keyMap := make(map[string]string)
	strHash := func(c string) string {
		return c
	}
	// create the dag image
	g := graph.New(strHash, graph.Directed())
	for i := range keys {
		name := strconv.Itoa(dagGraph[keys[i]].Weight) + ":" + strconv.Itoa(i+1)
		_ = g.AddVertex(name)
		keyMap[keys[i]] = name
	}

	// add the edges
	dag.Mux.Lock()
	for key, value := range keyMap {
		left := dagGraph[key].Tx.LeftTip
		right := dagGraph[key].Tx.RightTip
		leftTip := hex.EncodeToString(left[:])
		rightTip := hex.EncodeToString(right[:])
		v1 := keyMap[leftTip]
		v2 := keyMap[rightTip]
		err := g.AddEdge(value, v1, graph.EdgeWeight(1))
		if err != nil {
			if err.Error() != "target vertex 0: vertex not found" {
				fmt.Println("Error in adding the edge: ", err.Error())
			}
		}
		if keyMap[leftTip] != keyMap[rightTip] {
			err = g.AddEdge(value, v2, graph.EdgeWeight(2))
			if err != nil {
				if err.Error() != "target vertex 0: vertex not found" {
					fmt.Println("Error in adding the edge: ", err.Error())
				}
			}
		}
	}
	dag.Mux.Unlock()

	file, err := os.Create("image.gv")
	_ = draw.DOT(g, file)
	fmt.Println("Graph created,", file, err)

	_, err = exec.Command("dot", "-Tpng", "image.gv", "-o", "pics/image.png").Output()
	if err != nil {
		fmt.Println("Error in creating the image: ", err.Error())
	}
	img, err := os.Open("pics/image.png")
	if err != nil {
		fmt.Println("Error in opening the image: ", err.Error())
	}
	defer img.Close()
	imgInfo, _ := img.Stat()
	var size int64 = imgInfo.Size()
	imgBytes := make([]byte, size)
	// read image into bytes
	buffer := bufio.NewReader(img)
	_, err = buffer.Read(imgBytes)
	if err != nil {
		fmt.Println("Error in reading the image: ", err.Error())
	}

	// conert to base64
	// imgBase64 := base64.StdEncoding.EncodeToString(imgBytes)
	// render the image
	c.File("index.html")
}

// get the image of the dag
func getChainImage(c *gin.Context) {
	// sort the dag graph by timestamp
	chain.Mux.Lock()
	chainGraph := chain.Chain
	chain.Mux.Unlock()
	keys := make([]string, 0, len(chainGraph))
	for k := range chainGraph {
		keys = append(keys, k)
	}

	// sort the keys
	sort.SliceStable(keys, func(i, j int) bool {
		return chainGraph[keys[i]].Timestamp < chainGraph[keys[j]].Timestamp
	})

	keyMap := make(map[string]string)
	strHash := func(c string) string {
		return c
	}
	// create the dag image
	g := graph.New(strHash, graph.Directed())
	for i := range keys {
		name := strconv.Itoa(i + 1)
		_ = g.AddVertex(name)
		keyMap[keys[i]] = name
	}

	// add the edges
	dag.Mux.Lock()
	for key, value := range keyMap {
		next := chainGraph[key].PrevHash
		nextNode := hex.EncodeToString(next[:])
		v1 := keyMap[nextNode]
		err := g.AddEdge(value, v1, graph.EdgeWeight(1))
		if err != nil {
			if err.Error() != "target vertex 0: vertex not found" {
				fmt.Println("Error in adding the edge: ", err.Error())
			}
		}
	}
	dag.Mux.Unlock()

	file, err := os.Create("image.gv")
	_ = draw.DOT(g, file)
	fmt.Println("Graph created,", file, err)

	_, err = exec.Command("dot", "-Tpng", "image.gv", "-o", "pics/image.png").Output()
	if err != nil {
		fmt.Println("Error in creating the image: ", err.Error())
	}
	img, err := os.Open("pics/image.png")
	if err != nil {
		fmt.Println("Error in opening the image: ", err.Error())
	}
	defer img.Close()
	imgInfo, _ := img.Stat()
	var size int64 = imgInfo.Size()
	imgBytes := make([]byte, size)
	// read image into bytes
	buffer := bufio.NewReader(img)
	_, err = buffer.Read(imgBytes)
	if err != nil {
		fmt.Println("Error in reading the image: ", err.Error())
	}

	// conert to base64
	// imgBase64 := base64.StdEncoding.EncodeToString(imgBytes)
	// render the image
	c.File("index.html")
}

func generatePruningInformation() {
	for {
		time.Sleep(10 * time.Minute)
		pruneList := dh.GeneratePruneList(&dag)
		fmt.Println("PruneList: ", pruneList)

		// compress the prune list
		var adamPointState dh.AdamPointState
		adamPointState = dh.GetAdamPointState(pruneList)
		fmt.Println("adamPointState: ", adamPointState)

		// create a sc block
		adamPointStateBytes, _ := dh.Serialize(adamPointState)
		scBlock := sc.CreateSideChainBlock(string(adamPointStateBytes), time.Now().Unix(), gwPrivateKey)
		scBlock.Pow()
		scBlock.SignBlock(gwPrivateKey)

		// check if a new block has been added to the side chain
		chain.Mux.Lock()

		if chain.LatestBlockHash != hex.EncodeToString(scBlock.Hash[:]) {
			chain.Mux.Unlock()
			continue
		}
		chain.Mux.Unlock()

		sc.AddToSideChain(&chain, scBlock)
		p2p.BroadcastSCBlock(*scBlock)
	}
}

func getTransaction(c *gin.Context) {
	hash := c.Param("hash")
	fmt.Println("Hash: ", hash)
	// get the transaction from the dag
	dag.Mux.Lock()
	tx := dag.Graph[hash].Tx
	dag.Mux.Unlock()

	c.IndentedJSON(http.StatusOK, tx)

}

func getIH(c *gin.Context) {
	var tx_id_array []string
	c.BindJSON(&tx_id_array)

	ih_array := make(map[int]string, 0)
	for i := range tx_id_array {
		// get the IH's from the dag
		dag.Mux.Lock()
		ih := dag.Graph[tx_id_array[i]].Tx.IntermediateHashes
		count := dag.Graph[tx_id_array[i]].Tx.IhSequenceStart
		dag.Mux.Unlock()
		for k := range ih {
			hash := hex.EncodeToString(ih[k][:])
			ih_array[count] = hash
			count++
		}
	}

	c.IndentedJSON(http.StatusOK, ih_array)
}

func getMH(c *gin.Context) {
	tx_id := c.Param("txId")
	// get the MH from the dag
	fmt.Println("tx_id: ", tx_id)
	dag.Mux.Lock()
	mh := dag.Graph[tx_id].Tx.MerkleRoot
	dag.Mux.Unlock()
	hash := hex.EncodeToString(mh[:])
	fmt.Println("mh: ", mh)
	fmt.Println("hash: ", hash)
	c.IndentedJSON(http.StatusOK, hash)
}

func gwHTTPServer() {
	// gin server
	router := gin.Default()
	router.Static("/pics", "./pics")
	router.GET("/getTransaction/:hash", getTransaction)
	router.POST("/getIH", getIH)
	router.GET("/getMH/:txId", getMH)
	router.GET("/getDagImage", getDagImage)
	router.GET("/getChainImage", getChainImage)
	router.Run("0.0.0.0:4000")
}

func main() {

	// create key pairs
	gwPrivateKey, gwPublicKey = crypt.GenerateKeyPair()

	fmt.Println("pubKey: ", gwPublicKey)

	genesis := dh.CreateGenesisTx()
	// Init DAG
	dh.InitDag(&dag, genesis, gwPrivateKey)

	chain = *sc.InitSideChain()

	go gwHTTPServer()
	// // inserting genesis
	// err = dbh.InsertGenesisTx(db, genesisTx)
	// if err != nil {
	// 	fmt.Println("Error inserting genesis: ", err.Error())
	// 	return
	// }

	go p2p.StorageNode(&dag, &chain)
	fmt.Println("Storage node started successfully")

	generatePruningInformation()

}
