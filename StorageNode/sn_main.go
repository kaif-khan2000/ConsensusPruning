package main

import (
	crypt "StorageNode/Crypt_key_Manager"
	dh "StorageNode/Data_Handler"
	p2p "StorageNode/P2P_Manager"
	"sync"

	// pbft "StorageNode/Pbft_Consensus"
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
	isMyTurn     = make(chan bool)
	proposalChan = make(chan []byte)
	responseChan = make(chan bool)
	aprBucket    []dh.AdamPointState
	gwPrivateKey *ecdsa.PrivateKey
	gwPublicKey  ecdsa.PublicKey
	scBlocks     = make(map[int]sc.SideChainBlock)
	scBlocksMux  sync.Mutex
	txSize       = 208    // bytes;
	txRate       = 3      // tx/sec
	gwCap        = 200000 // bytes
	scThresh     = 4
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
	// time.Sleep(9 * time.Minute)
	fmt.Println("pbft: sn_main: waiting for my turn to generate pruning information")
	count := 0
	time.Sleep(6 * time.Minute)
	for {
		// _ = <-isMyTurn
		count++
		fmt.Println("pbft: sn_main: my turn to generate pruning information")
		epoch := gwCap / (scThresh * txSize * txRate)
		// convert int to time duration
		epochDuration := time.Duration(epoch) * time.Second
		time.Sleep(epochDuration)
		
		time1 := time.Now().UnixNano()
		rl, _, pl := dh.GeneratePruneList(&dag)
		fmt.Println("PruneList: ", rl)

		// compress the prune list
		var adamPointState dh.AdamPointState
		adamPointState = dh.GetAdamPointState(rl)
		fmt.Println("adamPointState: ", adamPointState)

		// create a sc block
		adamPointStateBytes, _ := dh.Serialize(adamPointState)
		scBlock := sc.CreateSideChainBlock(string(adamPointStateBytes), chain.LatestBlockHash, time.Now().Unix(), gwPrivateKey)
		// scBlock.Pow()
		// fmt.Println("adamPointState: pow done..........")
		scBlock.SignBlock(gwPrivateKey)
		scBlockBytes, _ := dh.Serialize(scBlock)

		// plBytes, _ := dh.Serialize(pl)

		fmt.Println("pbft: sn_main: proposal to pbft module: ", string(scBlockBytes))
		// // proposalChan <- plBytes
		// proposalChan <- scBlockBytes
		// // wait for the response
		// response := <-responseChan
		// fmt.Println("pbft: sn_main: response received from pbft module: ", response)
		// if !response {
		// 	fmt.Println("pbft: adamPointState: proposal rejected")
		// 	continue
		// }
		// check if a new block has been added to the side chain
		chain.Mux.Lock()

		if chain.LatestBlockHash != hex.EncodeToString(scBlock.PrevHash[:]) {
			fmt.Println("adamPointState: useless - ", chain.LatestBlockHash, hex.EncodeToString(scBlock.PrevHash[:]))
			chain.Mux.Unlock()
			continue
		}
		chain.Mux.Unlock()

		time2 := time.Now().UnixNano()
		fmt.Println("Time taken to generate pruning information: ", (time2-time1)/1000000, "ms")
		fmt.Println("2time: ", len(pl), (time2-time1)/1000000, "ms")

		sc.AddToSideChain(&chain, scBlock)
		// p2p.BroadcastSCBlock(*scBlock)
		strBlock, _ := dh.Serialize(scBlock)
		// dh.AdamPointPrune(rl[0], rl[0:], &dag)
		// dh.PRLPrune(prl, &dag)
		dag.Mux.Lock()
		fmt.Println("txRate: ", time.Now().UnixMilli(), len(dag.Graph))
		dag.Mux.Unlock()
		scBlocksMux.Lock()
		scBlocks[count] = *scBlock
		scBlocksMux.Unlock()
		fmt.Println("adamPointState: block added to the side chain..........", string(strBlock))
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

func getLatestSCBlock(c *gin.Context) {
	chain.Mux.Lock()
	latestBlockHash := chain.LatestBlockHash
	latestBlock := chain.Chain[latestBlockHash]
	chain.Mux.Unlock()
	c.IndentedJSON(http.StatusOK, latestBlock)
}

func getLatestPruningList(c *gin.Context) {
	chain.Mux.Lock()
	latestBlockHash := chain.LatestBlockHash
	latestBlock := chain.Chain[latestBlockHash]
	chain.Mux.Unlock()
	pruningListString := latestBlock.Data
	if pruningListString == "Genesis Block" {
		c.IndentedJSON(http.StatusOK, pruningListString)
		return
	}
	var pruningList dh.AdamPointState
	dh.Deserialize(pruningListString, &pruningList)
	c.IndentedJSON(http.StatusOK, pruningList)
}

func getPruneList(c *gin.Context) {
	// read the slug
	slug := c.Param("count")
	// convert the slug to int
	count, err := strconv.Atoi(slug)
	if err != nil {
		fmt.Println("Error in converting the slug to int: ", err.Error())
		c.IndentedJSON(http.StatusBadRequest, "Error in converting the slug to int")
		return
	}
	// check if the count is present in the map
	scBlocksMux.Lock()
	block, ok := scBlocks[count]
	scBlocksMux.Unlock()
	if !ok {
		c.IndentedJSON(http.StatusBadRequest, "Block not found")
		return
	}
	// get the pruning list from the block
	var pruningList dh.AdamPointState
	dh.Deserialize(block.Data, &pruningList)
	c.IndentedJSON(http.StatusOK, pruningList)
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
	router.GET("/getLatestSCBlock", getLatestSCBlock)
	router.GET("/getLatestPruneList", getLatestPruningList)
	router.GET("/getPruneList/:count", getPruneList)
	router.Run("0.0.0.0:4000")
}

func main() {

	// create key pairs
	gwPrivateKey, gwPublicKey = crypt.GenerateKeyPair()

	fmt.Println("pubKey: ", gwPublicKey)

	pkstring := hex.EncodeToString(crypt.SerializePublicKey(gwPrivateKey.PublicKey))
	fmt.Println("pkstring: ", pkstring)

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

	time.Sleep(5 * time.Second)
	fmt.Println("pbft: sn_main: did not start pruning")
	// go pbft.RunPbftConsensusModule(pkstring, &isMyTurn, &proposalChan, &responseChan)

	generatePruningInformation()
}
