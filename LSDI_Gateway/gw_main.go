package main

import (
	ckm "LSDI_Gateway/Crypt_key_Manager"
	dh "LSDI_Gateway/Data_Handler"
	p2p "LSDI_Gateway/P2P_Manager"
	"net/http"
	"strconv"

	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	// for dag image
	"bufio"
	"encoding/hex"
	"os"
	"os/exec"
	"sort"

	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"

	"github.com/gin-gonic/gin"
)

// global variables
var (
	// Chunk array
	dag          dh.DAG
	gwPrivateKey *ecdsa.PrivateKey
	gwPublicKey  ecdsa.PublicKey
	TxRate       float64
)

// get the image of the dag
func getDagImage(c *gin.Context) {
	// sort the dag graph by timestamp
	dagGraph := dag.Graph
	keys := make([]string, 0, len(dagGraph))
	for k := range dagGraph {
		keys = append(keys, k)
	}

	dag.Mux.Lock()
	fmt.Println("dag acquired by getDagImage")
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
		name := strconv.Itoa(dagGraph[keys[i]].Weight) + ":" + strconv.Itoa(dagGraph[keys[i]].Vertex_no) + ":" + strconv.Itoa(dagGraph[keys[i]].Gateway_no)
		// if weight == 0 or 1 and gateway_no == 0 and vertex_no == 0 then print keys[i]
		if dagGraph[keys[i]].Gateway_no == 0 && dagGraph[keys[i]].Vertex_no == 0 {
			fmt.Println("Orphan Count: ", keys[i])
		}
		_ = g.AddVertex(name)
		keyMap[keys[i]] = name
	}

	// add the edges
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

func simulateTransactions() {
	fmt.Println("Running the simulation")
	for {
		fmt.Println("new transaction creation")
		rand.Seed(time.Now().UnixNano())
		tempData := rand.Float64()
		tempHash := ckm.GenerateHashOfString(strconv.FormatFloat(tempData, 'f', -1, 64))
		transaction := dh.CreateTx(gwPrivateKey, &dag, tempHash)
		vertex := dh.AddToDAG(transaction, &dag, gwPrivateKey)
		p2p.BroadcastVtx(vertex)
	}
}

func getTx(c *gin.Context) {
	txHash := c.Param("tx_hash")
	dag.Mux.Lock()
	tx, ok := dag.Graph[txHash]
	if !ok {
		dag.Mux.Unlock()
		c.JSON(http.StatusNotFound, gin.H{
			"error": "transaction not found",
		})
		return
	}
	time1 := tx.Tx.Timestamp
	dag.Mux.Unlock()
	fmt.Println("Orphan Count: time1: ", time1)
	c.IndentedJSON(http.StatusOK, tx)
}

func gwHTTPServer() {
	// gin server
	router := gin.Default()
	router.Static("/pics", "./pics")
	router.GET("/getDagImage", getDagImage)
	router.GET("/getTx/:tx_hash", getTx)
	router.Run(":4000")
}

func demo() {
	// create keys for the gateway
	gwPrivateKey, gwPublicKey = ckm.GenerateKeyPair()
	serPubKey, _ := json.Marshal(gwPublicKey)
	fmt.Println("Orphan Count: gwPublicKey:> ", gwPublicKey)
	fmt.Println("Orphan Count: ", gwPublicKey.X, gwPublicKey.Y)
	fmt.Println("Orphan Count: ", string(serPubKey))
	// sessionExpiry = 3

	// avgRecRate = 100
	// generate session id
	// createSession()

	fmt.Println("successfully generated keys for the gateway")
	fmt.Println("\ngateway private key: ", gwPrivateKey)
	fmt.Println("\ngateway public key: ", gwPublicKey)
	// fmt.Println("\nsession id: ", session.Id)

	// Initialize the DAG
	dh.InitDag(&dag)

	go gwHTTPServer()

	// connect to the css
	// err := connectToCss()
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// create genesis transaction
	genesisTransaction := dh.CreateGenesisTx()
	fmt.Println("\nsuccessfully created genesis transaction")
	//add the genesis transaction to the dag
	dh.AddToDAG(genesisTransaction, &dag, gwPrivateKey)
	fmt.Println("\nsuccessfully added genesis transaction to the dag")

	time.Sleep(30 * time.Second)
	// now make connections with other gateways and start listening for connections
	p2ptest := make(chan bool)
	go p2p.Gateway(&dag, gwPrivateKey, p2ptest)
	// now start listening for connections
	// p2pStartListening <- true

	test := <-p2ptest
	fmt.Println(test)
	//check network connectivity
	if true {
		fmt.Println("p2p setup successful")
		simulateTransactions()
	} else {
		fmt.Println("Sensor is not running")
	}
	// for {

	// }
}

func main() {
	time.Sleep(1 * time.Minute)
	demo()
}
